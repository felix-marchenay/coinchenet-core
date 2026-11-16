# Coinchenet — Spécifications du serveur de jeu

> Objectif : faire le « chess.com de la coinche ». Ce dépôt contient le **serveur de jeu** (backend) : il fait tourner des parties de coinche en ligne entre humains, avec des bots en renfort et un classement ELO.
>
> Toutes les questions ouvertes ont été tranchées : chaque choix est marqué **[Décidé]** et justifié dans le [journal des décisions](#11-journal-des-décisions). Le protocole client détaillé est dans [`docs/protocole.md`](docs/protocole.md).

---

## 1. Périmètre

### 1.1 Inclus dans la V1 (implémenté)

- Moteur de règles de la coinche complet : enchères, coinche et surcoinche, belote, décompte, fin de partie.
- Parties en temps réel entre 4 joueurs humains. Des bots remplacent un joueur absent ou déconnecté et peuvent compléter une table privée.
- **File classée**, en solo ou en duo avec un partenaire invité. Seules ces parties comptent pour l'ELO.
- **Tables privées** : sur code d'invitation, avec des règles configurables, non classées.
- **ELO individuel** et classement public.
- Connexion par **OAuth** (Google, Discord). Une connexion par simple pseudo existe pour le développement.
- Historique complet des parties par **event sourcing**, ce qui permet de les rejouer.
- Chat de table : texte libre en table privée, messages prédéfinis en partie classée.
- API REST et WebSocket documentées (le front est un autre projet).
- Fonctionnement sur plusieurs instances, avec reprise des parties d'une instance arrêtée.

### 1.2 Hors périmètre de la V1

- Le client (web ou mobile), à développer dans un autre dépôt.
- Le mode spectateur, la liste d'amis et la détection de collusion.
- Le mode invité (jouer sans compte).
- Les annonces autres que la belote (tierce, cinquante, cent, carrés).
- Plusieurs niveaux de bot.
- Une interface d'administration.

---

## 2. Règles du jeu

### 2.1 Table et ordre de jeu

- 4 sièges numérotés de 0 à 3. L'équipe A occupe les sièges 0 et 2, l'équipe B les sièges 1 et 3 : les partenaires sont face à face.
- On joue dans le sens antihoraire (siège `n` → siège `(n+1) % 4`).
- Le premier donneur est tiré au sort, puis le rôle de donneur tourne d'un siège à chaque mène, redistributions comprises.
- Le joueur qui suit le donneur parle en premier aux enchères et entame le premier pli.
- Le gagnant d'un pli entame le pli suivant.

### 2.2 Donne

- Jeu de 32 cartes, mélangé avec Fisher-Yates et `crypto/rand`, puis coupé à une position aléatoire.
- Distribution en 3-2-3 depuis le dessus du paquet, en commençant par le joueur qui suit le donneur.
- Deux modes de distribution existent pour les mènes suivantes :
  - **traditionnelle** : on ramasse les plis dans l'ordre où ils ont été joués, on coupe et on distribue ;
  - **aléatoire** : le paquet est entièrement remélangé.
- **[Décidé]** Les parties classées utilisent la distribution aléatoire (voir D1). Les tables privées utilisent la distribution traditionnelle par défaut.
- Si les 4 joueurs passent, la mène est annulée, sans points. Le donneur suivant redistribue ; en mode traditionnel, on ramasse les mains en commençant par le joueur qui suit le donneur.

### 2.3 Enchères

- Contrats : de 80 à 160 par paliers de 10, **Capot** (250) et **Générale** (500).
- Atouts : ♥ ♦ ♣ ♠, **Sans-atout** et **Tout-atout**.
- Chaque annonce doit être strictement supérieure à la précédente, quelle que soit la couleur.
- Un joueur qui a passé peut reparler quand la parole lui revient. On peut surenchérir sur son partenaire.
- Les enchères s'arrêtent après 3 passes consécutives qui suivent une annonce.
- **Coinche** : seul un adversaire de l'équipe preneuse peut coincher, et seulement quand c'est à lui de parler. Le contrat compte alors double (×2).
- **[Décidé]** La coinche clôt les enchères (D2). La parole passe aux deux joueurs de l'équipe preneuse, dans l'ordre de jeu. Chacun peut **surcoincher** (×4, fin immédiate) ou passer. Si les deux passent, le contrat reste coinché.
- Avec un contrat Générale, seul le joueur qui l'a annoncée doit remporter tous les plis.

### 2.4 Jeu de la carte

1. Il faut fournir la couleur demandée.
2. Si l'atout est demandé, il faut monter quand on le peut, **même si le partenaire est maître**. Si l'on ne peut pas monter, on joue n'importe quel atout.
3. Si on n'a pas la couleur demandée et que le pli est à un adversaire, il faut couper.
4. Si un adversaire a déjà coupé, il faut **surcouper** quand on le peut, et sinon **sous-couper** : on ne se défausse pas tant qu'on a de l'atout.
5. **[Décidé]** Si le partenaire est maître du pli, on n'est pas obligé de couper : défausse libre (D3).
6. En Tout-atout, chaque couleur suit l'ordre de l'atout (V 9 A 10 R D 8 7) : il faut monter dans la couleur demandée, et on ne coupe jamais.
7. En Sans-atout, il faut fournir sans obligation de monter ; sinon défausse libre.

Ordre des cartes : à l'atout, V 9 A 10 R D 8 7 ; hors atout, A 10 R D V 9 8 7.

### 2.5 Décompte d'une mène

- Une mène vaut 162 points au total : 152 points de cartes plus 10 pour le **dix de der**. Ce total est le même en couleur, en Sans-atout et en Tout-atout.
- **Belote / rebelote** :
  - Le joueur qui a le Roi et la Dame d'atout à la fin des enchères marque 20 points pour son équipe.
  - Elle est annoncée automatiquement quand il joue la première de ces cartes (« belote »), puis la seconde (« rebelote »).
  - Elle reste acquise à son équipe, que le contrat soit fait ou chuté.
  - **[Décidé]** Il n'y a pas de belote en Sans-atout ni en Tout-atout (D4).
  - **[Décidé]** La belote compte pour remplir le contrat (D5).
- **[Décidé]** Un contrat est **fait** si l'équipe preneuse atteint la valeur du contrat **et** fait plus de points que la défense, belote comprise de part et d'autre (D6).
  - Exemple : un contrat de 80 réalisé avec 80 points contre 82 est chuté.
  - Exception : un Capot est fait si l'équipe preneuse remporte les 8 plis, une Générale si le preneur seul les remporte.
- **Capot non annoncé** : l'équipe qui remporte les 8 plis compte 250 points faits au lieu de 162.

Décompte selon le mode configuré (`v` = valeur du contrat, `m` = multiplicateur 1, 2 ou 4) :

| Situation | « Points faits + contrat » (défaut) | « Contrat seul » |
|---|---|---|
| Contrat fait, non coinché | Preneur : points faits + v. Défense : ses points faits | Preneur : v. Défense : 0 |
| Contrat fait, coinché ou surcoinché | Preneur : 160 + v × m. Défense : 0 | Preneur : v × m. Défense : 0 |
| Contrat chuté | Défense : 160 + v × m. Preneur : 0 | Défense : v × m. Preneur : 0 |
| Capot annoncé et fait | 250 + 250 = **500** (**[Décidé]**, D7) | 250 |
| Générale annoncée et faite | 250 + 500 = **750** (**[Décidé]**, D7) | 500 |
| Capot / Générale chutés | Défense : 160 + 250, ou 160 + 500 | Défense : 250 ou 500 |

Dans tous les cas, on ajoute ensuite la belote (+20) à l'équipe qui la détient. **[Décidé]** Quand l'équipe qui gagne la mène a fait tous les plis, la base de 160 est remplacée par 250, que la mène soit coinchée ou non : une défense qui fait capot sur un 80 marque 250 + 80 (D7).

### 2.6 Fin de partie

- La partie se termine à la fin de la mène au cours de laquelle au moins une équipe atteint ou dépasse l'**objectif**. L'équipe en tête l'emporte.
- En cas d'égalité au-dessus de l'objectif, on joue une mène de plus.
- Une partie peut aussi se terminer par **forfait** (§ 4.3).

### 2.7 Règles configurables

| Paramètre | Valeurs possibles | Partie classée (imposé) |
|---|---|---|
| Objectif de points | de 500 à 3000, par pas de 500 | **1000** |
| Mode de décompte | `faits-et-contrat` ou `contrat-seul` | **faits-et-contrat** |
| Distribution | `traditionnelle` ou `aleatoire` | **aleatoire** |
| Délai par action | 0 (illimité), 15, 30 ou 60 s | **30 s** (**[Décidé]**, configurable par l'hébergeur) |
| Bots au démarrage | oui en table privée | non : 4 humains |

---

## 3. Classement ELO

### 3.1 Principes

- Chaque joueur a son **ELO individuel**. **[Décidé]** Il démarre à **1200**, avec un plancher à 100 (D8).
- Seules les parties de la file classée font varier l'ELO.
- Les bots n'ont pas d'ELO. Une partie classée démarre toujours avec 4 humains.

### 3.2 Formule

```
R_A = (elo(a1) + elo(a2)) / 2
R_B = (elo(b1) + elo(b2)) / 2
E_A = 1 / (1 + 10^((R_B - R_A) / 400))
S_A = 1 si A gagne, 0 sinon
Δ(joueur de A) = round(K(joueur) × (S_A − E_A))
```

- **[Décidé]** `K = 40` pendant les 30 premières parties classées du joueur, puis `K = 20` (D8). Deux partenaires peuvent donc varier différemment.
- **[Décidé]** Un joueur reste « provisoire » tant qu'il a joué moins de 10 parties classées. Il n'apparaît alors pas au classement public et n'a pas de rang (D8).
- Le calcul se fait en base, dans la même transaction que la clôture de la partie. Il part des ELO **courants**, ceux du début de partie ayant pu changer entre-temps.

### 3.3 Abandon

| Joueur | Effet sur l'ELO |
|---|---|
| Le joueur qui abandonne | Défaite normale, plus une **pénalité de −15** (**[Décidé]**, D9) |
| Son partenaire | Aucun changement |
| Les adversaires | Victoire normale |

---

## 4. Déroulement d'une partie en ligne

### 4.1 Formation des tables

**File classée**

- On entre dans la file seul, ou en duo après avoir invité un partenaire par son pseudo (l'invitation expire au bout d'une minute).
- Le leader de la file cherche une table chaque seconde, en servant d'abord les joueurs qui attendent depuis le plus longtemps.
- **[Décidé]** La fenêtre d'ELO tolérée part de ±100 et s'élargit de 50 toutes les 15 s, jusqu'à ±400.
- Avec 4 joueurs seuls, les équipes sont équilibrées : le plus fort joue avec le plus faible.
- **[Décidé]** Un duo affronte de préférence un autre duo. Après 30 s d'attente, il peut affronter deux joueurs seuls (D10).
- Quand une table est trouvée, chaque joueur doit accepter dans les 15 s.
  - Ceux qui refusent ou ne répondent pas sortent de la file, avec leur partenaire de duo.
  - Les autres reviennent en file avec leur ancienneté.

**Table privée**

- Le créateur choisit les règles (§ 2.7) et reçoit un code de 6 caractères, sans caractères ambigus et insensible à la casse.
- Les joueurs rejoignent la table avec ce code et prennent un siège libre ; ils peuvent en changer.
- Le créateur place des bots, exclut des joueurs (qui ne peuvent plus revenir) et lance la partie quand les 4 sièges sont occupés.
- Si le créateur s'en va, la main passe à un autre joueur humain ; s'il n'y en a plus, la table est fermée.
- **[Décidé]** Une table non lancée expire au bout de 30 minutes. Elle n'est pas journalisée : si son instance s'arrête, elle est fermée et ses joueurs sont libérés (`table.fermee`, raison `arret_serveur`).
- **[Décidé]** File classée et table privée s'excluent (D16) :
  - rejoindre ou créer une table privée fait quitter la file ;
  - on ne peut pas entrer en file ni accepter une partie classée depuis une table.

### 4.2 Délai par action et absence

- Chaque action doit être jouée avant l'échéance annoncée dans `partie.tour`. Sinon, le bot joue à la place du joueur.
- **[Décidé]** Après 2 délais expirés d'affilée, le joueur est déclaré **absent** : le bot joue alors pour lui sans attendre (D11). Le joueur reprend la main en envoyant `partie.present`, ou simplement en jouant.

### 4.3 Déconnexion et abandon

- Quand un joueur se déconnecte, le bot prend le relais pour ne pas bloquer la table. Le joueur peut avoir plusieurs onglets : il n'est déconnecté que lorsque tous sont fermés.
- **[Décidé]** En partie classée, il a **3 minutes** pour revenir (D9). À son retour, il reçoit l'état complet ; s'il ne revient pas à temps, son équipe perd par forfait.
- `partie.abandonner` en partie classée : forfait immédiat.
- `partie.abandonner` en partie privée : le bot finit la partie à la place du joueur. S'il ne reste plus aucun humain, la partie s'arrête.

### 4.4 Chat

- En table privée, le texte est libre : 500 caractères maximum, sans caractères de contrôle.
- En partie classée, seuls des messages prédéfinis sont autorisés : `bonne_chance`, `bien_joue`, `merci`, `desole`, `oups`, `gg`.
- Chaque joueur peut envoyer au plus 5 messages par tranche de 10 secondes.
- Une fois la partie lancée, les messages sont journalisés, ce qui permet la modération.

---

## 5. Bot

- Le bot V1 est un bot simple à règles, déterministe à état égal.
- **Enchères** :
  - il évalue chaque atout possible en « levées probables », et estime son contrat à levées × 20 + 30 (apport du partenaire), plafonné à 160 ;
  - il n'annonce un capot que si toutes ses cartes sont sûres ;
  - il soutient son partenaire dans sa couleur et ne change jamais la couleur annoncée par son partenaire ;
  - il ne coinche ni ne surcoinche jamais.
- **Jeu** : il fait tomber les atouts quand son équipe a pris et qu'il tient l'atout maître. Il joue ses cartes maîtresses, charge des points quand son partenaire est sûr de remporter le pli, prend avec la plus petite carte suffisante et se défausse au moindre coût.
- Il n'utilise que les informations publiques et sa propre main.
- En simulation (1000 parties), environ 70 % des contrats sont réussis et le score moyen est de 272 points marqués par mène, cohérent avec la coinche réelle.

---

## 6. Architecture technique

### 6.1 Choix

| Sujet | Choix |
|---|---|
| Langage | Go 1.27 |
| Architecture | Monolithe modulaire : un binaire, des packages par domaine |
| Temps réel | WebSocket (`coder/websocket`) |
| Requêtes classiques | REST JSON (`net/http`, routage standard) |
| Données durables | PostgreSQL 18 (pgx) : joueurs, parties, événements, ELO |
| Données éphémères | Redis 8 : Pub/Sub entre instances, verrous, sessions, file classée, annuaire joueur → table, codes d'invitation |
| Authentification | OAuth 2.0 + PKCE (Google en OIDC, Discord), sessions opaques en Redis |
| Historique | Event sourcing des parties |
| Observabilité | Journaux `log/slog` en JSON, métriques Prometheus, pprof sur un port d'administration |
| Livraison | Image distroless non root, Docker Compose + Caddy, CI GitHub Actions publiant sur GHCR et déployant chaque merge sur `master` |

### 6.2 Organisation du code

```
cmd/coinchenet            serveur | migrer | simuler | sante | version
internal/coinche          moteur de règles PUR (bibliothèque standard uniquement, imposé par le lint)
internal/bot              bot à règles
internal/simulation       parties entre bots, avec invariants
internal/classement       ELO
internal/table            acteur de table, événements, délais, reprise, registre
internal/matchmaking      appariement, propositions, duos, leader unique
internal/auth             OAuth, sessions, tickets WebSocket
internal/api              REST et passerelle WebSocket
internal/stockage/...     postgres, redisstore
internal/application      assemblage et cycle de vie
```

Le domaine (`coinche`, `bot`, `classement`, `table`, `matchmaking`) ne dépend ni du stockage ni du transport : depguard le vérifie.

### 6.3 Exécution d'une table

- Chaque table est un **acteur** : une goroutine qui traite ses commandes une à une. Ce sont les actions des joueurs, les connexions, ainsi que les délais et le tour des bots, programmés avec un jeton qui invalide les échéances périmées.
- Pour chaque action, l'acteur :
  1. l'applique au moteur, qui la valide ;
  2. journalise dans Postgres l'événement source et ses conséquences, en une transaction ;
  3. diffuse les messages ;
  4. planifie l'étape suivante.
- Si la journalisation échoue, la table s'arrête et sera reconstruite depuis le journal : l'état en mémoire n'est jamais en avance sur le journal.
- Une table appartient à une seule instance, garantie par un verrou Redis (TTL 30 s, prolongé toutes les 10 s). Les commandes reçues par une autre instance sont relayées par Pub/Sub.
- **Reprise** : chaque instance cherche régulièrement les parties en cours sans propriétaire. Elle prend leur verrou, rejoue le journal et prévient les joueurs (`partie.commencee`) ; leurs connexions se rattachent d'elles-mêmes. Un journal irrécupérable conduit à annuler la partie.
- À l'arrêt, une instance rend ses tables : les autres les reprennent sans attendre l'expiration des verrous.

### 6.4 Event sourcing

- Chaque événement a la forme `(partie, séquence, type, données JSON, horodatage)`, avec une clé primaire `(partie, séquence)`. Si deux instances écrivent la même partie, la seconde échoue et s'arrête.
- **Événements source**, qui suffisent à rejouer la partie :
  - `partie_creee` (type, options, sièges, premier donneur) ;
  - `donne` (le **paquet complet**) ;
  - `enchere` ;
  - `carte` ;
  - `abandon`.
- **Faits dérivés**, conservés pour le replay et l'analyse :
  - `contrat`, `redistribution`, `pli`, `mene` ;
  - `chat` ;
  - `partie_terminee`, écrit dans la même transaction que la mise à jour de l'ELO.
- **Information cachée** : le journal contient tout, mais chaque joueur ne reçoit que sa main. Le replay complet n'est accessible qu'une fois la partie terminée ; pour une partie privée, seulement à ses joueurs.

### 6.5 Modèle de données (PostgreSQL)

```
joueurs(id, pseudo citext unique, elo, elo_max, nb_parties_classees, nb_victoires_classees, cree_le, vu_le)
identites(fournisseur, sujet, joueur_id, email)               PK (fournisseur, sujet)
parties(id, type, options jsonb, statut en_cours|terminee|annulee, cause, equipe_gagnante, score_a, score_b, debut, fin)
participants(partie_id, siege, joueur_id null si bot, pseudo, bot, elo_avant, elo_apres, abandon)
evenements(partie_id, sequence, type, donnees jsonb, horodatage)   PK (partie_id, sequence)
schema_migrations(version, appliquee_le)
```

Les migrations sont embarquées dans le binaire et appliquées au démarrage sous verrou consultatif, ce qui est sans risque à plusieurs instances.

### 6.6 Clés Redis

Toutes les clés sont préfixées par `coinche:`.

| Clé | Contenu |
|---|---|
| `verrou:table:<id>`, `verrou:matchmaking:leader` | Propriétaire, avec TTL |
| `joueur:<id>:table` | Table courante du joueur (reconnexion automatique) |
| `code:<CODE>` | Table privée correspondant au code |
| `session:<sha256>`, `ticket:<sha256>`, `oauth:<sha256>` | Sessions, tickets et états OAuth : seule l'empreinte du jeton est stockée |
| `matchmaking:commandes` (liste) et `matchmaking:etat` (JSON) | File de commandes et état du leader |

Canaux Pub/Sub : `joueur:<id>` et `table:<id>`.

---

## 7. API

La référence complète (REST, messages WebSocket, codes d'erreur, exemples) est dans [`docs/protocole.md`](docs/protocole.md).

En résumé :
- **REST** : comptes, profils, classement, historique, replay, création de table privée et santé.
- **WebSocket** : file classée, duos, table privée et jeu, avec un instantané complet `partie.etat` à chaque (re)connexion.

---

## 8. Exigences non fonctionnelles

- **Équité et anti-triche** :
  - le mélange utilise `crypto/rand` ;
  - le serveur est la seule source de vérité et revalide chaque commande ;
  - un joueur ne reçoit jamais la main d'un autre ;
  - le replay n'est possible qu'une fois la partie finie ;
  - le chat est restreint en partie classée ;
  - en partie classée, la distribution est aléatoire pour qu'on ne puisse pas déduire les mains (D1).
- **Sécurité** :
  - le cookie de session est `HttpOnly`, `Secure` et `SameSite=Lax` ;
  - la connexion OAuth est liée au navigateur par un cookie d'état, contre le « login CSRF » ;
  - les requêtes qui modifient des données doivent être en JSON (protection CSRF) ;
  - l'origine des WebSocket est contrôlée ;
  - les redirections après connexion sont limitées à des chemins relatifs ;
  - le débit est limité par client WebSocket, par adresse IP sur `/auth`, et pour la création de tables ; un compte ouvre au plus 5 connexions par instance ;
  - les entrées sont validées strictement (champs inconnus refusés) ;
  - l'image tourne en distroless, sans utilisateur root.
- **Performance** : le temps de traitement d'une action est dominé par l'écriture du journal (une transaction Postgres) ; la métrique `coinche_table_commande_secondes` le suit en production. Une table inactive ne coûte qu'une goroutine en attente et son état en mémoire. La capacité d'une instance reste à mesurer par un test de charge.
- **Observabilité** : journaux JSON et métriques Prometheus, dont :
  - tables actives, parties terminées par cause, temps de traitement des commandes ;
  - connexions WebSocket, taille de la file et temps d'attente ;
  - requêtes HTTP.
- **Tests** : voir le README. Ils comprennent les tests unitaires, les simulations entre bots, les tests à horloge virtuelle, les tests d'intégration Postgres et Redis, et un test de bout en bout à deux instances avec reprise après arrêt.

---

## 9. État de la réalisation

Tous les écarts relevés sur l'ancien moteur sont corrigés :

- [x] **Placement des sièges** : les partenaires sont face à face.
- [x] Contrôle du **tour de parole**.
- [x] **Rotation du donneur**, premier à parler et premier à entamer.
- [x] **Enchères complètes** : passe, fin après 3 passes, coinche, surcoinche, redistribution.
- [x] **Règle de pose** : surcoupe, sous-coupe, partenaire maître, tout-atout. L'ancien ordre hors atout (10 sous le valet) était faux et a été corrigé.
- [x] **Belote / rebelote**, **dix de der**, **capot non annoncé**.
- [x] **Décompte de la mène** : les deux modes et les multiplicateurs.
- [x] **Fin de partie** selon l'objectif.
- [x] Mélange sur `crypto/rand`.
- [x] Moteur déterministe et rejouable, piloté par des événements.
- [x] Sièges et équipes indexés (0-3, A/B) au lieu de pointeurs.

## 10. Découpage en lots

| Lot | Contenu | Statut |
|---|---|---|
| 1. Moteur complet | § 9 | ✅ |
| 2. Bot et simulation | Bot à règles, simulation avec invariants | ✅ |
| 3. Acteur de table et événements | Délais, journal Postgres, reconstruction | ✅ |
| 4. Comptes et API REST | OAuth Google et Discord, sessions, profils, classement, historique | ✅ |
| 5. WebSocket et tables privées | Passerelle, projections par joueur, salon, bots, chat | ✅ |
| 6. File classée et ELO | Solo et duo, acceptation, ELO, classement, chat prédéfini | ✅ |
| 7. Robustesse | Reconnexion, abandon, plusieurs instances, reprise, métriques, déploiement | ✅ |

Pistes pour la suite :
- un client web ;
- le mode spectateur, avec un différé et sans les mains ;
- une liste d'amis ;
- un bot plus fort (Monte-Carlo) ;
- la détection de collusion (paires de joueurs souvent adversaires) ;
- un back-office de modération (bannissement, lecture des chats).

---

## 11. Journal des décisions

| # | Question | Décision | Raison |
|---|---|---|---|
| D1 | Distribution des mènes en partie classée | Mélange complet à chaque mène | En distribution traditionnelle, l'ordre des plis ramassés est public. Un programme pourrait alors déduire une partie des mains : en classé, l'équité passe avant la tradition. Les tables privées gardent la distribution traditionnelle par défaut. |
| D2 | Peut-on surenchérir après une coinche ? | Non. La coinche clôt les enchères ; seule l'équipe preneuse peut encore surcoincher, chacun de ses joueurs ayant un tour de parole. | Règle la plus répandue en ligne, et plus simple pour les joueurs. |
| D3 | Obligation de couper quand le partenaire est maître | Non, défausse libre | Règle FFB de la coinche. À l'atout demandé, l'obligation de monter reste entière. |
| D4 | Belote en Sans-atout / Tout-atout | Aucune | Il n'y a pas d'atout unique. Évite aussi les variantes à une belote par couleur, peu jouées en classé. |
| D5 | La belote compte-t-elle pour le contrat ? | Oui | Usage majoritaire en coinche en ligne. |
| D6 | Contrat fait à égalité de points | Il faut atteindre le contrat **et** battre la défense | Pas de « litige » en coinche : 80 contre 82 est chuté. |
| D7 | Points d'un capot ou d'une générale réussis | Capot : 250 faits + 250 de contrat = 500. Générale : 250 + 500 = 750. Chute : 160 + valeur pour la défense. Coinché : 160 + valeur × multiplicateur. Si l'équipe gagnante a fait tous les plis, la base passe de 160 à 250, y compris pour une défense qui fait capot. | Une seule formule pour tous les contrats, cohérente avec le capot non annoncé (250 points faits). |
| D8 | Paramètres de l'ELO | Départ à 1200, plancher à 100, K = 40 pendant 30 parties puis 20, classement public à partir de 10 parties | Valeurs classiques de l'ELO : convergence rapide des nouveaux, stabilité ensuite. |
| D9 | Abandon | 3 minutes pour revenir, puis forfait. Pénalité de −15 pour l'auteur, 0 pour son partenaire. | Assez long pour une coupure réseau, assez court pour ne pas bloquer trois joueurs. Le partenaire ne doit pas payer pour l'autre. |
| D10 | Un duo doit-il affronter un duo ? | De préférence. Après 30 s d'attente, il peut affronter deux joueurs seuls. | Équité (coordination d'un duo habitué) sans attente interminable aux heures creuses. |
| D11 | Absence | 2 délais expirés d'affilée font déclarer le joueur absent ; le bot joue alors sans attendre. | Évite de faire attendre 30 s à chaque tour les trois autres joueurs. |
| D12 | Fournisseurs OAuth | Google (OIDC) et Discord, plus une connexion par pseudo réservée au développement | Discord est très utilisé par les joueurs. La connexion de développement permet de tester sans compte externe et refuse de s'activer par défaut. |
| D13 | Hébergement | Docker Compose sur un serveur, Caddy en frontal, 2 instances par défaut | Déploiement en une commande. Plusieurs instances fonctionnent dès la V1 grâce à Redis et à la reprise des parties. Kubernetes n'est pas nécessaire à ce stade. |
| D14 | Pseudo | Proposé à partir du nom chez le fournisseur (suffixe numérique en cas de collision), modifiable ensuite (3 à 20 caractères `[A-Za-z0-9_-]`) | Aucune étape bloquante à l'inscription. Le client propose de changer de pseudo à la première connexion (`?nouveau=1`). |
| D15 | Plis déjà joués visibles pendant la mène ? | Tous les plis de la mène en cours sont visibles des joueurs | N'importe quel client peut de toute façon les mémoriser au fil des messages : les montrer à tous évite un avantage aux clients modifiés. |
| D16 | Peut-on être en file classée et à une table privée ? | Non. Rejoindre ou créer une table privée fait quitter la file ; la file refuse un joueur assis. Fermer son dernier onglet fait aussi quitter la file. | Sinon, la même personne peut se retrouver assise à deux parties et perdre l'une par forfait. |
| D17 | Déploiement continu | À chaque merge sur `master`, la CI se connecte en SSH avec une clé restreinte à `coinchenet-deployer` (une clé par dépôt, liée à sa cible `api` ou `web`). Pour l'API, le serveur n'accepte qu'un commit de `master` et lit la configuration à ce commit. Les réglages `COINCHE_*` viennent des secrets et variables GitHub. API et client sur deux domaines | Le serveur n'a besoin d'aucun accès au dépôt ni au registre. Une clé de CI divulguée ne donne pas de shell : elle permet seulement de redéployer un commit de `master` (API) ou de publier un build du client (web). Le client et l'API se déploient indépendamment. |
