#!/usr/bin/env bash
set -euo pipefail
bundle_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
echo "deploy/install.sh is retained as a compatibility entry point; using initialize.sh." >&2
exec "$bundle_dir/initialize.sh" "$@"
