#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
command=$1
case "$command" in
    galaxy-files) component=file-manager ;;
    galaxy-servers) component=server-manager ;;
    *) printf '지원하지 않는 명령: %s\n' "$command" >&2; exit 1 ;;
esac
architecture=${2:-$(go env GOHOSTARCH)}
case "$architecture" in
    amd64|arm64) ;;
    *) printf '지원하지 않는 아키텍처: %s\n' "$architecture" >&2; exit 1 ;;
esac

export GOOS=linux GOARCH="$architecture" CGO_ENABLED=0 GOAMD64=v1 GOARM64=v8.0 GOWORK=off
mkdir -p "$root/dist"
cd "$root/$component"
go build -mod=readonly -trimpath -buildvcs=false -ldflags '-s -w' \
    -o "$root/dist/$command-linux-$architecture" .
sed '1s/Galaxy IDE/Galaxy Ubuntu tools/' "$root/cli-ide/THIRD_PARTY_LICENSES.txt" \
    > "$root/dist/$command-LICENSES.txt"
