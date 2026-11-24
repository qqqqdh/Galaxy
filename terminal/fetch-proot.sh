#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
cache="$root/.build-cache/terminal/ubuntu-build"
commit=7029b1fe8fa84d64339cabe38f9c5be2ea2ae7ac
pr="https://raw.githubusercontent.com/oonid/pr/$commit"
bin="$pr/android/proot-engine/src/main/jniLibs/arm64-v8a"
samba='https://raw.githubusercontent.com/samba-team/samba/2f8dfde1210395175e726455bdb63a7b97245a72'
spdx='https://raw.githubusercontent.com/spdx/license-list-data/v3.26.0/text'

mkdir -p "$cache"
stage=$(mktemp -d "$cache/.fetch.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT

fetch() {
    local name=$1 url=$2 checksum=$3
    if [[ -f "$cache/$name" ]] && printf '%s  %s\n' "$checksum" "$cache/$name" | sha256sum -c --status; then return; fi
    curl -fsSL --retry 2 --max-time 90 "$url" -o "$stage/$name"
    printf '%s  %s\n' "$checksum" "$stage/$name" | sha256sum -c -
    mv -f -- "$stage/$name" "$cache/$name"
}

fetch libproot.so "$bin/libproot.so" 9a964b4e5d42e80ca6891ce661f552a753feb4837e4cc5ecaeb7f0125b5cbfeb
fetch libproot-loader.so "$bin/libproot-loader.so" e427c4fac91d18a197e6624e5907b408b1868f86de9022f06e8f39448b991fe1
fetch PRoot-COPYING.txt "$pr/src/proot/COPYING" 8177f97513213526df2cf6184d8ff986c675afb514d4e68a404010521b880643
fetch upstream-pr.tar.gz "https://codeload.github.com/oonid/pr/tar.gz/$commit" 9a488aa92b3e38e6c136a37c6e78a5dd3fe19bb0aeda78503663fe608b6947d3
fetch talloc.c "$samba/lib/talloc/talloc.c" 28c1f23f4efe21c785c3794a15916c671bd18f55be5c56813c46cda1e6fa0738
fetch talloc.h "$samba/lib/talloc/talloc.h" 42b0ad93555c28d2177f016b1f73ebdb848dbc36e0c5cdab21786dee4341067f
fetch GPL-3.txt "$samba/COPYING" 8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903
fetch talloc-LGPL.txt "$spdx/LGPL-3.0-or-later.txt" 996af0513df21f7496288951c41428a03c174e9e4a9d63665c57d670f845ccb1

source_dir="$stage/source"
vendor="$source_dir/vendor/samba/lib/talloc"
mkdir -p "$vendor"
tar -xf "$cache/upstream-pr.tar.gz" -C "$source_dir" --strip-components=1 \
    "pr-$commit/src/proot" "pr-$commit/scripts/build.sh"
cp "$cache/"{talloc.c,talloc.h,talloc-LGPL.txt} "$vendor"
cp "$root/terminal/PRoot-NOTICE.txt" "$cache/GPL-3.txt" "$source_dir"
tar -czf "$stage/PRoot-source.tar.gz" -C "$source_dir" PRoot-NOTICE.txt GPL-3.txt scripts src vendor
mv -f -- "$stage/PRoot-source.tar.gz" "$cache/PRoot-source.tar.gz"
