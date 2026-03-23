$ErrorActionPreference = 'Stop'

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$exePath = Join-Path $serverDir 'server-run.exe'
$outLog = Join-Path $serverDir 'server.40061.out.log'
$errLog = Join-Path $serverDir 'server.40061.err.log'

$existing = Get-Process server-run, server -ErrorAction SilentlyContinue
if ($existing) {
  $existing | Stop-Process -Force
  Start-Sleep -Seconds 2
}

if (-not (Test-Path $exePath)) {
  throw "server-run.exe not found: $exePath"
}

Start-Process -FilePath $exePath `
  -WorkingDirectory $serverDir `
  -RedirectStandardOutput $outLog `
  -RedirectStandardError $errLog

Start-Sleep -Seconds 3

try {
  $response = Invoke-WebRequest -Uri 'http://127.0.0.1:40061/healthz' -UseBasicParsing -TimeoutSec 5
  Write-Output $response.StatusCode
  Write-Output $response.Content
} catch {
  Write-Output 'HEALTH_CHECK_FAILED'
  if (Test-Path $errLog) {
    Get-Content $errLog -Tail 50
  }
}
