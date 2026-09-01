#!/usr/bin/env bash
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
  echo "Run as root: sudo $0 RELEASE_DIRECTORY" >&2
  exit 1
fi
if [[ $# -ne 1 || ! -d $1 ]]; then
  echo "Usage: sudo $0 RELEASE_DIRECTORY" >&2
  exit 2
fi

release_id=$(date -u +%Y%m%dT%H%M%SZ)
release_dir=/opt/ekkoplayer/releases/$release_id
install -d -o root -g root "$release_dir"
cp -a "$1"/. "$release_dir"/
test -x "$release_dir/ekkoplayer"
test -f "$release_dir/player/index.html"
test -f "$release_dir/admin/index.html"
ln -sfn "$release_dir" /opt/ekkoplayer/current.new
mv -Tf /opt/ekkoplayer/current.new /opt/ekkoplayer/current
ln -sfn /opt/ekkoplayer/current/ekkoplayer /usr/local/bin/ekkoplayer
systemctl restart playerd nginx
curl --fail --retry 10 --retry-connrefused --retry-delay 1 http://127.0.0.1:9091/api/v1/health
