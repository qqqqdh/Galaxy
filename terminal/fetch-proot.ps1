$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$cache = Join-Path (Split-Path -Parent $PSScriptRoot) '.build-cache/terminal/ubuntu-build'
$commit = '7029b1fe8fa84d64339cabe38f9c5be2ea2ae7ac'
$pr = "https://raw.githubusercontent.com/oonid/pr/$commit"
$bin = "$pr/android/proot-engine/src/main/jniLibs/arm64-v8a"
$samba = 'https://raw.githubusercontent.com/samba-team/samba/2f8dfde1210395175e726455bdb63a7b97245a72'
$spdx = 'https://raw.githubusercontent.com/spdx/license-list-data/v3.26.0/text'

function Get-VerifiedFile($name, $url, $hash) {
    $path = Join-Path $cache $name
    if ((Test-Path -LiteralPath $path) -and (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash -eq $hash) { return }
    $partial = "$path.partial"
    try {
        curl.exe --fail --location --silent --show-error --max-time 60 --output $partial $url
        if ((Get-FileHash -Algorithm SHA256 -LiteralPath $partial).Hash -ne $hash) { throw "Invalid SHA-256 for $name." }
        Move-Item -Force -LiteralPath $partial -Destination $path
    } finally {
        if (Test-Path -LiteralPath $partial) { Remove-Item -LiteralPath $partial }
    }
}

New-Item -ItemType Directory -Force -Path $cache | Out-Null
Get-VerifiedFile 'libproot.so' "$bin/libproot.so" '9a964b4e5d42e80ca6891ce661f552a753feb4837e4cc5ecaeb7f0125b5cbfeb'
Get-VerifiedFile 'libproot-loader.so' "$bin/libproot-loader.so" 'e427c4fac91d18a197e6624e5907b408b1868f86de9022f06e8f39448b991fe1'
Get-VerifiedFile 'PRoot-COPYING.txt' "$pr/src/proot/COPYING" '8177f97513213526df2cf6184d8ff986c675afb514d4e68a404010521b880643'
Get-VerifiedFile 'upstream-pr.tar.gz' "https://codeload.github.com/oonid/pr/tar.gz/$commit" '9a488aa92b3e38e6c136a37c6e78a5dd3fe19bb0aeda78503663fe608b6947d3'
Get-VerifiedFile 'talloc.c' "$samba/lib/talloc/talloc.c" '28c1f23f4efe21c785c3794a15916c671bd18f55be5c56813c46cda1e6fa0738'
Get-VerifiedFile 'talloc.h' "$samba/lib/talloc/talloc.h" '42b0ad93555c28d2177f016b1f73ebdb848dbc36e0c5cdab21786dee4341067f'
Get-VerifiedFile 'GPL-3.txt' "$samba/COPYING" '8ceb4b9ee5adedde47b31e975c1d90c73ad27b6b165a1dcd80c7c545eb65b903'
Get-VerifiedFile 'talloc-LGPL.txt' "$spdx/LGPL-3.0-or-later.txt" '996af0513df21f7496288951c41428a03c174e9e4a9d63665c57d670f845ccb1'

$sourceDir = Join-Path $cache 'source'
$tallocDir = Join-Path $sourceDir 'vendor/samba/lib/talloc'
$prefix = "pr-$commit"
New-Item -ItemType Directory -Force -Path $tallocDir | Out-Null
tar.exe -xf "$cache/upstream-pr.tar.gz" -C $sourceDir --strip-components 1 "$prefix/src/proot" "$prefix/scripts/build.sh"
Copy-Item "$cache/talloc.c","$cache/talloc.h","$cache/talloc-LGPL.txt" $tallocDir
Copy-Item "$PSScriptRoot/PRoot-NOTICE.txt","$cache/GPL-3.txt" $sourceDir
tar.exe -czf "$cache/PRoot-source.tar.gz" -C $sourceDir PRoot-NOTICE.txt GPL-3.txt scripts src vendor
