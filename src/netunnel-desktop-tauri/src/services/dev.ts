import type { ReturnTypeCreateApiClient } from '@/services/shared'

export async function bootstrapDevelopmentUser(client: ReturnTypeCreateApiClient, payload: {
  email: string
  nickname: string
  password: string
}) {
  return client.request<{ user?: { id?: string; email?: string; nickname?: string } }>('/api/v1/dev/bootstrap-user', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}
