param(
    [string]$Target = "x86_64-pc-windows-msvc"
)

# 一键构建 Tauri 桌面客户端：
#   1. 编译 Go 服务端为 sidecar 二进制
#   2. 调用 cargo tauri build 打出 NSIS 安装包
$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot

Push-Location $projectRoot
try {
    if (-not (Get-Command cargo -ErrorAction SilentlyContinue)) {
        throw "未找到 cargo。请先安装 Rust: https://rustup.rs"
    }
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        throw "未找到 go。请先安装 Go。"
    }

    # 1. 编译 sidecar（文件名必须带 target triple 后缀，Tauri 按此定位）
    $binDir = Join-Path $projectRoot "src-tauri\binaries"
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    $sidecar = Join-Path $binDir "server-$Target.exe"
    $env:CGO_ENABLED = "0"
    & go build -buildvcs=false -trimpath -ldflags "-s -w" -o $sidecar ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "Go sidecar 编译失败" }

    # 2. 构建 Tauri 客户端
    Push-Location (Join-Path $projectRoot "src-tauri")
    try {
        cargo tauri build
        if ($LASTEXITCODE -ne 0) { throw "tauri build 失败" }
    } finally {
        Pop-Location
    }

    Write-Host ""
    Write-Host "单文件客户端: src-tauri\target\release\edh-testing-tool-client.exe"
} finally {
    Pop-Location
}
