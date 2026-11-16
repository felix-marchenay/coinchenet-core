# Coinchenet — serveur de jeu de coinche

Le serveur du « chess.com de la coinche » : parties en ligne en temps réel, bots, file classée avec classement ELO, tables privées entre amis et replay complet des parties.

- **Moteur de règles complet** : enchères avec coinche et surcoinche, sans-atout et tout-atout, capot et générale, belote, dix de der, deux modes de décompte.
- **Parties en temps réel** en WebSocket. Un bot remplace un joueur absent ou déconnecté, et complète une table privée.
- **File classée** en solo ou en duo, avec appariement par ELO et acceptation de la table trouvée. Le classement ELO est individuel, et un abandon est pénalisé.
- **Event sourcing** : chaque partie est journalisée et peut être rejouée coup par coup.
- **Plusieurs instances possibles** derrière un répartiteur. Si une instance s'arrête, les autres reprennent ses parties depuis le journal.
- **Connexion OAuth** (Google, Discord), métriques Prometheus et journaux JSON.

La spécification fonctionnelle et les décisions prises sont dans [`spec.md`](spec.md). Le protocole client est documenté dans [`docs/protocole.md`](docs/protocole.md).

## Démarrage rapide

Seul Docker est nécessaire : Go, les tests et le linter tournent dans des conteneurs.

```bash
make up                      # Postgres + Redis + serveur sur http://localhost:8080
COINCHE_PORT=8188 make up    # sur un autre port
make logs
```

La stack de développement active la connexion par pseudo (`COINCHE_AUTH_DEV=true`) :

```bash
curl -s -X POST localhost:8080/auth/dev -H 'Content-Type: application/json' -d '{"pseudo":"Alice"}'
# → {"joueur": {...}, "token": "…"}

TOKEN=…
curl -s -X POST localhost:8080/tables -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{}'
# → {"table": "…", "code": "K7M2QX", …}

websocat -H "Authorization: Bearer $TOKEN" ws://localhost:8080/ws
{"type":"table.rejoindre","data":{"code":"K7M2QX"}}
{"type":"table.bot","data":{"siege":1}}
{"type":"table.bot","data":{"siege":2}}
{"type":"table.bot","data":{"siege":3}}
{"type":"table.lancer"}
```

## Commandes

| Commande | Rôle |
|---|---|
| `make up` / `make down` / `make logs` | Démarre, arrête ou suit la stack de développement |
| `make test` | Tests unitaires, sans base |
| `make test-race` | Tests unitaires avec le détecteur de concurrence |
| `make test-integration` | Tests contre Postgres et Redis, dont le test de bout en bout à deux instances |
| `make test-coverage` | Couverture des tests unitaires |
| `make lint` / `make fmt` | golangci-lint (avec des règles d'architecture) et formatage |
| `make simulate N=5000` | Fait jouer N parties entre bots et vérifie les invariants |
| `make build` | Construit l'image de production |

Le binaire `coinchenet` propose les sous-commandes `serveur` (par défaut), `migrer`, `simuler`, `sante` et `version`.

## Architecture

```
cmd/coinchenet            point d'entrée (serveur, migrations, simulation, healthcheck)
internal/coinche          moteur de règles pur : aucune E/S, aucun hasard non injecté
internal/bot              bot à règles (enchères et jeu de la carte)
internal/simulation       parties entre bots, avec vérification des invariants
internal/classement       calcul de l'ELO
internal/table            acteur de table, événements, délais, reprise, registre des tables
internal/matchmaking      file classée : appariement, propositions, duos, leader unique
internal/auth             OAuth 2.0 / OIDC, sessions, tickets WebSocket
internal/api              API REST et passerelle WebSocket
internal/stockage         postgres (journal, parties, joueurs, ELO) et redisstore (bus, verrous, sessions, file)
internal/application      assemblage et cycle de vie
```

`golangci-lint` (depguard) fait respecter deux règles : le moteur ne dépend que de la bibliothèque standard, et le domaine (`coinche`, `bot`, `classement`, `table`, `matchmaking`) ne dépend ni du stockage ni du transport.

### Une partie = un acteur

Chaque table est une goroutine qui traite ses commandes une par une : actions des joueurs, délais, bots, connexions. L'état d'une partie n'a donc besoin d'aucun verrou.

Chaque action est appliquée au moteur, puis le résultat est journalisé dans Postgres avant d'être diffusé. Le journal contient le paquet complet de chaque donne : la partie se rejoue sans aucun tirage.

### Plusieurs instances

Une table appartient à une seule instance à la fois, grâce à un verrou Redis prolongé en continu. Les commandes et les messages transitent par Redis Pub/Sub, quelle que soit l'instance où le joueur est connecté. Une seule instance, le leader, dirige la file classée.

Quand une instance s'arrête, ou qu'elle perd son verrou, une autre reprend la partie : elle rejoue le journal et prévient les joueurs. Leurs connexions se rattachent automatiquement à la partie reprise. Le test de bout en bout `internal/application` le vérifie avec deux instances réelles.

## Configuration

Toute la configuration passe par des variables d'environnement.

| Variable | Défaut | Rôle |
|---|---|---|
| `COINCHE_POSTGRES_URL` | **obligatoire** | URL Postgres |
| `COINCHE_REDIS_URL` | **obligatoire** | URL Redis |
| `COINCHE_ADRESSE` | `:8080` | Écoute HTTP / WebSocket |
| `COINCHE_ADRESSE_ADMIN` | `:9090` | `/metrics` (Prometheus) et `/debug/pprof/`, à ne pas exposer |
| `COINCHE_URL_PUBLIQUE` | `http://localhost:8080` | Adresse publique du serveur (retours OAuth) |
| `COINCHE_URL_FRONT` | `= URL publique` | Client web vers lequel on redirige après connexion |
| `COINCHE_ORIGINES` | `= URL front` | Origines autorisées (CORS et WebSocket), séparées par des virgules |
| `COINCHE_COOKIE_SECURE` / `COINCHE_COOKIE_DOMAINE` | `true` / vide | Cookie de session |
| `COINCHE_DUREE_SESSION` | `720h` | Durée glissante d'une session |
| `COINCHE_GOOGLE_CLIENT_ID` / `_SECRET` | vide | Active la connexion Google |
| `COINCHE_DISCORD_CLIENT_ID` / `_SECRET` | vide | Active la connexion Discord |
| `COINCHE_AUTH_DEV` | `false` | Connexion par simple pseudo, **développement uniquement** |
| `COINCHE_INSTANCE` | nom d'hôte | Identifiant de l'instance (verrous) |
| `COINCHE_LOG_NIVEAU` / `COINCHE_LOG_FORMAT` | `info` / `json` | Journaux (`texte` pour le développement) |
| `COINCHE_MIGRER_AU_DEMARRAGE` | `true` | Applique les migrations au démarrage (sans risque à plusieurs instances) |
| `COINCHE_DELAI_ACTION_CLASSEE` | `30` | Secondes par action en partie classée (15, 30 ou 60) |
| `COINCHE_DELAI_RETOUR` | `3m` | Délai de retour d'un joueur déconnecté avant le forfait (partie classée) |
| `COINCHE_DELAI_BOT` | `800ms` | Rythme des actions automatiques |
| `COINCHE_DELAI_DEMARRAGE` / `COINCHE_DELAI_ENTRE_MENES` | `3s` / `3s` | Pauses de mise en scène |
| `COINCHE_DELAI_LOBBY` | `30m` | Durée de vie d'une table privée non lancée |
| `COINCHE_INTERVALLE_REPRISE` | `10s` | Fréquence de recherche des parties orphelines |
| `COINCHE_DEBIT_WS` | `10` | Messages par seconde acceptés d'un client WebSocket |
| `COINCHE_ARRET_GRACIEUX` | `20s` | Temps laissé à l'arrêt propre |

## Tests

- **Moteur** : tests tabulaires des règles de pose, des enchères et du décompte, une partie complète, et le rejeu.
- **Simulation** : des milliers de parties entre bots. Les invariants vérifiés sont :
  - 162 points par mène (250 en cas de capot) ;
  - chaque carte est jouée exactement une fois ;
  - le score cumulé est égal à la somme des mènes.
- **Tables** : `testing/synctest` (horloge virtuelle) pour les délais, les absences, le forfait après déconnexion, la reprise depuis le journal et les pannes de stockage.
- **Redis** avec miniredis.
- **Postgres** sur une base temporaire réelle (`-tags integration`).
- **Bout en bout** : deux instances, une partie privée avec bots, une partie classée entre quatre joueurs répartis sur les deux instances, et la reprise d'une partie après l'arrêt de l'instance qui l'exécutait.

## Déploiement

Le dossier [`deploy/`](deploy) contient une stack de production prête à l'emploi : Caddy (HTTPS automatique, WebSocket, client web), deux instances du serveur, Postgres et Redis.

```bash
# Sur un serveur avec Docker, le DNS du domaine pointant vers lui :
cd deploy
cp .env.exemple .env      # domaine, mot de passe Postgres, identifiants OAuth
docker compose up -d
```

Caddy sert aussi le client web sur le même domaine : son build (le `dist/` du client) est lu dans `deploy/web`, ou dans le dossier indiqué par `COINCHE_WEB`. Les chemins de l'API (`/auth/*`, `/ws`, `/moi`, `/joueurs`, `/parties/*`, `/classement`…) vont au serveur ; toute autre route renvoie `index.html`.

La CI (GitHub Actions) lance le lint, les tests unitaires et d'intégration, puis une simulation. À chaque push sur `master` et à chaque tag `v*`, elle publie l'image `ghcr.io/felix-marchenay/coinchenet-core`. Si le paquet GHCR est privé, faire un `docker login ghcr.io` sur le serveur, ou rendre le paquet public.

Mettre à jour la production :

```bash
docker compose pull && docker compose up -d
```

Les parties en cours sont reprises depuis le journal par les nouvelles instances, et les joueurs sont reconnectés automatiquement.

Exploitation :
- **Métriques** : port 9090, non exposé (`/metrics`), à collecter depuis le réseau Docker.
- **Journaux** : JSON sur la sortie standard.
- **Sauvegardes** : `docker compose exec postgres pg_dump -U coinche coinche | gzip > coinche-$(date +%F).sql.gz`.
