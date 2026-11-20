#!/usr/bin/env bash
set -eu
binary=${1:-$(dirname "$0")/../dist/galaxy-ide-linux-arm64}
if [[ $(uname -m) != aarch64 && $(uname -m) != arm64 ]]; then
    printf '이 배포 파일은 ARM64 Linux용입니다. 다른 플랫폼에서는 Go로 빌드하세요.\n' >&2
    exit 1
fi
if [[ ! -f $binary ]]; then
    printf '실행 파일을 찾을 수 없습니다: %s\n' "$binary" >&2
    exit 1
fi
mkdir -p "$HOME/.local/bin"
temporary=$(mktemp "$HOME/.local/bin/.galaxy-ide.XXXXXXXX")
trap 'rm -f -- "$temporary"' EXIT
# PRoot may not reproduce GNU install's copy operations, so stream into a local staging file.
cat -- "$binary" > "$temporary"
chmod 0755 "$temporary"
mv -f -- "$temporary" "$HOME/.local/bin/galaxy-ide"
"$HOME/.local/bin/galaxy-ide" --version
printf '\ngalaxy-ide 또는 galaxy-ide 프로젝트폴더로 실행하세요.\n'
case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) printf '\n셸에 다음 경로를 추가하세요.\nexport PATH="$HOME/.local/bin:$PATH"\n' ;;
esac
