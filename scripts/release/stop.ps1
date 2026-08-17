$ErrorActionPreference = "Stop"
$appRoot = Split-Path -Parent $PSScriptRoot
$binaryPath = Join-Path $appRoot "PowerLevel.exe"
$runtimeRoot = Join-Path $appRoot "runtime"
$pidFile = Join-Path $runtimeRoot "powerlevel.pid"

if (-not (Test-Path $pidFile)) { Write-Host "PowerLevel status updated."; exit 0 }
$storedPid = 0
if (-not [int]::TryParse((Get-Content $pidFile -Raw).Trim(), [ref]$storedPid)) {
    Remove-Item $pidFile -Force
    throw "PowerLevel operation failed."
}
$process = Get-CimInstance Win32_Process -Filter "ProcessId = $storedPid" -ErrorAction SilentlyContinue
if ($null -eq $process) { Remove-Item $pidFile -Force; Write-Host "PowerLevel status updated."; exit 0 }
if ([string]::IsNullOrWhiteSpace($process.ExecutablePath) -or -not [string]::Equals([IO.Path]::GetFullPath($process.ExecutablePath), [IO.Path]::GetFullPath($binaryPath), [StringComparison]::OrdinalIgnoreCase)) {
    throw "PowerLevel operation failed."
}
Stop-Process -Id $storedPid -ErrorAction Stop
try { Wait-Process -Id $storedPid -Timeout 20 -ErrorAction Stop } catch { Stop-Process -Id $storedPid -Force -ErrorAction Stop; Wait-Process -Id $storedPid -Timeout 5 -ErrorAction SilentlyContinue }
Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
Write-Host "PowerLevel status updated."
