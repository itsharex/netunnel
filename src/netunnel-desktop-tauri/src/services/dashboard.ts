import type { DashboardSummary } from '@/types/netunnel'
import type { ReturnTypeCreateApiClient } from '@/services/shared'

export async function fetchDashboardSummary(client: ReturnTypeCreateApiClient, userId: string) {
  return client.request<{ summary: DashboardSummary }>(`/api/v1/dashboard/summary?user_id=${encodeURIComponent(userId)}`)
}
