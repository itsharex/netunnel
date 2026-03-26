$ErrorActionPreference = 'Stop'

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$goCache = Join-Path $serverDir '.gocache'
$windowsPowerShellDir = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0'
$portsToFree = @(40061, 40062, 40063)
$localConfigPath = Join-Path $serverDir 'config.local.yaml'
$defaultConfigPath = Join-Path $serverDir 'config.yaml'

if (-not (Test-Path $goCache)) {
  New-Item -ItemType Directory -Path $goCache | Out-Null
}

$env:GOCACHE = $goCache
if (Test-Path $windowsPowerShellDir) {
  $env:Path = "$windowsPowerShellDir;$env:Path"
}

function Stop-ListenerProcesses {
  param(
    [int[]]$Ports
  )

  $listeners = @()
  if (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue) {
    $listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
      Where-Object { $Ports -contains $_.LocalPort } |
      Select-Object @{Name = 'LocalPort'; Expression = { [int]$_.LocalPort } }, @{Name = 'OwningProcess'; Expression = { [int]$_.OwningProcess } })
  } else {
    $netstatPath = Join-Path $env:SystemRoot 'System32\netstat.exe'
    if (-not (Test-Path $netstatPath)) {
      throw '无法找到 netstat.exe，无法自动释放端口。'
    }

    $portLookup = @{}
    foreach ($port in $Ports) {
      $portLookup[$port] = $true
    }

    $listeners = @(foreach ($line in & $netstatPath -ano -p tcp) {
      if ($line -notmatch '^\s*TCP\s+') {
        continue
      }

      $parts = $line -split '\s+'
      if ($parts.Count -lt 5) {
        continue
      }

      $state = $parts[3]
      if ($state -ne 'LISTENING') {
        continue
      }

      $localAddress = $parts[1]
      $pidText = $parts[4]
      $portText = $localAddress.Substring($localAddress.LastIndexOf(':') + 1)

      $port = 0
      $pid = 0
      if (-not [int]::TryParse($portText, [ref]$port)) {
        continue
      }
      if (-not $portLookup.ContainsKey($port)) {
        continue
      }
      if (-not [int]::TryParse($pidText, [ref]$pid)) {
        continue
      }

      [pscustomobject]@{
        LocalPort     = $port
        OwningProcess = $pid
      }
    })
  }

  if (-not $listeners) {
    return
  }

  $pids = $listeners |
    Select-Object -ExpandProperty OwningProcess -Unique |
    Where-Object { $_ -gt 0 }

  foreach ($pid in $pids) {
    $process = Get-CimInstance Win32_Process -Filter "ProcessId = $pid" -ErrorAction SilentlyContinue
    if (-not $process) {
      continue
    }

    $portsText = ($listeners |
      Where-Object { $_.OwningProcess -eq $pid } |
      Select-Object -ExpandProperty LocalPort -Unique |
      Sort-Object |
      ForEach-Object { $_.ToString() }) -join ', '

    $displayPath = if ($process.ExecutablePath) { $process.ExecutablePath } else { $process.Name }
    Write-Host "Stopping PID $pid listening on port(s): $portsText -> $displayPath"
    Stop-Process -Id $pid -Force -ErrorAction Stop
  }

  Start-Sleep -Seconds 2
}

$air = Get-Command air -ErrorAction SilentlyContinue
if (-not $air) {
  $airExe = Join-Path $env:USERPROFILE 'go\bin\air.exe'
  if (Test-Path $airExe) {
    $air = Get-Item $airExe
  } else {
    throw "air 未安装。先执行: go install github.com/air-verse/air@latest"
  }
}

Push-Location $serverDir
try {
  if (-not $env:NETUNNEL_CONFIG) {
    if (Test-Path $localConfigPath) {
      $env:NETUNNEL_CONFIG = $localConfigPath
      Write-Host "Using local config: $($env:NETUNNEL_CONFIG)"
    } elseif (Test-Path $defaultConfigPath) {
      $env:NETUNNEL_CONFIG = $defaultConfigPath
      Write-Host "Using default config: $($env:NETUNNEL_CONFIG)"
    }
  } else {
    Write-Host "Using NETUNNEL_CONFIG from environment: $($env:NETUNNEL_CONFIG)"
  }

  Stop-ListenerProcesses -Ports $portsToFree
  $airPath = if ($air -is [System.Management.Automation.CommandInfo]) { $air.Source } else { $air.FullName }
  & $airPath -c .air.toml
} finally {
  Pop-Location
}
