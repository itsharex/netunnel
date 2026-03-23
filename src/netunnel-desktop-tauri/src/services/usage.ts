import type { UsageConnection, UsageTrafficBucket } from '@/types/netunnel'
import type { ReturnTypeCreateApiClient } from '@/services/shared'

export async function fetchUsageConnections(client: ReturnTypeCreateApiClient, options: {
  userId: string
  tunnelId?: string
  limit: number
}) {
  const usageQuery = options.tunnelId ? `&tunnel_id=${encodeURIComponent(options.tunnelId)}` : ''
  return client.request<{ connections: UsageConnection[] }>(
    `/api/v1/usage/connections?user_id=${encodeURIComponent(options.userId)}${usageQuery}&limit=${options.limit}`,
  )
}

export async function fetchUsageTraffic(client: ReturnTypeCreateApiClient, options: {
  userId: string
  tunnelId?: string
  hours: number
}) {
  const usageQuery = options.tunnelId ? `&tunnel_id=${encodeURIComponent(options.tunnelId)}` : ''
  return client.request<{ usages: UsageTrafficBucket[] }>(
    `/api/v1/usage/traffic?user_id=${encodeURIComponent(options.userId)}${usageQuery}&hours=${options.hours}`,
  )
}
