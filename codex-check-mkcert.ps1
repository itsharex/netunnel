$ErrorActionPreference = 'Stop'

$targetDir = Join-Path $PSScriptRoot 'src\netunnel-server\certs\localtest.me'
$certFile = Join-Path $targetDir 'localtest.me.pem'
$keyFile = Join-Path $targetDir 'localtest.me-key.pem'

if (-not (Test-Path $targetDir)) {
  New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
}

$mkcert = Get-Command mkcert -ErrorAction SilentlyContinue
if (-not $mkcert) {
  Write-Output 'MKCERT_MISSING'
  exit 0
}

if (-not (Test-Path $certFile) -or -not (Test-Path $keyFile)) {
  & $mkcert.Source -install
  & $mkcert.Source -cert-file $certFile -key-file $keyFile localtest.me *.localtest.me
}

if ((Test-Path $certFile) -and (Test-Path $keyFile)) {
  Write-Output 'MKCERT_READY'
  Write-Output $certFile
  Write-Output $keyFile
} else {
  Write-Output 'MKCERT_FAILED'
}
