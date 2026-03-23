import type { ReturnTypeCreateApiClient } from '@/services/shared'

export async function fetchUserProfile(client: ReturnTypeCreateApiClient, userId: string) {
  return client.request<{ user: { id: string; email?: string; nickname: string; avatar_url?: string } }>(
    `/api/v1/users/profile?user_id=${encodeURIComponent(userId)}`,
  )
}
