#!/bin/sh
# Déploiement continu, installé en /usr/local/bin/coinchenet-deployer par preparer-serveur.sh.
#
# Chaque clé SSH de CI est restreinte à une cible (authorized_keys de l'utilisateur deploiement) :
#   restrict,command="/usr/local/bin/coinchenet-deployer api" ssh-ed25519 …
# et ne transmet que le commit à déployer. À la main : coinchenet-deployer <api|web> <commit>.
#
#   api : image ghcr.io/…:sha-<commit> ; compose.yaml et Caddyfile lus dans le dépôt à ce commit,
#         qui doit être sur master. Retour à la version précédente si elle ne démarre pas.
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
	comparaison=$(curl -fsS "https://api.github.com/repos/$depot/compare/$commit...master") ||
		erreur "commit $commit introuvable dans $depot"
	statut=$(printf '%s' "$comparaison" | python3 -c 'import json, sys; print(json.load(sys.stdin)["status"])')
	case $statut in
	identical | ahead) ;;
	*) erreur "$commit n'est pas sur master" ;;
	esac

	image=ghcr.io/$depot:sha-$commit
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	for f in compose.yaml Caddyfile; do
		curl -fsS -o "$tmp/$f" "https://raw.githubusercontent.com/$depot/$commit/deploy/$f"
	done
	docker pull -q "$image" >/dev/null

	cp compose.yaml "$tmp/compose.yaml.precedent"
	cp Caddyfile "$tmp/Caddyfile.precedent"
	precedente=$(sed -n 's/^COINCHE_IMAGE=//p' .env)

	installer "$tmp/compose.yaml" "$tmp/Caddyfile" "$image"
	if ! demarrer; then
		echo "coinchenet-deployer : échec du démarrage, retour à ${precedente:-image par défaut}" >&2
		installer "$tmp/compose.yaml.precedent" "$tmp/Caddyfile.precedent" "$precedente"
		demarrer || true
		exit 1
	fi
	docker image prune -af --filter until=168h >/dev/null
}

# cp réécrit les fichiers en place : Caddy monte son Caddyfile seul et ne verrait pas un nouvel inode.
installer() {
	cp "$1" compose.yaml
	cp "$2" Caddyfile
	if grep -q '^COINCHE_IMAGE=' .env; then
		sed -i "s|^COINCHE_IMAGE=.*|COINCHE_IMAGE=$3|" .env
	else
		echo "COINCHE_IMAGE=$3" >>.env
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
