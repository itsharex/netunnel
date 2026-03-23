export interface Agent {
  id: string
  user_id: string
  name: string
  machine_code: string
  secret_key: string
  status: string
  client_version: string
  os_type: string
}

export interface DashboardSummary {
  user_id: string
  account: { balance: string; currency: string }
  total_agents: number
  online_agents: number
  total_tunnels: number
  enabled_tunnels: number
  disabled_billing_tunnels: number
  recent_traffic_bytes_24h: number
  unbilled_traffic_bytes_24h: number
  recent_business_records: Array<{
    id: string
    record_type: string
    change_amount: string
    traffic_balance_before: string
    traffic_balance_after: string
    related_resource_type?: string
    related_resource_id?: string
    traffic_bytes?: number
    billable_bytes?: number
    package_expires_at?: string
    payment_order_biz_id?: string
    description?: string
  }>
  recent_usages: Array<{
    id: string
    tunnel_id?: string
    bucket_time: string
    total_bytes: number
    billed_bytes: number
  }>
}

export interface UserBusinessRecord {
  id: string
  record_type: string
  change_amount: string
  traffic_balance_before: string
  traffic_balance_after: string
  related_resource_type?: string
  related_resource_id?: string
  traffic_bytes?: number
  billable_bytes?: number
  package_expires_at?: string
  payment_order_biz_id?: string
  description?: string
  created_at: string
}

export interface Tunnel {
  id: string
  agent_id: string
  name: string
  type: 'tcp' | 'http_host'
  status: string
  enabled: boolean
  local_host: string
  local_port: number
  remote_port?: number
  access_target?: string
}

export interface DomainRoute {
  id: string
  tunnel_id: string
  domain: string
  scheme: 'http' | 'https'
  access_url?: string
}

export interface UsageConnection {
  id: string
  tunnel_id?: string
  protocol?: string
  source_addr?: string
  target_addr?: string
  total_bytes?: number
  ingress_bytes?: number
  egress_bytes?: number
  status?: string
}

export interface UsageTrafficBucket {
  id: string
  tunnel_id?: string
  bucket_time: string
  ingress_bytes: number
  egress_bytes: number
  total_bytes?: number
  billed_bytes?: number
}

export interface SettlementResult {
  charged_bytes: number
  included_bytes?: number
  billable_bytes?: number
  charge_amount: string
  transaction?: { id?: string }
}

export interface BillingAccount {
  id?: string
  user_id?: string
  balance: string
  currency: string
  status?: string
}

export interface PricingRule {
  id: string
  name: string
  display_name: string
  description: string
  billing_mode: 'traffic' | 'subscription'
  price_per_gb: string
  free_quota_bytes: number
  subscription_price: string
  included_traffic_bytes: number
  subscription_period: 'none' | 'month' | 'year'
  traffic_reset_period: 'none' | 'month' | 'year'
  is_unlimited: boolean
  status: string
}

export interface UserSubscription {
  id: string
  user_id: string
  pricing_rule_id: string
  status: string
  started_at: string
  current_period_start: string
  current_period_end?: string
  current_period_used_bytes: number
  expires_at?: string
  cancelled_at?: string
}

export interface BillingProfile {
  account: BillingAccount
  pricing_rule: PricingRule
  subscription?: UserSubscription
}
