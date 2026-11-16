#!/bin/sh
# Déploiement continu, installé en /usr/local/bin/coinchenet-deployer par preparer-serveur.sh.
#
# Chaque clé SSH de CI est restreinte à une cible (authorized_keys de l'utilisateur deploiement) :
#   restrict,command="/usr/local/bin/coinchenet-deployer api" ssh-ed25519 …
# et ne transmet que le commit à déployer. À la main : coinchenet-deployer <api|web> <commit>.
#
#   api : image ghcr.io/…:sha-<commit> ; compose.yaml et Caddyfile lus dans le dépôt à ce commit,
#         qui doit être sur master ; réglages COINCHE_* reçus de la CI sur l'entrée standard et
#         écrits dans .env. Retour à la version précédente si elle ne démarre pas.
#   web : archive tar.gz du build du client sur l'entrée standard, publiée dans web/courant.
set -eu

depot=felix-marchenay/coinchenet-core
dossier=/opt/coinchenet
versions_web_gardees=5

cible=${1:-}
commit=${SSH_ORIGINAL_COMMAND:-${2:-}}

erreur() {
	echo "coinchenet-deployer : $*" >&2
	exit 1
}

case $commit in
'' | *[!0-9a-f]*) erreur "commit invalide : « $commit »" ;;
esac
[ ${#commit} -eq 40 ] || erreur "commit invalide : « $commit »"

cd "$dossier"
exec 9>.deploiement.verrou
flock 9

deployer_api() {
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	# Réglages COINCHE_* (secrets et variables GitHub) envoyés par la CI, une ligne CLE=valeur chacun.
	if [ -n "${SSH_ORIGINAL_COMMAND:-}" ]; then
		cat >"$tmp/reglages"
	else
		: >"$tmp/reglages"
	fi
	valider_reglages "$tmp/reglages"

	comparaison=$(curl -fsS "https://api.github.com/repos/$depot/compare/$commit...master") ||
		erreur "commit $commit introuvable dans $depot"
	statut=$(printf '%s' "$comparaison" | python3 -c 'import json, sys; print(json.load(sys.stdin)["status"])')
	case $statut in
	identical | ahead) ;;
	*) erreur "$commit n'est pas sur master" ;;
	esac

	image=ghcr.io/$depot:sha-$commit
	for f in compose.yaml Caddyfile; do
		curl -fsS -o "$tmp/$f" "https://raw.githubusercontent.com/$depot/$commit/deploy/$f"
	done
	docker pull -q "$image" >/dev/null

	# cp réécrit les fichiers en place : Caddy monte son Caddyfile seul et ne verrait pas un nouvel inode.
	for f in compose.yaml Caddyfile .env; do
		cp "$f" "$tmp/$f.precedent"
	done
	cp "$tmp/compose.yaml" compose.yaml
	cp "$tmp/Caddyfile" Caddyfile
	reglage COINCHE_IMAGE "$image"
	while IFS= read -r ligne || [ -n "$ligne" ]; do
		reglage "${ligne%%=*}" "${ligne#*=}"
		echo "réglage : ${ligne%%=*}"
	done <"$tmp/reglages"

	if ! demarrer; then
		echo "coinchenet-deployer : échec du démarrage, retour à la version précédente" >&2
		for f in compose.yaml Caddyfile .env; do
			cp "$tmp/$f.precedent" "$f"
		done
		demarrer || true
		exit 1
	fi
	docker image prune -af --filter until=168h >/dev/null
}

# Les noms et valeurs sont restreints : ils finissent dans .env, interprété par Docker Compose.
# Une erreur ne cite jamais la ligne, qui peut contenir un secret.
valider_reglages() {
	n=0
	while IFS= read -r ligne || [ -n "$ligne" ]; do
		n=$((n + 1))
		case $ligne in
		COINCHE_IMAGE=*) erreur "réglage $n : COINCHE_IMAGE est fixé par le déploiement" ;;
		COINCHE_*=*) ;;
		*) erreur "réglage $n : nom attendu COINCHE_…=valeur" ;;
		esac
		case ${ligne%%=*} in
		*[!A-Z0-9_]*) erreur "réglage $n : nom invalide" ;;
		esac
		case ${ligne#*=} in
		*[!A-Za-z0-9._:/@,+=~-]*) erreur "réglage ${ligne%%=*} : valeur refusée (lettres, chiffres et ._:/@,+=~- seulement)" ;;
		esac
	done <"$1"
}

reglage() {
	if grep -q "^$1=" .env; then
		sed -i "s|^$1=.*|$1=$2|" .env
	else
		echo "$1=$2" >>.env
	fi
}

# Le rechargement couvre un Caddyfile modifié sans que le conteneur de Caddy soit recréé.
demarrer() {
	docker compose up -d --wait --wait-timeout 120 --remove-orphans &&
		docker compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile
}

deployer_web() {
	version=web/versions/$commit
	mkdir -p web/versions
	rm -rf "$version.tmp"
	mkdir "$version.tmp"
	tar -xzf - -C "$version.tmp" --no-same-owner --no-same-permissions
	[ -f "$version.tmp/index.html" ] || {
		rm -rf "$version.tmp"
		erreur "archive sans index.html"
	}
	chmod -R a+rX "$version.tmp"
	rm -rf "$version"
	mv "$version.tmp" "$version"
	ln -sfn "versions/$commit" web/courant.tmp
	mv -T web/courant.tmp web/courant

	# shellcheck disable=SC2012 # noms de dossiers : des commits en hexadécimal
	ls -1t web/versions | tail -n +$((versions_web_gardees + 1)) | while read -r v; do
		rm -rf "web/versions/$v"
	done
}

case $cible in
api) deployer_api ;;
web) deployer_web ;;
*) erreur "cible inconnue : « $cible » (api ou web)" ;;
esac
echo "déployé : $cible $commit"
