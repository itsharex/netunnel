import type { DomainRoute, Tunnel } from '@/types/netunnel'
import type { ReturnTypeCreateApiClient } from '@/services/shared'

export async function fetchTunnels(client: ReturnTypeCreateApiClient, userId: string) {
  return client.request<{ tunnels: Tunnel[] }>(`/api/v1/tunnels?user_id=${encodeURIComponent(userId)}`)
}

export async function fetchDomainRoutes(client: ReturnTypeCreateApiClient, tunnelId: string) {
  return client.request<{ routes: DomainRoute[] }>(`/api/v1/domain-routes?tunnel_id=${encodeURIComponent(tunnelId)}`)
}

export async function createTcpTunnel(client: ReturnTypeCreateApiClient, payload: {
  user_id: string
  agent_id: string
  name: string
  local_host: string
  local_port: number
}) {
  return client.request('/api/v1/tunnels/tcp', { method: 'POST', body: JSON.stringify(payload) })
}

export async function createHostTunnel(client: ReturnTypeCreateApiClient, payload: {
  user_id: string
  agent_id: string
  name: string
  local_host: string
  local_port: number
  domain_prefix: string
}) {
  return client.request('/api/v1/tunnels/http-host', { method: 'POST', body: JSON.stringify(payload) })
}

export async function updateTunnel(client: ReturnTypeCreateApiClient, tunnelId: string, payload: {
  user_id: string
  agent_id: string
  name: string
  local_host: string
  local_port: number
  domain?: string
}) {
  return client.request(`/api/v1/tunnels/${tunnelId}`, { method: 'PUT', body: JSON.stringify(payload) })
}

export async function toggleTunnelStatus(client: ReturnTypeCreateApiClient, tunnelId: string, userId: string, enabled: boolean) {
  return client.request(`/api/v1/tunnels/${tunnelId}/${enabled ? 'disable' : 'enable'}?user_id=${encodeURIComponent(userId)}`, { method: 'POST' })
}

export async function deleteTunnel(client: ReturnTypeCreateApiClient, tunnelId: string, userId: string) {
  return client.request(`/api/v1/tunnels/${tunnelId}?user_id=${encodeURIComponent(userId)}`, { method: 'DELETE' })
}

export async function deleteDomainRoute(client: ReturnTypeCreateApiClient, routeId: string, userId: string) {
  return client.request(`/api/v1/domain-routes/${routeId}?user_id=${encodeURIComponent(userId)}`, { method: 'DELETE' })
}
