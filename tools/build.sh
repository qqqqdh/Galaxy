#!/usr/bin/env bash
set -euo pipefail

cd -- "$(dirname -- "$0")"
architecture=${1:-$(go env GOHOSTARCH)}
case "$architecture" in
    amd64|arm64) ;;
    *) printf '지원하지 않는 아키텍처: %s\n' "$architecture" >&2; exit 1 ;;
esac

export GOOS=linux GOARCH="$architecture" CGO_ENABLED=0
mkdir -p ../dist
for command in galaxy-files galaxy-servers; do
    go build -mod=readonly -trimpath -ldflags '-s -w' \
        -o "../dist/$command-linux-$architecture" "./cmd/$command"
done
cp ../cli-ide/THIRD_PARTY_LICENSES.txt ../dist/galaxy-tools-LICENSES.txt
