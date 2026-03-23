import type { ReturnTypeCreateApiClient } from '@/services/shared'

export async function fetchPlatformConfig(client: ReturnTypeCreateApiClient) {
  return client.request<{ host_domain_suffix?: string }>('/api/v1/platform/config')
}
