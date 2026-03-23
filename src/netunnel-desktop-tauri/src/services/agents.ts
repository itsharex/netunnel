import type { ReturnTypeCreateApiClient } from '@/services/shared'
import type { Agent } from '@/types/netunnel'

export async function registerAgent(client: ReturnTypeCreateApiClient, payload: {
  user_id: string
  name: string
  machine_code: string
  client_version: string
  os_type: string
}) {
  return client.request<{ created: boolean; agent: Agent }>('/api/v1/agents/register', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}
