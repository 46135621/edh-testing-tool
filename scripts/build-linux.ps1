param(
    [string]$GOOS = "linux",
    [string]$GOARCH = "arm64",
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$distRoot = Join-Path $projectRoot "dist"
$packageName = "PowerLevel-linux-$GOARCH-$Version"
$stagingRoot = Join-Path $distRoot $packageName
$appRoot = Join-Path $stagingRoot "PowerLevel"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found in PATH. Install Go before building a release."
}

Push-Location $projectRoot
try {
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed." }
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw "Go tests failed." }

    Remove-Item $stagingRoot -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path (Join-Path $appRoot "internal") -Force | Out-Null

    $previousGOOS = $env:GOOS
    $previousGOARCH = $env:GOARCH
    $previousCGO = $env:CGO_ENABLED
    $env:GOOS = $GOOS
    $env:GOARCH = $GOARCH
    $env:CGO_ENABLED = "0"
    try {
        & go build -trimpath -ldflags "-s -w" -o (Join-Path $appRoot "powerlevel") ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw "Linux build failed." }
    } finally {
        $env:GOOS = $previousGOOS
        $env:GOARCH = $previousGOARCH
        $env:CGO_ENABLED = $previousCGO
    }

    Copy-Item (Join-Path $PSScriptRoot "release\start.ps1") (Join-Path $appRoot "internal\start.ps1")
    Copy-Item (Join-Path $PSScriptRoot "release\stop.ps1") (Join-Path $appRoot "internal\stop.ps1")
    Copy-Item (Join-Path $PSScriptRoot "release\start.cmd") (Join-Path $appRoot "启动工具.cmd")
    Copy-Item (Join-Path $PSScriptRoot "release\stop.cmd") (Join-Path $appRoot "关闭工具.cmd")
    Copy-Item (Join-Path $PSScriptRoot "release\config.env.example") (Join-Path $appRoot "config.env.example")
    Copy-Item (Join-Path $PSScriptRoot "release\USER-GUIDE.txt") (Join-Path $appRoot "README-USER.txt")

    Write-Host "Linux binary created: $(Join-Path $appRoot 'powerlevel')"
    Write-Host "Deploy the whole directory to the target, set BROWSER_PATH=/usr/bin/chromium and run ./powerlevel."
} finally {
    Pop-Location
}
