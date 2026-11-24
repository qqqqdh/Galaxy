#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
exec bash "$root/tools/build-tool.sh" 'galaxy-servers' "$@"
