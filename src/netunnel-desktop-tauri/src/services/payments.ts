import type { ReturnTypeCreateApiClient } from '@/services/shared'

export interface PaymentBusinessNotify {
  status: string
  attempts: number
  notifiedAt?: string
  response?: string
  error?: string
}

export interface PaymentSession {
  sessionId: string
  appId: string
  bizId: string
  status: 'pending' | 'paid' | 'expired' | 'closed'
  amount: number
  notifyUrl: string
  qrCodeUrl: string
  checkoutUrl: string
  pollUrl: string
  expiresAt?: string
  paidAt?: string
  paymentProduct: {
    id: string
    name: string
    description: string
    price: number
  }
  businessNotify?: PaymentBusinessNotify
}

export interface PaymentOrder {
  biz_id: string
  user_id: string
  order_type: string
  payment_product_id: string
  pricing_rule_id?: string
  recharge_gb?: number
  session_id?: string
  notify_url: string
  poll_url?: string
  qr_code_url?: string
  checkout_url?: string
  amount: number
  platform_status: string
  apply_status: 'pending' | 'processing' | 'applied' | 'failed'
  business_notify_status?: string
  business_notify_error?: string
  expires_at?: string
  paid_at?: string
  apply_error?: string
  created_at: string
  updated_at: string
}

export interface PaymentOrderSnapshot {
  order: PaymentOrder
  session?: PaymentSession
  applied: boolean
  appliedAt?: string
  applyError?: string
  businessNotify?: PaymentBusinessNotify
}

export async function createPaymentOrder(client: ReturnTypeCreateApiClient, payload: {
  user_id: string
  order_type: 'traffic_recharge' | 'pricing_rule'
  payment_product_id: string
  pricing_rule_id?: string
  recharge_gb?: number
}) {
  return client.request<PaymentOrderSnapshot>('/api/v1/payments/orders', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function pollPaymentOrder(client: ReturnTypeCreateApiClient, bizId: string) {
  return client.request<PaymentOrderSnapshot>(`/api/v1/payments/orders/by-biz/${encodeURIComponent(bizId)}`)
}
