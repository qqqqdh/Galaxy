#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." && pwd)
sdk=${ANDROID_SDK_ROOT:-"${ANDROID_HOME:-$root/.build-cache/android}"}
cache=${GALAXY_SDK_ARCHIVE_CACHE:-"$root/.build-cache/android-downloads"}
mkdir -p -- "$sdk" "$cache"
stage=$(mktemp -d "$sdk/.sdk-install.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT

install_package() {
    local archive=$1 hash=$2 entry=$3 destination=$4 required=$5
    if [[ -f "$sdk/$destination/$required" ]]; then return; fi
    if [[ -e "$sdk/$destination" ]]; then
        printf 'Incomplete SDK package at %s. Choose a clean ANDROID_SDK_ROOT.\n' "$sdk/$destination" >&2
        return 1
    fi

    local file="$cache/$archive"
    if [[ ! -f "$file" ]] || ! printf '%s  %s\n' "$hash" "$file" | sha256sum -c --status; then
        curl -fsSL --retry 2 "https://dl.google.com/android/repository/$archive" -o "$stage/$archive"
        printf '%s  %s\n' "$hash" "$stage/$archive" | sha256sum -c -
        mv -f -- "$stage/$archive" "$file"
    fi
    unzip -q "$file" -d "$stage"
    test -f "$stage/$entry/$required"
    mkdir -p -- "$sdk/$(dirname -- "$destination")"
    mv -T -- "$stage/$entry" "$sdk/$destination"
}

# These archives match the pinned SDK packages in the Windows build.
install_package platform-35_r02.zip 0988cacad01b38a18a47bac14a0695f246bc76c1b06c0eeb8eb0dc825ab0c8e0 \
    android-35 platforms/android-35 android.jar
install_package build-tools_r35_linux.zip bd3a4966912eb8b30ed0d00b0cda6b6543b949d5ffe00bea54c04c81e1561d88 \
    android-15 build-tools/35.0.0 aapt2
install_package android-ndk-r28c-linux.zip dfb20d396df28ca02a8c708314b814a4d961dc9074f9a161932746f815aa552f \
    android-ndk-r28c ndk/28.2.13676358 toolchains/llvm/prebuilt/linux-x86_64/bin/clang
