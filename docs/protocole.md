# Protocole client ↔ serveur

Ce document décrit tout ce dont un client (web, mobile, bot externe) a besoin : l'authentification, l'API REST et les messages WebSocket. Les exemples supposent le serveur sur `https://coinche.example.com`.

## Conventions

- Tout est en JSON UTF-8. Les dates sont au format RFC 3339 (UTC).
- **Cartes** : `"<valeur>-<couleur>"`.
  - Valeurs : `7`, `8`, `9`, `10`, `V`, `D`, `R`, `A`.
  - Couleurs : `coeur`, `carreau`, `trefle`, `pique`.
  - Exemples : `"V-coeur"`, `"10-pique"`.
- **Atouts** : `coeur`, `carreau`, `trefle`, `pique`, `sans-atout`, `tout-atout`.
- **Sièges** : de `0` à `3`, dans l'ordre de jeu.
  - L'équipe `0` (A) occupe les sièges 0 et 2, l'équipe `1` (B) les sièges 1 et 3.
  - Les tableaux de scores sont indexés par équipe : `[A, B]`.
- **Erreurs REST** : `{"erreur": {"code": "…", "message": "…"}}`, avec un statut HTTP adapté.
- **Erreurs WebSocket** : message `{"type": "erreur", "ref": "…", "data": {"code": "…", "message": "…"}}`. Le `ref` reprend celui de la commande fautive.

## Authentification

| Méthode | Route | Description |
|---|---|---|
| GET | `/auth/fournisseurs` | Fournisseurs activés : `{"fournisseurs": ["google", "discord"]}` |
| GET | `/auth/{google\|discord}/connexion?retour=/chemin` | Redirige vers le fournisseur (OAuth 2.0 + PKCE). `retour` est un chemin relatif du client web. Un cookie `coinche_oauth` lie la connexion au navigateur : un lien de retour ouvert dans un autre navigateur est refusé. |
| GET | `/auth/{fournisseur}/retour` | Retour du fournisseur : pose le cookie de session puis redirige vers `COINCHE_URL_FRONT + retour`. Ajoute `?nouveau=1` à la première connexion, pour proposer de choisir un pseudo. En cas d'échec, redirige vers `/connexion?erreur=refusee\|expiree\|identification\|indisponible`. |
| POST | `/auth/deconnexion` | Ferme la session (204). |
| POST | `/auth/ticket` | Donne un ticket WebSocket à usage unique valable 30 s : `{"ticket": "…"}`. |
| POST | `/auth/dev` | **Développement seulement** (`COINCHE_AUTH_DEV=true`). Corps `{"pseudo": "Alice"}`, réponse `{"joueur": {…}, "token": "…"}`. |

Les routes `/auth/…` sont limitées à 30 requêtes en rafale par adresse IP (1 par seconde ensuite).

La session est un cookie `coinche_session` (`HttpOnly`, `Secure`, `SameSite=Lax`, 30 jours glissants). Les clients qui ne gèrent pas les cookies peuvent envoyer le même jeton dans l'en-tête `Authorization: Bearer <jeton>`.

Les requêtes REST qui modifient des données exigent `Content-Type: application/json`. C'est ce qui protège contre les formulaires tiers (CSRF).

## API REST

| Méthode | Route | Auth | Description |
|---|---|---|---|
| GET | `/sante` | – | Vivacité du processus. |
| GET | `/pret` | – | Disponibilité de Postgres et Redis (200 ou 503 avec le détail). |
| GET | `/version` | – | Version du serveur. |
| GET | `/regles` | – | Règles par défaut, règles classées, valeurs autorisées, messages de chat prédéfinis, paramètres d'ELO. |
| GET | `/bots` | – | Familles de bots, de la plus prudente à la plus téméraire : `{"bots": [{"id", "famille", "style", "description"}]}`. La famille décide de la façon de jouer. |
| GET | `/moi` | oui | Mon profil, plus `table` si je suis à une table. |
| PATCH | `/moi` | oui | `{"pseudo": "Nouveau"}` : de 3 à 20 caractères parmi `[A-Za-z0-9_-]`. Erreurs `pseudo_invalide` (400) ou `pseudo_pris` (409). |
| GET | `/joueurs?pseudo=Alice` | – | Recherche exacte par pseudo, insensible à la casse. |
| GET | `/joueurs/{id}` | – | Profil public : `elo`, `elo_max`, `nb_parties`, `nb_victoires`, `provisoire`, `rang`. |
| GET | `/joueurs/{id}/parties?avant=<date>&limite=20` | – | Historique, du plus récent au plus ancien. Si la page est pleine, `suivant` donne la valeur à passer dans `avant` pour la page suivante. |
| GET | `/joueurs/{id}/elo?limite=100` | – | Évolution de l'ELO, partie par partie : `{"points": [{partie, date, elo, delta}]}`. |
| GET | `/classement?page=1&taille=50` | – | Classement public. Les joueurs provisoires (moins de 10 parties classées) n'y figurent pas. |
| GET | `/parties/{id}` | – | Résumé d'une partie : participants, scores, ELO avant et après. |
| GET | `/parties/{id}/evenements` | selon le cas | Replay complet, mains comprises, seulement pour une partie terminée. Une partie privée n'est visible que de ses joueurs (403 sinon). |
| POST | `/tables` | oui | Crée une table privée. Corps : `{"regles": {...}, "delai_action": 30}` ; les champs absents prennent les valeurs par défaut. Réponse 201 : `{"table": "<uuid>", "code": "K7M2QX", "options": {...}}`. Renvoie 409 `deja_a_une_table` si l'on est déjà à une table. |

Options d'une table :

```json
{
  "regles": {
    "objectif": 1000,
    "decompte": "faits-et-contrat",
    "distribution": "traditionnelle"
  },
  "delai_action": 30
}
```

| Champ | Valeurs possibles |
|---|---|
| `objectif` | de 500 à 3000, par pas de 500 |
| `decompte` | `faits-et-contrat` ou `contrat-seul` |
| `distribution` | `traditionnelle` (on ramasse les plis et on coupe) ou `aleatoire` (mélange complet à chaque mène) |
| `delai_action` | 0 (sans limite), 15, 30 ou 60 secondes |

## WebSocket `/ws`

Il y a trois façons de s'authentifier :
- le cookie de session (même site) ;
- `?ticket=<ticket>` (ticket obtenu par `POST /auth/ticket`) ;
- l'en-tête `Authorization: Bearer` (clients autres que les navigateurs).

L'origine doit faire partie de `COINCHE_ORIGINES`.

Chaque message, dans les deux sens, a la forme `{"type": "...", "ref": "optionnel", "data": {...}}`.

Le serveur commence par `bienvenue` : `{"joueur": {id, pseudo, elo}, "connexion": "<id>"}`. Il n'y a ensuite rien à faire pour se reconnecter :
- si le joueur a une partie en cours, la connexion la rejoint automatiquement et reçoit `partie.etat` ;
- s'il est dans la file classée, il reçoit `file.rejointe`, ou `file.trouvee` puis `file.acceptation` si une table l'attend.

Limites :
- 10 messages par seconde et par connexion, avec des pointes jusqu'à 20 ; au-delà, le serveur répond `trop_de_requetes` ;
- 5 connexions simultanées par compte et par instance (HTTP 429 au-delà).

File classée et tables privées s'excluent :
- rejoindre ou créer une table privée fait quitter la file ;
- `file.rejoindre`, `duo.*` et `file.accepter` sont refusés (`deja_a_une_table`) tant qu'on est à une table ;
- fermer son dernier onglet fait quitter la file, sauf lors d'un redémarrage du serveur.

### Messages du client

| Type | Données | Effet |
|---|---|---|
| `ping` | – | Le serveur répond `pong`. |
| `file.rejoindre` | – | Entre seul dans la file classée. |
| `file.quitter` | – | Quitte la file (un duo la quitte ensemble). |
| `file.accepter` / `file.refuser` | `{"proposition": "<id>"}` | Répond à une table trouvée. |
| `duo.inviter` | `{"pseudo": "Bob"}` | Invite un partenaire à entrer en file avec soi. |
| `duo.repondre` | `{"invitation": "<id>", "accepte": true}` | Accepter une invitation fait entrer le duo en file. |
| `table.rejoindre` | `{"code": "K7M2QX"}` ou `{"table": "<uuid>"}` | Rejoint une table privée et prend le premier siège libre. |
| `table.etat` | – | Redemande l'instantané `partie.etat`. |
| `table.siege` | `{"siege": 2}` | Change de siège (avant le lancement). |
| `table.bot` | `{"siege": 3, "profil": "narvaloche"}` | Place un bot (créateur seulement, avant le lancement). `profil` est l'`id` d'une famille de `GET /bots`, facultatif : sans lui, la famille est tirée au hasard. Le bot prend pour pseudo un prénom de sa famille tiré au hasard, suivi du nom de famille (« Django Narvaloche ») ; deux bots d'une même table ne portent jamais le même nom. |
| `table.liberer` | `{"siege": 3}` | Retire un bot ou exclut un joueur (créateur seulement). |
| `table.lancer` | – | Lance la partie (créateur seulement, 4 sièges occupés). |
| `table.quitter` | – | Quitte une table pas encore lancée. |
| `partie.encherir` | `{"action": "annonce", "valeur": 90, "atout": "pique"}`, ou `{"action": "passe"}`, `"coinche"`, `"surcoinche"` | Enchérit (Capot = 250, Générale = 500). |
| `partie.jouer` | `{"carte": "V-coeur"}` | Joue une carte. |
| `partie.present` | – | Reprend la main après avoir été déclaré absent. |
| `partie.abandonner` | – | Partie classée : forfait immédiat. Partie privée : un bot reprend le siège. |
| `partie.chat` | `{"texte": "…"}` (500 caractères maximum) ou `{"message": "bien_joue"}` | Messages rapides : `bonne_chance`, `bien_joue`, `merci`, `desole`, `oups`, `gg`. |

### Messages du serveur

Les messages de partie portent `table` (uuid) et `seq`, le numéro de l'événement dans le journal de la partie. `seq` croît strictement : un client peut ignorer un doublon.

| Type | Données principales |
|---|---|
| `partie.etat` | Instantané complet : `type` (`classee`, `privee` ou `avec_bots`), `statut` (`attente`, `en_cours`, `terminee`), `code`, `createur`, `options`, `sieges[4]` (`joueur`, `bot`, `profil`, la famille d'un bot, `statut`, `retour_avant`), `moi`, `echeance` et `jeu`. `jeu` contient : `scores`, `historique`, ma `main`, et la mène en cours dans `mene` (`phase`, `tour`, `donneur`, `encheres`, `contrat`, `preneur`, `multiplicateur`, `pli_en_cours`, `plis`, `nb_cartes`, `belote` une fois annoncée, et pour le joueur dont c'est le tour, `cartes_jouables` ou `options_enchere`). |
| `partie.commencee` | `{table, type}` : une partie démarre (ou reprend). La connexion la rejoint d'elle-même. |
| `partie.donne` | `{mene, donneur, main}` : la main du joueur seulement. |
| `partie.tour` | `{siege, phase, echeance}`. Le joueur concerné reçoit en plus `cartes_jouables` ou `options_enchere` (`{passe, annonce_min, coinche, surcoinche}`). |
| `partie.enchere` | `{siege, action, contrat?}` |
| `partie.contrat` | `{contrat: {atout, valeur}, preneur, multiplicateur}` : les enchères sont closes. |
| `partie.redistribution` | `{mene}` : tout le monde a passé, une nouvelle donne suit. |
| `partie.carte` | `{siege, carte, belote?}`, où `belote` vaut `"belote"` ou `"rebelote"`. |
| `partie.pli` | `{pli: {entame, cartes[{siege, carte}], gagnant}, points}` |
| `partie.mene` | `{mene, resultat, scores}`. `resultat` contient : `contrat`, `preneur`, `multiplicateur`, `plis_gagnes`, `points_faits`, `belote`, `capot`, `reussi`, `points`. |
| `partie.statut` | `{siege, statut, retour_avant?}`. Statuts possibles : `connecte`, `deconnecte`, `absent`, `bot`, `abandon`. |
| `partie.chat` | `{siege, pseudo, texte}` ou `{siege, pseudo, message}` |
| `partie.terminee` | `{gagnante, cause: "score"\|"forfait", scores, elo: [{joueur, avant, apres, delta}]}` |
| `table.fermee` | `{table, raison}`. Raisons : `expiree`, `abandonnee`, `quittee`, `arret_serveur` (un salon non lancé ne survit pas au redémarrage de son instance). |
| `table.exclu` | Le créateur m'a retiré de la table ; je ne peux plus la rejoindre (code `exclu`). |
| `file.rejointe` | `{mode: "solo"\|"duo", depuis, partenaire?}` |
| `file.quittee` | `{raison}`. Raisons : `demande`, `deconnexion`, `partenaire`. |
| `file.trouvee` | `{proposition, expire_le, equipe_a, equipe_b}` : il faut accepter dans les 15 s. Un siège `null` est tenu par un bot : faute d'adversaires, un bot rejoint la table toutes les 5 s, et la partie (type `avec_bots`) ne compte alors pas pour l'ELO. |
| `file.acceptation` | `{proposition, acceptes}` : progression des acceptations, sur 4 ; les bots acceptent d'office. |
| `file.annulee` | `{proposition, raison, en_file}`. Raisons : `refus`, `delai`, `erreur`. Si `en_file` vaut vrai, le joueur est remis en file avec son ancienneté. |
| `duo.invitation` / `duo.envoyee` / `duo.refusee` / `duo.expiree` | `{invitation, de, a, expire_le}` |

### Codes d'erreur fréquents

| Code | Signification |
|---|---|
| `pas_ton_tour` | Ce n'est pas au joueur d'agir. |
| `phase` | Action impossible à ce moment de la mène. |
| `carte_interdite` | Règle de pose non respectée. |
| `carte_absente` | La carte n'est pas dans la main. |
| `enchere_invalide` | Enchère refusée. |
| `non_assis` | Le joueur n'est pas assis à cette table. |
| `exclu` | Le créateur de la table en a exclu le joueur. |
| `table_complete` | Il n'y a plus de siège libre. |
| `profil_inconnu` | Famille de bots inexistante. |
| `interdit` | Action réservée à un autre joueur (le créateur, par exemple). |
| `partie_non_commencee`, `partie_deja_lancee`, `partie_terminee` | La partie n'est pas dans l'état attendu. |
| `aucune_table` | Commande de partie envoyée sans être à une table. |
| `deja_a_une_table` | Le joueur est déjà à une autre table. |
| `deja_en_file` | Le joueur est déjà dans la file. |
| `partenaire_occupe` | Le partenaire invité est déjà dans la file. |
| `chat_invalide` | Message de chat refusé. |
| `trop_de_requetes` | Débit dépassé. |
| `requete_invalide` | Message mal formé. |
| `indisponible` | Service momentanément indisponible. |

## Déroulé type d'une partie classée

```
client → file.rejoindre
serveur ← file.rejointe
serveur ← file.trouvee {proposition}
client → file.accepter {proposition}
serveur ← partie.commencee {table}      (la connexion rejoint la table toute seule)
serveur ← partie.etat
serveur ← partie.donne {main}
serveur ← partie.tour {siege, phase: "encheres", options_enchere…}
client → partie.encherir {"action":"annonce","valeur":80,"atout":"coeur"}
…
serveur ← partie.terminee {gagnante, scores, elo}
```
