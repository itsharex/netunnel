$ErrorActionPreference = 'Stop'

$lines = netstat -ano | Select-String ':40061'
if (-not $lines) {
  Write-Output 'NO_LISTENER'
  exit 0
}

$listenLine = $lines | Select-Object -First 1
Write-Output $listenLine.Line

$parts = ($listenLine.Line -split '\s+') | Where-Object { $_ -ne '' }
$pid = $parts[-1]
Write-Output "PID=$pid"

$proc = Get-Process -Id $pid -ErrorAction SilentlyContinue
if ($proc) {
  Write-Output "PROCESS=$($proc.ProcessName)"
}
