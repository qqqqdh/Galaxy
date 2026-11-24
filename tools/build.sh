#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
bash "$root/file-manager/build.sh" "$@"
bash "$root/server-manager/build.sh" "$@"
cp "$root/dist/galaxy-files-LICENSES.txt" "$root/dist/galaxy-tools-LICENSES.txt"
