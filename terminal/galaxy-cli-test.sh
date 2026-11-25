#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -f -- "$work/bin/galaxy-doctor" "$work/bin/go" "$work/bin/gopls" "$work/project/go.mod" "$work/project/sample.go"; rmdir -- "$work/bin" "$work/project" "$work"' EXIT
mkdir "$work/bin" "$work/project"
cp "$root/galaxy-cli" "$work/bin/galaxy-doctor"
printf 'module example.com/sample\n' >"$work/project/go.mod"
printf 'package sample\n' >"$work/project/sample.go"
cat >"$work/bin/go" <<'EOF'
#!/usr/bin/env bash
[[ $GOTOOLCHAIN == local && $GOPROXY == off && $GOFLAGS == -mod=readonly ]] || exit 2
case "$1" in
    version) echo 'go version test' ;;
    env) printf '%s/go.mod\n' "$PWD" ;;
    list) [[ $2 == ./... ]] && echo 'example.com/sample' ;;
    *) exit 2 ;;
esac
EOF
cat >"$work/bin/gopls" <<'EOF'
#!/usr/bin/env bash
if [[ $1 == version ]]; then echo 'gopls test'; exit 0; fi
[[ $1 == check && -f $2 ]] || exit 2
if [[ ${FAIL_GOPLS:-} == 1 ]]; then echo 'test workspace error'; exit 1; fi
EOF
chmod +x "$work/bin/"*
export PATH="$work/bin:$PATH"
ok=$(galaxy-doctor "$work/project")
[[ $ok == *'패키지 읽기 · 성공'* && $ok == *'gopls · ./sample.go 검사 성공'* ]]
if failed=$(FAIL_GOPLS=1 galaxy-doctor "$work/project"); then
    printf 'galaxy-doctor did not report the workspace failure\n' >&2
    exit 1
fi
[[ $failed == *'gopls · ./sample.go 검사 실패'* && $failed == *'test workspace error'* ]]
printf 'Passed offline Go module and gopls diagnostics.\n'
