param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$distRoot = Join-Path $projectRoot "dist"
$packageName = "PowerLevel-windows-amd64-$Version"
$stagingRoot = Join-Path $distRoot $packageName
$appRoot = Join-Path $stagingRoot "PowerLevel"
$zipPath = Join-Path $distRoot "$packageName.zip"
$hashPath = "$zipPath.sha256"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found in PATH. Install Go before building a release."
}

Push-Location $projectRoot
try {
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw "Go tests failed." }

    Remove-Item $stagingRoot -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $zipPath, $hashPath -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path (Join-Path $appRoot "internal") -Force | Out-Null

    $previousGOOS = $env:GOOS
    $previousGOARCH = $env:GOARCH
    $previousCGO = $env:CGO_ENABLED
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    try {
        & go build -trimpath -ldflags "-s -w" -o (Join-Path $appRoot "PowerLevel.exe") ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw "Windows build failed." }
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

    Compress-Archive -Path $appRoot -DestinationPath $zipPath -CompressionLevel Optimal
    $hash = (Get-FileHash -Algorithm SHA256 $zipPath).Hash.ToLowerInvariant()
    Set-Content -Path $hashPath -Value "$hash  $([IO.Path]::GetFileName($zipPath))" -Encoding ascii
    Write-Host "Release created: $zipPath"
    Write-Host "SHA-256: $hash"
} finally {
    Pop-Location
}
