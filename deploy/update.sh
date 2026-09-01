#!/usr/bin/env bash
set -Eeuo pipefail
umask 027

db=$(jq -er '.database_path' /etc/ekkoplayer/player.json)
state_dir=$(dirname "$(dirname "$db")")/update
request=$state_dir/request.json
status=$state_dir/status.json
work=$(mktemp -d /var/tmp/ekkoplayer-update.XXXXXX)
old_release=$(readlink -f /opt/ekkoplayer/current)
db_backup=$work/player.db

write_status(){ local state=$1 target=$2 message=$3 tmp;tmp=$(mktemp "$state_dir/status.XXXXXX");jq -n --arg state "$state" --arg current "$(cat /etc/ekkoplayer/installed-version 2>/dev/null || echo unknown)" --arg target "$target" --arg message "$message" --arg updated_at "$(date -u +%FT%TZ)" '{state:$state,current_version:$current,target_version:$target,message:$message,updated_at:$updated_at}' > "$tmp";chmod 0640 "$tmp";chown root:ekkoplayer "$tmp";mv "$tmp" "$status";}
cleanup(){ rm -rf "$work"; }
trap cleanup EXIT
[[ -f $request ]] || exit 0
version=$(jq -er '.Version|select(test("^[0-9]+\\.[0-9]+\\.[0-9]+([.-][A-Za-z0-9.-]+)?$"))' "$request")
asset_url=$(jq -er '.AssetURL|select(startswith("https://github.com/rickcollette/ekkoplay/releases/download/"))' "$request")
checksum_url=$(jq -er '.ChecksumURL|select(startswith("https://github.com/rickcollette/ekkoplay/releases/download/"))' "$request")
mv "$request" "$state_dir/request.processing.json"
write_status downloading "$version" "Downloading and verifying release"
curl --fail --location --retry 3 --connect-timeout 10 --max-time 600 -o "$work/release.tar.gz" "$asset_url"
curl --fail --location --retry 3 --connect-timeout 10 --max-time 60 -o "$work/release.sha256" "$checksum_url"
expected=$(awk 'NR==1{print $1}' "$work/release.sha256")
[[ $expected =~ ^[0-9a-fA-F]{64}$ ]] || { write_status failed "$version" "Invalid release checksum";exit 1; }
actual=$(sha256sum "$work/release.tar.gz"|awk '{print $1}')
[[ ${actual,,} == ${expected,,} ]] || { write_status failed "$version" "Release checksum mismatch";exit 1; }
if tar -tzf "$work/release.tar.gz" | grep -Eq '(^/|(^|/)\.\.(/|$))';then write_status failed "$version" "Unsafe release archive";exit 1;fi
tar -xzf "$work/release.tar.gz" -C "$work"
bundle=$(find "$work" -mindepth 1 -maxdepth 1 -type d -name 'ekkoplayer_*' -print -quit)
[[ -n $bundle && -x $bundle/ekkoplayer && -f $bundle/player/index.html && -f $bundle/admin/index.html ]] || { write_status failed "$version" "Incomplete release bundle";exit 1; }
case "$(uname -m)" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; *) write_status failed "$version" "Unsupported architecture";exit 1;;esac
[[ $(<"$bundle/ARCH") == "$arch" && $(<"$bundle/VERSION") == "$version" ]] || { write_status failed "$version" "Release identity mismatch";exit 1; }
release_dir="/opt/ekkoplayer/releases/${version}-$(date -u +%Y%m%dT%H%M%SZ)"
write_status installing "$version" "Backing up data and activating release"
systemctl start ekkoplayer-backup.service
systemctl stop playerd.service
[[ ! -f $db ]] || cp --reflink=auto --sparse=always "$db" "$db_backup"
install -d -o root -g root "$release_dir"
cp -a "$bundle/ekkoplayer" "$bundle/player" "$bundle/admin" "$release_dir/"
chmod 0755 "$release_dir/ekkoplayer"
ln -sfn "$release_dir" /opt/ekkoplayer/current.new;mv -Tf /opt/ekkoplayer/current.new /opt/ekkoplayer/current
ln -sfn /opt/ekkoplayer/current/ekkoplayer /usr/local/bin/ekkoplayer
systemctl restart playerd.service nginx
if curl --fail --retry 20 --retry-connrefused --retry-delay 1 --max-time 5 http://127.0.0.1:9091/api/v1/health >/dev/null;then
  printf '%s\n' "$version" > /etc/ekkoplayer/installed-version
  install -m 0755 "$bundle/deploy/update.sh" /usr/local/libexec/ekkoplayer-update
  install -m 0644 "$bundle/deploy/systemd/ekkoplayer-update.service" /etc/systemd/system/ekkoplayer-update.service
  systemctl daemon-reload
  write_status complete "$version" "Update installed successfully"
  rm -f "$state_dir/request.processing.json"
  exit 0
fi
write_status rolling_back "$version" "Health check failed; restoring previous release"
ln -sfn "$old_release" /opt/ekkoplayer/current.new;mv -Tf /opt/ekkoplayer/current.new /opt/ekkoplayer/current
[[ ! -f $db_backup ]] || install -o ekkoplayer -g ekkoplayer -m 0640 "$db_backup" "$db"
systemctl restart playerd.service nginx
write_status failed "$version" "Update failed and was rolled back"
exit 1
