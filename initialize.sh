#!/usr/bin/env bash
set -Eeuo pipefail
umask 027

fail(){ echo "initialize: $*" >&2; exit 1; }
[[ ${EUID:-$(id -u)} -eq 0 ]] || fail "run as root: sudo ./initialize.sh CONFIG"
[[ $# -eq 1 && -f $1 ]] || fail "usage: sudo ./initialize.sh CONFIG"
bundle_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
install_config=$(readlink -f -- "$1")
[[ -x "$bundle_dir/ekkoplayer" ]] || fail "bundle is missing ekkoplayer"
"$bundle_dir/ekkoplayer" install validate "$install_config"

. /etc/os-release
case "${ID:-}:${VERSION_ID:-}" in debian:12|debian:13|ubuntu:22.04|ubuntu:24.04) ;; *) fail "supported systems: Debian 12/13 or Ubuntu 22.04/24.04";; esac
case "$(uname -m)" in x86_64) host_arch=amd64;; aarch64|arm64) host_arch=arm64;; *) fail "unsupported CPU architecture: $(uname -m)";; esac
[[ ! -f "$bundle_dir/ARCH" || $(<"$bundle_dir/ARCH") == "$host_arch" ]] || fail "this bundle targets $(<"$bundle_dir/ARCH"), not $host_arch"

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y nginx mpv ffmpeg alsa-utils libchromaprint-tools transmission-daemon curl ca-certificates jq
systemctl disable --now transmission-daemon.service 2>/dev/null || true

room_name=$(jq -r '.room_name' "$install_config"); http_port=$(jq -r '.http_port' "$install_config")
data_path=$(jq -r '.paths.data' "$install_config"); music_path=$(jq -r '.paths.music' "$install_config"); import_path=$(jq -r '.paths.imports' "$install_config")
art_path=$(jq -r '.paths.artwork' "$install_config"); backup_path=$(jq -r '.paths.backups' "$install_config"); torrent_path=$(jq -r '.paths.torrents' "$install_config")
audio_device=$(jq -r '.audio_device' "$install_config"); audio_filter=$(jq -r '.audio_filter' "$install_config")
default_volume=$(jq -r '.default_volume' "$install_config"); maximum_volume=$(jq -r '.maximum_volume' "$install_config"); workers=$(jq -r '.import_workers' "$install_config")
acoustid=$(jq -r '.acoustid_key' "$install_config"); peer_port=$(jq -r '.torrent_peer_port' "$install_config"); seed_days=$(jq -r '.torrent_seed_days' "$install_config")
tls_enabled=$(jq -r '.tls.enabled' "$install_config"); tls_cert=$(jq -r '.tls.certificate' "$install_config"); tls_key=$(jq -r '.tls.private_key' "$install_config")
admin_username=$(jq -r '.admin_username // ""' "$install_config"); admin_password=$(jq -r '.admin_password // ""' "$install_config")
audio_outputs=$(jq -c '.audio_outputs // []' "$install_config")

if command -v ss >/dev/null && ss -Hln "sport = :$http_port" | grep -q . && ! systemctl is-active --quiet nginx; then fail "port $http_port is already in use"; fi
device_name=${audio_device#alsa/}
if [[ $device_name != default ]] && ! aplay -L 2>/dev/null | grep -Fxq "$device_name"; then fail "ALSA device is unavailable: $audio_device"; fi
while IFS= read -r configured_device;do device_name=${configured_device#alsa/};if [[ $device_name != default ]]&&! aplay -L 2>/dev/null|grep -Fxq "$device_name";then fail "ALSA output is unavailable: $configured_device";fi;done < <(jq -r '.audio_outputs[]? | select(.enabled) | .device' "$install_config")

id -u ekkoplayer >/dev/null 2>&1 || useradd --system --home-dir "$data_path" --shell /usr/sbin/nologin ekkoplayer
usermod -a -G audio ekkoplayer
install -d -o root -g ekkoplayer -m 0750 /etc/ekkoplayer
install -d -o root -g root -m 0755 /opt/ekkoplayer/releases
for path in "$data_path" "$music_path" "$import_path" "$art_path" "$backup_path" "$torrent_path" "$data_path/database" "$data_path/tmp" "$data_path/update"; do install -d -o ekkoplayer -g ekkoplayer -m 0750 "$path"; done
if [[ ! -f /etc/ekkoplayer/jwt.key ]]; then head -c 64 /dev/urandom > /etc/ekkoplayer/jwt.key; chown root:ekkoplayer /etc/ekkoplayer/jwt.key; chmod 0640 /etc/ekkoplayer/jwt.key; fi

runtime_tmp=$(mktemp /etc/ekkoplayer/player.json.XXXXXX)
jq -n --arg room "$room_name" --arg music "$music_path" --arg imports "$import_path" --arg art "$art_path" --arg backups "$backup_path" --arg torrents "$torrent_path" --arg device "$audio_device" --arg filter "$audio_filter" --arg acoustid "$acoustid" --argjson outputs "$audio_outputs" --argjson dv "$default_volume" --argjson mv "$maximum_volume" --argjson workers "$workers" --argjson peer "$peer_port" --argjson days "$seed_days" --argjson secure "$tls_enabled" '{listen:"127.0.0.1:9091",database_path:($ARGS.named.music|sub("/music$";"/database/player.db")),music_path:$music,import_path:$imports,artwork_path:$art,backup_path:$backups,mpv_binary:"/usr/bin/mpv",mpv_socket:"/run/ekkoplayer/mpv.sock",audio_device:$device,audio_filter:$filter,audio_outputs:$outputs,default_volume:$dv,maximum_volume:$mv,start_mpv:true,seed_demo:false,ffmpeg_binary:"/usr/bin/ffmpeg",ffprobe_binary:"/usr/bin/ffprobe",max_upload_bytes:17179869184,import_workers:$workers,acoustid_key:$acoustid,fpcalc_binary:"/usr/bin/fpcalc",torrent_binary:"/usr/bin/transmission-daemon",torrent_path:$torrents,torrent_rpc_url:"http://127.0.0.1:9092/transmission/rpc",torrent_peer_port:$peer,torrent_seed_days:$days,jwt_secret_path:"/etc/ekkoplayer/jwt.key",cookie_secure:$secure,room_name:$room,update_repository:"rickcollette/ekkoplay"}' > "$runtime_tmp"
# The database always lives under the durable data root, even when music is mounted elsewhere.
jq --arg db "$data_path/database/player.db" '.database_path=$db' "$runtime_tmp" > "${runtime_tmp}.fixed" && mv "${runtime_tmp}.fixed" "$runtime_tmp"
chown root:ekkoplayer "$runtime_tmp"; chmod 0640 "$runtime_tmp"

version=$(<"$bundle_dir/VERSION"); release_dir="/opt/ekkoplayer/releases/${version}-$(date -u +%Y%m%dT%H%M%SZ)"
old_release=$(readlink -f /opt/ekkoplayer/current 2>/dev/null || true)
old_runtime=$(mktemp);had_runtime=false;if [[ -f /etc/ekkoplayer/player.json ]];then cp -a /etc/ekkoplayer/player.json "$old_runtime";had_runtime=true;fi
old_nginx=$(mktemp);had_nginx=false;if [[ -f /etc/nginx/sites-available/ekkoplayer ]];then cp -a /etc/nginx/sites-available/ekkoplayer "$old_nginx";had_nginx=true;fi
password_file=""
rollback(){ echo "Initialization failed; restoring previous release." >&2; systemctl stop playerd.service 2>/dev/null || true; if [[ -n $old_release ]];then ln -sfn "$old_release" /opt/ekkoplayer/current.new;mv -Tf /opt/ekkoplayer/current.new /opt/ekkoplayer/current;fi;if $had_runtime;then cp -a "$old_runtime" /etc/ekkoplayer/player.json;else rm -f /etc/ekkoplayer/player.json;fi;if $had_nginx;then cp -a "$old_nginx" /etc/nginx/sites-available/ekkoplayer;else rm -f /etc/nginx/sites-available/ekkoplayer;fi;[[ -z $password_file ]]||rm -f "$password_file"; }
trap rollback ERR
install -d -o root -g root "$release_dir"
cp -a "$bundle_dir/ekkoplayer" "$bundle_dir/player" "$bundle_dir/admin" "$release_dir/"
chmod 0755 "$release_dir/ekkoplayer"
ln -sfn "$release_dir" /opt/ekkoplayer/current.new; mv -Tf /opt/ekkoplayer/current.new /opt/ekkoplayer/current
mv "$runtime_tmp" /etc/ekkoplayer/player.json

install -m 0644 "$bundle_dir/deploy/systemd/playerd.service" /etc/systemd/system/playerd.service
install -m 0644 "$bundle_dir/deploy/systemd/ekkoplayer-backup.service" /etc/systemd/system/ekkoplayer-backup.service
install -m 0644 "$bundle_dir/deploy/systemd/ekkoplayer-backup.timer" /etc/systemd/system/ekkoplayer-backup.timer
install -m 0644 "$bundle_dir/deploy/systemd/ekkoplayer-update.service" /etc/systemd/system/ekkoplayer-update.service
install -m 0644 "$bundle_dir/deploy/systemd/ekkoplayer-update.path" /etc/systemd/system/ekkoplayer-update.path
install -D -m 0755 "$bundle_dir/deploy/update.sh" /usr/local/libexec/ekkoplayer-update
sed -i "s|^PathChanged=.*|PathChanged=$data_path/update/request.json|" /etc/systemd/system/ekkoplayer-update.path
sed -i "s|^ReadWritePaths=.*|ReadWritePaths=$data_path $music_path $import_path $art_path $backup_path $torrent_path /run/ekkoplayer|" /etc/systemd/system/playerd.service
sed -i "s|^ReadWritePaths=.*|ReadWritePaths=$data_path $backup_path|" /etc/systemd/system/ekkoplayer-backup.service

nginx_tmp=$(mktemp /etc/nginx/sites-available/ekkoplayer.XXXXXX)
listen_line="listen 0.0.0.0:$http_port;"; tls_lines=""
if [[ $tls_enabled == true ]]; then listen_line="listen 0.0.0.0:$http_port ssl;";tls_lines=$(printf 'ssl_certificate %s;\n    ssl_certificate_key %s;' "$tls_cert" "$tls_key");fi
sed -e "s|@@LISTEN@@|$listen_line|" -e "s|@@TLS@@|$tls_lines|" -e "s|@@ARTWORK@@|$art_path/|" "$bundle_dir/deploy/nginx/ekkoplayer.conf.template" > "$nginx_tmp"
chmod 0644 "$nginx_tmp";mv "$nginx_tmp" /etc/nginx/sites-available/ekkoplayer
ln -sfn /etc/nginx/sites-available/ekkoplayer /etc/nginx/sites-enabled/ekkoplayer;rm -f /etc/nginx/sites-enabled/default
nginx -t

export EKKOPLAYER_CONFIG=/etc/ekkoplayer/player.json
if ! /opt/ekkoplayer/current/ekkoplayer admin exists >/dev/null 2>&1; then
  [[ -n $admin_username && -n $admin_password ]] || fail "bootstrap admin_username and admin_password are required because no administrator exists"
  password_file=$(mktemp);chmod 0600 "$password_file";printf '%s' "$admin_password" > "$password_file"
  /opt/ekkoplayer/current/ekkoplayer admin create --username "$admin_username" --password-file "$password_file"
  rm -f "$password_file"
fi
unset admin_password

systemctl daemon-reload;systemctl enable playerd.service nginx ekkoplayer-backup.timer ekkoplayer-update.path
systemctl restart playerd.service nginx;systemctl restart ekkoplayer-backup.timer
curl --fail --retry 15 --retry-connrefused --retry-delay 1 http://127.0.0.1:9091/api/v1/health >/dev/null
curl --fail http://127.0.0.1:9091/api/v1/version >/dev/null
systemctl is-active --quiet playerd.service nginx ekkoplayer-backup.timer ekkoplayer-update.path
[[ $(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:9092/transmission/rpc) =~ ^(200|409)$ ]] || fail "Transmission RPC check failed"

redacted=$(mktemp "$(dirname "$install_config")/.ekkoplayer-config.XXXXXX");jq 'del(.admin_password)' "$install_config" > "$redacted";chmod --reference="$install_config" "$redacted";chown --reference="$install_config" "$redacted";mv "$redacted" "$install_config"
printf '%s\n' "$version" > /etc/ekkoplayer/installed-version;chmod 0644 /etc/ekkoplayer/installed-version
trap - ERR
rm -f "$old_runtime" "$old_nginx"
if [[ $tls_enabled != true ]];then echo "WARNING: Admin credentials use trusted-LAN HTTP. Enable TLS before exposing this service." >&2;fi
echo "ekkoPlayer $version is installed and healthy on port $http_port."
