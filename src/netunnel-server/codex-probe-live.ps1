$ErrorActionPreference = 'Stop'

Write-Output '=== HEALTH ==='
try {
  $health = Invoke-WebRequest -Uri 'http://127.0.0.1:40061/healthz' -UseBasicParsing -TimeoutSec 5
  Write-Output "STATUS=$($health.StatusCode)"
  Write-Output $health.Content
} catch {
  Write-Output 'HEALTH_FAILED'
  Write-Output $_.Exception.Message
}

Write-Output '=== AGENT REGISTER ==='
try {
  $body = @{
    user_id = 'debug-user'
    name = 'debug-agent'
    machine_code = 'debug-machine'
    client_version = '0.1.0'
    os_type = 'windows'
  } | ConvertTo-Json

  $resp = Invoke-WebRequest `
    -Uri 'http://127.0.0.1:40061/api/v1/agents/register' `
    -Method POST `
    -ContentType 'application/json' `
    -Body $body `
    -UseBasicParsing `
    -TimeoutSec 5
  Write-Output "STATUS=$($resp.StatusCode)"
  Write-Output $resp.Content
} catch {
  Write-Output 'AGENT_REGISTER_FAILED'
  if ($_.Exception.Response) {
    Write-Output $_.Exception.Response.StatusCode.value__
  } else {
    Write-Output $_.Exception.Message
  }
}
