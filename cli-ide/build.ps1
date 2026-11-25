$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$root = Split-Path -Parent $PSScriptRoot
$cache = Join-Path $root '.build-cache'
$previous = @{}
foreach ($name in 'GOCACHE', 'GOMODCACHE', 'GOOS', 'GOARCH', 'CGO_ENABLED') {
    $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
    $env:GOCACHE = Join-Path $cache 'go-build'
    $env:GOMODCACHE = Join-Path $cache 'go-mod'
    New-Item -ItemType Directory -Force $env:GOCACHE, $env:GOMODCACHE, (Join-Path $root 'dist') | Out-Null
    Push-Location $PSScriptRoot
    try {
        $overlay = go run terminal-overlay.go
        go test -overlay $overlay -timeout 90s ./...
        go test -overlay $overlay -tags terminal_worker_experiment -run '^TestTerminal(ScrollbackOrder|ScrollRegion|EditorPartialRedraw|WorkerFrameExchange)$' -timeout 45s
        $env:GOOS = 'linux'
        $env:GOARCH = 'arm64'
        $env:CGO_ENABLED = '0'
        $output = Join-Path $root 'dist/galaxy-ide-linux-arm64'
        go build -overlay $overlay -trimpath -buildvcs=false -ldflags '-s -w' -o $output .
        Copy-Item -LiteralPath 'THIRD_PARTY_LICENSES.txt' -Destination (Join-Path $root 'dist/galaxy-ide-LICENSES.txt')
        Get-FileHash -LiteralPath $output -Algorithm SHA256
    } finally {
        Pop-Location
    }
} finally {
    foreach ($name in $previous.Keys) {
        [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
    }
}
