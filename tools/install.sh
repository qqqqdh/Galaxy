#!/usr/bin/env bash
set -euo pipefail

case "$(uname -m)" in
    aarch64|arm64) architecture=arm64 ;;
    x86_64) architecture=amd64 ;;
    *) printf '지원하지 않는 Linux 아키텍처입니다.\n' >&2; exit 1 ;;
esac
tools=$(cd -- "$(dirname -- "$0")" && pwd)
source_dir=${1:-"$tools/../dist"}
destination=${GALAXY_BIN_DIR:-"$HOME/.local/bin"}
commands=(galaxy-files galaxy-servers)
for command in "${commands[@]}"; do
    if [[ ! -f "$source_dir/$command-linux-$architecture" ]]; then
        printf '빌드 파일이 없습니다: %s\n' "$source_dir/$command-linux-$architecture" >&2
        printf '저장소에서 bash tools/build.sh를 실행하세요.\n' >&2
        exit 1
    fi
done
mkdir -p -- "$destination"
for command in "${commands[@]}"; do
    stage=$(mktemp "$destination/.$command.XXXXXXXX")
    trap 'rm -f -- "$stage"' EXIT
    cat -- "$source_dir/$command-linux-$architecture" > "$stage"
    chmod 0755 "$stage"
    mv -fT -- "$stage" "$destination/$command"
    trap - EXIT
done
printf '설치 완료 · %s\ngalaxy-files [폴더] 또는 galaxy-servers [프로젝트 폴더]\n' "$destination"
