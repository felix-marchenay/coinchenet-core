#!/bin/sh
# Prépare un serveur Docker au déploiement continu. En root, depuis une copie de deploy/ :
#   sh preparer-serveur.sh "<clé publique de la CI de coinchenet-core>" "<clé publique de la CI de coincheweb>"
# Relançable : réinstalle le script de déploiement, la configuration et les clés, garde le .env.
set -eu

cd "$(dirname "$0")"
dossier=/opt/coinchenet
utilisateur=deploiement
maison=/var/lib/$utilisateur

[ "$(id -u)" -eq 0 ] || { echo "à lancer en root" >&2; exit 1; }
command -v docker >/dev/null || { echo "Docker doit être installé" >&2; exit 1; }
[ $# -eq 2 ] || { echo "usage : $0 <clé CI api> <clé CI web>" >&2; exit 2; }

if ! id "$utilisateur" >/dev/null 2>&1; then
	useradd --system --create-home --home-dir "$maison" --shell /bin/sh --groups docker "$utilisateur"
fi
# Sans mot de passe, mais pas verrouillé : sshd refuse les clés d'un compte verrouillé.
usermod -p '*' "$utilisateur"

install -m 755 deployer.sh /usr/local/bin/coinchenet-deployer

install -d -o "$utilisateur" -g "$utilisateur" "$dossier" "$dossier/web/versions"
# cp et non install : le Caddyfile est monté seul, il doit garder son inode.
for f in compose.yaml Caddyfile; do
	cp "$f" "$dossier/$f"
	chmod 644 "$dossier/$f"
done
if [ ! -e "$dossier/.env" ]; then
	install -m 600 .env.exemple "$dossier/.env"
	echo "$dossier/.env créé : renseigner les domaines, le mot de passe Postgres et les identifiants OAuth."
fi
chown -R "$utilisateur:$utilisateur" "$dossier"

install -d -m 700 -o "$utilisateur" -g "$utilisateur" "$maison/.ssh"
cat >"$maison/.ssh/authorized_keys" <<EOF
restrict,command="/usr/local/bin/coinchenet-deployer api" $1
restrict,command="/usr/local/bin/coinchenet-deployer web" $2
EOF
chown "$utilisateur:$utilisateur" "$maison/.ssh/authorized_keys"
chmod 600 "$maison/.ssh/authorized_keys"

echo "Prêt. Premier lancement : cd $dossier && sudo -u $utilisateur docker compose up -d"
