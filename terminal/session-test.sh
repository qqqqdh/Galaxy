#!/bin/bash
# Run inside the installed Ubuntu terminal with util-linux script available.
set -eu
user=$(id -un)
current=$(tty)
has_row() {
    awk -v u="$user" -v t="${1#/dev/}" '$1 == u && $2 == t { found=1 } END { exit !found }'
}
who | has_row "$current"
w | has_row "$current"
stage=$(mktemp -d)
trap 'rm -r -- "$stage"' EXIT
export GALAXY_TEST_TTY="$stage/tty"
script -qec 'exec /bin/bash --noprofile --rcfile /etc/galaxy-bashrc -ic "tty >\"\$GALAXY_TEST_TTY\"; who; w; exit"' /dev/null >"$stage/output"
second=$(cat "$GALAXY_TEST_TTY")
[[ $second != "$current" ]]
awk -v u="$user" -v t="${second#/dev/}" '$1==u && $2==t {n++} END {exit n!=2}' "$stage/output"
for attempt in {1..20}; do
    if ! who | has_row "$second"; then break; fi
    sleep 0.1
done
if who | has_row "$second" || w | has_row "$second"; then exit 1; fi
who | has_row "$current"
w | has_row "$current"
printf 'PASS %s: %s stayed open, %s logged out\n' "$user" "$current" "$second"
