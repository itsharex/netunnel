$ErrorActionPreference = 'Stop'

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$exePath = Join-Path $serverDir 'server-run.exe'
$outLog = Join-Path $serverDir 'server.40061.out.log'
$errLog = Join-Path $serverDir 'server.40061.err.log'
$goCache = Join-Path $serverDir '.gocache'

Push-Location $serverDir
try {
  if (-not (Test-Path $goCache)) {
    New-Item -ItemType Directory -Path $goCache | Out-Null
  }
  $env:GOCACHE = $goCache

  $existing = Get-Process server-run, server -ErrorAction SilentlyContinue
  if ($existing) {
    $existing | Stop-Process -Force
    Start-Sleep -Seconds 2
  }

  & go build -buildvcs=false -o server-run.exe ./cmd/server
  if ($LASTEXITCODE -ne 0) {
    throw "go build failed with exit code $LASTEXITCODE"
  }

  $item = Get-Item $exePath
  Write-Output "EXE_LAST_WRITE=$($item.LastWriteTime.ToString('s'))"

  Start-Process -FilePath $exePath `
    -WorkingDirectory $serverDir `
    -RedirectStandardOutput $outLog `
    -RedirectStandardError $errLog

  Start-Sleep -Seconds 3

  $health = Invoke-WebRequest -Uri 'http://127.0.0.1:40061/healthz' -UseBasicParsing -TimeoutSec 5
  Write-Output "HEALTH_STATUS=$($health.StatusCode)"
  Write-Output $health.Content

  try {
    Invoke-WebRequest `
      -Uri 'http://127.0.0.1:40061/api/v1/payments/orders' `
      -Method POST `
      -ContentType 'application/json' `
      -Body '{}' `
      -UseBasicParsing `
      -TimeoutSec 5
  } catch {
    $response = $_.Exception.Response
    if ($null -ne $response) {
      Write-Output "PAYMENTS_STATUS=$([int]$response.StatusCode)"
    } else {
      Write-Output 'PAYMENTS_STATUS=NO_RESPONSE'
      Write-Output $_.Exception.Message
    }
  }
} finally {
  Pop-Location
}
