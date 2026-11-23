#!/usr/bin/env bash
set -eu
profile=$(realpath "${1:-$(dirname "$0")/galaxy-bashrc}")
scratch=$(mktemp -d)
trap 'rm -rf -- "$scratch"' EXIT
mkdir "$scratch/bin" "$scratch/home"
cat > "$scratch/bin/codex" <<'EOF'
#!/bin/bash
printf '<%s>\n' "$@"
exit "${CODEX_TEST_EXIT:-0}"
EOF
chmod +x "$scratch/bin/codex"
env HOME="$scratch/home" PATH="$scratch/bin:$PATH" GALAXY_TEST_PROFILE="$profile" \
    /bin/bash --noprofile --norc <<'EOF'
set -eu
PS1= PS2=
. "$GALAXY_TEST_PROFILE"
prefix=$'<-c>\n<tui.disable_paste_burst=true>\n<--no-daemon>'
[[ $(codex) == "$prefix" ]]
[[ $(codex resume --last) == "$prefix"$'\n<resume>\n<--last>' ]]
[[ $(codex fork session-id "two words") == "$prefix"$'\n<fork>\n<session-id>\n<two words>' ]]
[[ $(codex --no-daemon resume --last) == "$prefix"$'\n<resume>\n<--last>' ]]
[[ $(codex -- --no-daemon) == "$prefix"$'\n<-->\n<--no-daemon>' ]]
status=0
CODEX_TEST_EXIT=23 codex --version >/dev/null || status=$?
[[ $status == 23 ]]
status=0
CODEX_TEST_EXIT=24 codex --no-daemon --version >/dev/null || status=$?
[[ $status == 24 ]]
printf "Passed Codex terminal defaults, argument forwarding and exit status.\n"
EOF
