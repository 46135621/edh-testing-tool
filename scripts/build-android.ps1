param(
    [string]$AndroidSdk = $env:ANDROID_HOME,
    [string]$JavaHome = $env:JAVA_HOME
)

# 一键构建 Android APK：
#   1. 确认 gomobile / Android SDK / JDK 环境
#   2. gomobile bind 把 cmd/mobile 打成 mobile.aar（在 AAR 里启动内嵌 HTTP 服务）
#   3. 用 Gradle 把 android/ 壳工程 + mobile.aar 打包成 apk
#
# 依赖：Go >= 1.22 + gomobile（go install golang.org/x/mobile/cmd/gomobile@latest；
#   gomobile init 会自动下载 NDK）、Android SDK（compileSdk 34）、JDK 17。
$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot

if (-not (Get-Command gomobile -ErrorAction SilentlyContinue)) {
    throw "未找到 gomobile。请先安装: go install golang.org/x/mobile/cmd/gomobile@latest 并确保 GOPATH/bin 在 PATH 中。"
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "未找到 go。请先安装 Go。"
}
$javaHome = $JavaHome
if (-not $javaHome) { $javaHome = $env:JAVA_HOME }
if (-not $javaHome) { throw "未设置 JAVA_HOME（需要 JDK 17 或更高）。" }

$androidHome = $AndroidSdk
if (-not $androidHome) { $androidHome = $env:ANDROID_HOME }
if (-not $androidHome -and $env:ANDROID_SDK_ROOT) { $androidHome = $env:ANDROID_SDK_ROOT }
if (-not $androidHome) { throw "未设置 ANDROID_HOME / ANDROID_SDK_ROOT。请安装 Android SDK（compileSdk 34）。" }

Push-Location $projectRoot
try {
    # 1. gomobile bind
    $aarDir = Join-Path $projectRoot "android\app\libs"
    New-Item -ItemType Directory -Path $aarDir -Force | Out-Null
    & gomobile bind -target=android -o (Join-Path $aarDir "mobile.aar") ./cmd/mobile
    if ($LASTEXITCODE -ne 0) { throw "gomobile bind 失败" }

    # 2. Gradle 打包（android/ 壳工程，mobile.aar 已就位）
    $gradle = Get-Command gradle -ErrorAction SilentlyContinue
    if (-not $gradle) {
        $wrapper = Join-Path $projectRoot "android\gradlew.bat"
        if (Test-Path $wrapper) { $gradle = $wrapper } else {
            throw "未找到 gradle 或 android/gradlew.bat。"
        }
    }
    Push-Location (Join-Path $projectRoot "android")
    try {
        $env:ANDROID_HOME = $androidHome
        $env:JAVA_HOME = $javaHome
        & $gradle.Source assembleRelease
        if ($LASTEXITCODE -ne 0) { throw "Gradle assembleRelease 失败" }
    } finally {
        Pop-Location
    }

    Write-Host ""
    Write-Host "APK: android\app\build\outputs\apk\release\app-release-unsigned.apk"
} finally {
    Pop-Location
}
