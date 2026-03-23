param(
    [string]$HealthUrl = 'http://127.0.0.1:40061/healthz',
    [int]$StartupTimeoutSeconds = 15
)

$ErrorActionPreference = 'Stop'

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$outLog = Join-Path $serverDir 'server.40461.out.log'
$errLog = Join-Path $serverDir 'server.40461.err.log'

$candidates = @('server-run.exe', 'server.exe') |
    ForEach-Object { Join-Path $serverDir $_ } |
    Where-Object { Test-Path $_ }

if ($candidates.Count -eq 0) {
    throw "No backend executable found in $serverDir"
}

$exePath = $candidates[0]

$existing = Get-CimInstance Win32_Process |
    Where-Object {
        $_.ExecutablePath -and
        (($_.ExecutablePath -ieq (Join-Path $serverDir 'server-run.exe')) -or
         ($_.ExecutablePath -ieq (Join-Path $serverDir 'server.exe')))
    }

if ($existing) {
    $existing | ForEach-Object {
        Write-Host "Stopping backend PID $($_.ProcessId): $($_.ExecutablePath)"
        Stop-Process -Id $_.ProcessId -Force
    }
    Start-Sleep -Seconds 2
}

$process = Start-Process -FilePath $exePath `
    -WorkingDirectory $serverDir `
    -RedirectStandardOutput $outLog `
    -RedirectStandardError $errLog `
    -PassThru

Write-Host "Started backend PID $($process.Id): $exePath"

$deadline = (Get-Date).AddSeconds($StartupTimeoutSeconds)
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 1

    try {
        $response = Invoke-WebRequest -Uri $HealthUrl -UseBasicParsing -TimeoutSec 5
        Write-Host "Backend is healthy: $($response.StatusCode) $($response.Content)"
        exit 0
    } catch {
        if (-not (Get-Process -Id $process.Id -ErrorAction SilentlyContinue)) {
            Write-Host 'Backend exited during startup. Recent stderr:'
            if (Test-Path $errLog) {
                Get-Content $errLog -Tail 50
            }
            exit 1
        }
    }
}

Write-Host "Health check timed out: $HealthUrl"
if (Test-Path $errLog) {
    Write-Host 'Recent stderr:'
    Get-Content $errLog -Tail 50
}
exit 1
