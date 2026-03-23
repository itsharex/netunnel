$ErrorActionPreference = 'Stop'

$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$goCache = Join-Path $serverDir '.gocache'
if (-not (Test-Path $goCache)) {
  New-Item -ItemType Directory -Path $goCache | Out-Null
}
$env:GOCACHE = $goCache

Push-Location $serverDir
try {
  & go list -f '{{.Dir}}' ./cmd/server
  & go list -f '{{.Dir}}' netunnel/server/internal/transport/http
  & go list -f '{{.Dir}}' netunnel/server/internal/service
} finally {
  Pop-Location
}
