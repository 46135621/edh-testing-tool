param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$distRoot = Join-Path $projectRoot "dist"
$stagingRoot = Join-Path $distRoot "PowerLevel-mobile-$Version"
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
    New-Item -ItemType Directory -Path $appRoot -Force | Out-Null

    $previousGOOS = $env:GOOS
    $previousGOARCH = $env:GOARCH
    $previousCGO = $env:CGO_ENABLED

    # 三个目标：android/arm64（真机）、linux/arm64（Termux/树莓派）。
    # android/amd64 与 android/386 需要 NDK+cgo，Go 纯 Go 路径不支持，故不产出。
    $targets = @(
        @{ GOOS = "android"; GOARCH = "arm64"; Name = "powerlevel-android" },
        @{ GOOS = "linux";   GOARCH = "arm64"; Name = "powerlevel-linux-arm64" }
    )

    try {
        foreach ($target in $targets) {
            $env:GOOS = $target.GOOS
            $env:GOARCH = $target.GOARCH
            $env:CGO_ENABLED = "0"
            $out = Join-Path $appRoot $target.Name
            & go build -buildvcs=false -trimpath -ldflags "-s -w" -o $out ./cmd/server
            if ($LASTEXITCODE -ne 0) {
                throw "移动端构建失败: $($target.GOOS)/$($target.GOARCH)"
            }
            Write-Host "已生成: $out"
        }
    } finally {
        $env:GOOS = $previousGOOS
        $env:GOARCH = $previousGOARCH
        $env:CGO_ENABLED = $previousCGO
    }

    Copy-Item (Join-Path $PSScriptRoot "release\config.env.example") (Join-Path $appRoot "config.env.example")
    Write-Host ""
    Write-Host "移动端产物已写入: $appRoot"
    Write-Host "Android 真机经 Termux 运行 powerlevel-android；配方见 README-USER 中的说明。"
} finally {
    Pop-Location
}
