$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$root = Split-Path -Parent $PSScriptRoot
$cache = '.build-cache/terminal'
$sdk = '.build-cache/android'
$tools = "$sdk/build-tools/35.0.0"
$platform = "$sdk/platforms/android-35/android.jar"
$llvm = "$sdk/ndk/28.2.13676358/toolchains/llvm/prebuilt/windows-x86_64/bin"
$xterm = 'terminal/node_modules/@xterm'
$proot = "$cache/ubuntu-build"

Push-Location $root
try {
    if (!(Test-Path "$llvm/clang.exe")) { throw 'Install Android NDK 28.2.13676358 using SDK Manager.' }
    if (!(Test-Path "$xterm/xterm/lib/xterm.js") -or !(Test-Path "$xterm/addon-fit/lib/addon-fit.js")) {
        throw 'Run npm ci --ignore-scripts inside terminal first.'
    }
    & "$PSScriptRoot/fetch-proot.ps1"
    $java = javac -J-XshowSettings:properties -version 2>&1 | Out-String
    $jdk = [regex]::Match($java, '(?m)^\s*java\.home = ([^\r\n]+)').Groups[1].Value.Trim()
    if (!$jdk) { throw 'A JDK with javac, jar and keytool is required.' }
    New-Item -Force -ItemType Directory $cache, "$cache/classes", "$cache/assets", 'dist' | Out-Null
    Copy-Item 'terminal/ui/index.html','terminal/ui/terminal.css','terminal/ui/terminal.js' "$cache/assets"
    foreach ($profile in 'galaxy.mkshrc','galaxy-bashrc','galaxy-w','galaxy-ubuntu-setup.sh','ubuntu-locales.txt') {
        $content = (Get-Content -Raw -Encoding UTF8 "terminal/$profile").Replace("`r`n", "`n")
        Set-Content -LiteralPath "$cache/assets/$profile" -Value $content -Encoding utf8NoBOM -NoNewline
    }
    Copy-Item "$proot/PRoot-COPYING.txt","$proot/PRoot-source.tar.gz","$proot/GPL-3.txt","$proot/talloc-LGPL.txt",'terminal/PRoot-NOTICE.txt' "$cache/assets"
    Copy-Item "$xterm/xterm/lib/xterm.js","$xterm/xterm/css/xterm.css","$xterm/addon-fit/lib/addon-fit.js" "$cache/assets"
    Copy-Item "$xterm/xterm/LICENSE" "$cache/assets/xterm-LICENSE.txt"
    Copy-Item "$xterm/addon-fit/LICENSE" "$cache/assets/addon-fit-LICENSE.txt"

    javac -encoding UTF-8 --release 8 -classpath $platform -d "$cache/classes" terminal/TerminalActivity.java terminal/EditorInput.java terminal/PtySession.java terminal/Ubuntu.java terminal/UbuntuSetup.java terminal/BrowserActivity.java terminal/WebAddress.java
    & "$jdk/bin/jar.exe" cf "$cache/classes.jar" -C "$cache/classes" .
    & "$tools/d8.bat" --lib $platform --min-api 26 --output $cache "$cache/classes.jar"

    $targets = @{
        'arm64-v8a' = 'aarch64-linux-android26'
        'armeabi-v7a' = 'armv7a-linux-androideabi26'
        'x86_64' = 'x86_64-linux-android26'
    }
    foreach ($abi in $targets.Keys) {
        New-Item -Force -ItemType Directory "$cache/lib/$abi" | Out-Null
        & "$llvm/clang.exe" "--target=$($targets[$abi])" -shared -fPIC -O2 -Wall -Wextra -Werror `
            '-Wl,--no-undefined,-z,max-page-size=16384' -s terminal/pty.c -o "$cache/lib/$abi/libgalaxy_pty.so"
    }

    & "$llvm/clang.exe" --target=aarch64-linux-android28 -static -O2 -Wall -Wextra -Werror -s `
        '-Wl,-z,max-page-size=16384' terminal/ubuntu-session.c -o "$cache/lib/arm64-v8a/libgalaxy_session.so"
    & "$tools/aapt2.exe" compile --dir terminal/res -o "$cache/browser-res.zip"
    & "$tools/aapt2.exe" link -I $platform --manifest terminal/AndroidManifest.xml -A "$cache/assets" `
        "$cache/browser-res.zip" -o "$cache/base.apk" --version-code 15 --version-name 0.4.9
    Copy-Item -Force "$cache/base.apk" "$cache/packed.apk"
    $archive = [IO.Compression.ZipFile]::Open([IO.Path]::GetFullPath("$cache/packed.apk", $root), 'Update')
    try {
        [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, "$root/$cache/classes.dex", 'classes.dex') | Out-Null
        foreach ($abi in $targets.Keys) {
            $library = "lib/$abi/libgalaxy_pty.so"
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, "$root/$cache/$library", $library,
                [IO.Compression.CompressionLevel]::NoCompression) | Out-Null
        }
        $helper = 'lib/arm64-v8a/libgalaxy_session.so'
        [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, "$root/$cache/$helper", $helper,
            [IO.Compression.CompressionLevel]::NoCompression) | Out-Null
        foreach ($name in 'libproot.so','libproot-loader.so') {
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, "$root/$proot/$name", "lib/arm64-v8a/$name",
                [IO.Compression.CompressionLevel]::NoCompression) | Out-Null
        }
    } finally {
        $archive.Dispose()
    }
    & "$tools/zipalign.exe" -f -P 16 4 "$cache/packed.apk" "$cache/aligned.apk"
    & "$tools/zipalign.exe" -c -P 16 4 "$cache/aligned.apk"

    $key = "$cache/terminal-dev.jks"
    if (!(Test-Path $key)) {
        & "$jdk/bin/keytool.exe" -genkeypair -keystore $key -storepass android -keypass android -alias terminal-dev `
            -keyalg RSA -keysize 2048 -validity 10000 -dname 'CN=Galaxy Terminal Development'
    }
    $apk = 'Galaxy-Terminal.apk'
    & "$tools/apksigner.bat" sign --ks $key --ks-pass pass:android --key-pass pass:android --v4-signing-enabled false `
        --out "$cache/$apk" "$cache/aligned.apk"
    & "$tools/apksigner.bat" verify --verbose "$cache/$apk"
    & "$tools/aapt2.exe" dump badging "$cache/$apk" | Select-String '^(package|sdkVersion|targetSdkVersion|launchable-activity|native-code):'
    Move-Item -Force -LiteralPath "$cache/$apk" -Destination "dist/$apk"
} finally {
    Pop-Location
}
