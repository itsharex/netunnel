$ErrorActionPreference = 'Stop'

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$outLog = Join-Path $serverDir 'server.40061.out.log'
$errLog = Join-Path $serverDir 'server.40061.err.log'

Write-Output '=== PROCESSES ==='
Get-Process server-run, server -ErrorAction SilentlyContinue | Select-Object Id, ProcessName, Path

Write-Output '=== MATCHING SERVER DIR PROCESSES ==='
$serverPathHint = $serverDir.ToLowerInvariant()
Get-Process -ErrorAction SilentlyContinue |
  ForEach-Object {
    try {
      if ($_.Path -and $_.Path.ToLowerInvariant().Contains($serverPathHint)) {
        [PSCustomObject]@{
          Id = $_.Id
          ProcessName = $_.ProcessName
          Path = $_.Path
        }
      }
    } catch {
    }
  }

Write-Output '=== ERR LOG ==='
if (Test-Path $errLog) {
  Get-Content $errLog -Tail 80
} else {
  Write-Output 'NO_ERR_LOG'
}

Write-Output '=== OUT LOG ==='
if (Test-Path $outLog) {
  Get-Content $outLog -Tail 80
} else {
  Write-Output 'NO_OUT_LOG'
}
