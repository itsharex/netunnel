export interface Account {
  id: string;
  user_id: string;
  balance: string;
  currency: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface User {
  id: string;
  email?: string;
  nickname: string;
  password_hash?: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface Tunnel {
  id: string;
  user_id: string;
  agent_id: string;
  name: string;
  type: "tcp" | "http_host";
  status: string;
  enabled: boolean;
  local_host: string;
  local_port: number;
  remote_port?: number;
  created_at: string;
  updated_at: string;
}

export interface DomainRoute {
  id: string;
  tunnel_id: string;
  domain: string;
  scheme: "http" | "https";
  created_at: string;
  updated_at: string;
}

export interface AccountTransaction {
  id: string;
  user_id: string;
  account_id: string;
  type: string;
  amount: string;
  balance_before: string;
  balance_after: string;
  reference_type?: string;
  reference_id?: string;
  remark?: string;
  created_at: string;
}

export interface PricingRule {
  id: string;
  name: string;
  billing_mode: string;
  price_per_gb: string;
  free_quota_bytes: number;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface BillingSettlementResult {
  account: Account;
  pricing_rule: PricingRule;
  transaction: AccountTransaction | null;
  charged_bytes: number;
  charge_amount: string;
}

export interface TrafficUsage {
  id: string;
  user_id: string;
  agent_id?: string;
  tunnel_id?: string;
  bucket_time: string;
  ingress_bytes: number;
  egress_bytes: number;
  total_bytes: number;
  billed_bytes: number;
  created_at: string;
  updated_at: string;
}

export interface TunnelConnection {
  id: string;
  user_id: string;
  tunnel_id: string;
  agent_id?: string;
  protocol: string;
  source_addr?: string;
  target_addr?: string;
  started_at: string;
  ended_at?: string;
  ingress_bytes: number;
  egress_bytes: number;
  total_bytes: number;
  status: string;
}

export interface DashboardSummary {
  user_id: string;
  account: Account;
  total_agents: number;
  online_agents: number;
  total_tunnels: number;
  enabled_tunnels: number;
  disabled_billing_tunnels: number;
  recent_traffic_bytes_24h: number;
  unbilled_traffic_bytes_24h: number;
  recent_transactions: AccountTransaction[];
  recent_usages: TrafficUsage[];
}
