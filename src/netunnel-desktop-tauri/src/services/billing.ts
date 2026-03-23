import type { BillingProfile, PricingRule, SettlementResult, UserBusinessRecord } from '@/types/netunnel'
import type { ReturnTypeCreateApiClient } from '@/services/shared'

export async function manualRecharge(client: ReturnTypeCreateApiClient, payload: {
  user_id: string
  amount: string
  remark: string
}) {
  return client.request('/api/v1/billing/recharge/manual', { method: 'POST', body: JSON.stringify(payload) })
}

export async function settleBilling(client: ReturnTypeCreateApiClient, userId: string) {
  return client.request<SettlementResult>('/api/v1/billing/settle', {
    method: 'POST',
    body: JSON.stringify({ user_id: userId }),
  })
}

export async function fetchBillingProfile(client: ReturnTypeCreateApiClient, userId: string) {
  return client.request<BillingProfile>(`/api/v1/billing/profile?user_id=${encodeURIComponent(userId)}`)
}

export async function fetchPricingRules(client: ReturnTypeCreateApiClient) {
  return client.request<{ pricing_rules: PricingRule[] }>('/api/v1/billing/plans')
}

export async function activatePricingRule(client: ReturnTypeCreateApiClient, payload: {
  user_id: string
  pricing_rule_id: string
}) {
  return client.request('/api/v1/billing/plans/activate', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function fetchBusinessRecords(client: ReturnTypeCreateApiClient, userId: string, limit = 100) {
  return client.request<{ business_records: UserBusinessRecord[] }>(
    `/api/v1/billing/business-records?user_id=${encodeURIComponent(userId)}&limit=${limit}`,
  )
}
