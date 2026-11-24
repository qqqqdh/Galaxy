#!/usr/bin/env bash
set -euo pipefail

cd -- "$(dirname -- "$0")/.."
root=$PWD
sdk=${ANDROID_SDK_ROOT:-${ANDROID_HOME:-"$root/.build-cache/android"}}
tools="$sdk/build-tools/35.0.0"
platform="$sdk/platforms/android-35/android.jar"
clang="$sdk/ndk/28.2.13676358/toolchains/llvm/prebuilt/linux-x86_64/bin/clang"
xterm="$root/terminal/node_modules/@xterm"
cache="$root/.build-cache/terminal"
proot="$cache/ubuntu-build"
version=${GALAXY_TERMINAL_VERSION:-0.4.15}
preview=${GALAXY_PREVIEW:-0}

if [[ ! -x "$clang" || ! -x "$tools/aapt2" || ! -f "$platform" ]]; then
    printf 'Android platform 35, build-tools 35.0.0, and NDK 28.2.13676358 are required.\n' >&2
    exit 1
fi
if [[ ! -f "$xterm/xterm/lib/xterm.js" ]]; then
    printf 'Run npm --prefix terminal ci --ignore-scripts first.\n' >&2
    exit 1
fi

mkdir -p "$cache" dist
stage=$(mktemp -d "$cache/.build.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT
mkdir -p "$stage/"{assets,classes,lib}
manifest="$root/terminal/AndroidManifest.xml"
apk=Galaxy-Terminal.apk
if [[ $preview == 1 ]]; then
    manifest="$stage/AndroidManifest.xml"
    sed -e 's/package="dev.galaxy.terminal"/package="dev.galaxy.terminal.preview"/' \
        -e 's/android:label="Galaxy Terminal"/android:label="Galaxy Terminal Preview"/' \
        -e 's/android:name="\./android:name="dev.galaxy.terminal./g' \
        terminal/AndroidManifest.xml > "$manifest"
    apk=Galaxy-Terminal-Preview.apk
    if [[ -z ${GALAXY_SIGNING_KEYSTORE:-} ]]; then
        GALAXY_SIGNING_KEYSTORE="$cache/terminal-preview.jks"
        export GALAXY_SIGNING_PASSWORD=android
        GALAXY_SIGNING_ALIAS=terminal-preview
        if [[ ! -f "$GALAXY_SIGNING_KEYSTORE" ]]; then
            keytool -genkeypair -keystore "$GALAXY_SIGNING_KEYSTORE" -storepass:env GALAXY_SIGNING_PASSWORD \
                -keypass:env GALAXY_SIGNING_PASSWORD -alias "$GALAXY_SIGNING_ALIAS" -keyalg RSA -keysize 2048 \
                -validity 10000 -dname 'CN=Galaxy Terminal Preview'
        fi
    fi
fi
: "${GALAXY_SIGNING_KEYSTORE:?Set the existing APK signing keystore, or use GALAXY_PREVIEW=1}"
: "${GALAXY_SIGNING_PASSWORD:?Set the signing keystore password in the environment}"
: "${GALAXY_SIGNING_ALIAS:=terminal-dev}"

bash terminal/fetch-proot.sh
cp terminal/ui/{index.html,terminal.css,terminal.js} "$stage/assets"
for profile in galaxy.mkshrc galaxy-bashrc galaxy-w galaxy-ubuntu-setup.sh ubuntu-locales.txt; do
    sed 's/\r$//' "terminal/$profile" > "$stage/assets/$profile"
done
cp "$proot/"{PRoot-COPYING.txt,PRoot-source.tar.gz,GPL-3.txt,talloc-LGPL.txt} terminal/PRoot-NOTICE.txt "$stage/assets"
cp "$xterm/"{xterm/lib/xterm.js,xterm/css/xterm.css,addon-fit/lib/addon-fit.js,addon-web-links/lib/addon-web-links.js} "$stage/assets"
for module in xterm addon-fit addon-web-links; do
    cp "$xterm/$module/LICENSE" "$stage/assets/$module-LICENSE.txt"
done

javac -encoding UTF-8 --release 8 -classpath "$platform" -d "$stage/classes" \
    terminal/{TerminalActivity,EditorInput,PtySession,Ubuntu,UbuntuSetup,BrowserActivity,WebAddress,FileTransfer}.java
jar cf "$stage/classes.jar" -C "$stage/classes" .
"$tools/d8" --lib "$platform" --min-api 26 --output "$stage" "$stage/classes.jar"

for pair in 'arm64-v8a:aarch64-linux-android26' 'armeabi-v7a:armv7a-linux-androideabi26' 'x86_64:x86_64-linux-android26'; do
    abi=${pair%%:*}
    target=${pair#*:}
    mkdir -p "$stage/lib/$abi"
    "$clang" --target="$target" -shared -fPIC -O2 -Wall -Wextra -Werror \
        -Wl,--no-undefined,-z,max-page-size=16384 -s terminal/pty.c -o "$stage/lib/$abi/libgalaxy_pty.so"
done
"$clang" --target=aarch64-linux-android28 -static -O2 -Wall -Wextra -Werror \
    -Wl,-z,max-page-size=16384 -s terminal/ubuntu-session.c -o "$stage/lib/arm64-v8a/libgalaxy_session.so"
cp "$proot/"{libproot.so,libproot-loader.so} "$stage/lib/arm64-v8a"

"$tools/aapt2" compile --dir terminal/res -o "$stage/resources.zip"
"$tools/aapt2" link -I "$platform" --manifest "$manifest" -A "$stage/assets" "$stage/resources.zip" \
    -o "$stage/packed.apk" --version-code 21 --version-name "$version"
(cd "$stage" && zip -q -0 -r packed.apk classes.dex lib)
"$tools/zipalign" -f -P 16 4 "$stage/packed.apk" "$stage/aligned.apk"
"$tools/zipalign" -c -P 16 4 "$stage/aligned.apk"
export GALAXY_SIGNING_PASSWORD
"$tools/apksigner" sign --ks "$GALAXY_SIGNING_KEYSTORE" --ks-key-alias "$GALAXY_SIGNING_ALIAS" \
    --ks-pass env:GALAXY_SIGNING_PASSWORD --key-pass env:GALAXY_SIGNING_PASSWORD --v4-signing-enabled false \
    --out "$stage/$apk" "$stage/aligned.apk"
"$tools/apksigner" verify --verbose --print-certs "$stage/$apk"
"$tools/aapt2" dump badging "$stage/$apk"
unzip -tq "$stage/$apk"
mv -f -- "$stage/$apk" "dist/$apk"
