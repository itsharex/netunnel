import type { WechatProfile } from '@/types/auth'

const WECHAT_LOGIN_BASE_URL =
  'https://open.tx07.cn/api/v1/apps/app_mmzvo9v9e89cc5bbda9611551902/wechat-login'

export interface WechatLoginSession {
  sessionId: string
  bizId: string
  status: 'pending' | 'success' | 'expired'
  qrCodeUrl: string
  pollUrl: string
  expiresAt: string
  profile: WechatProfile | null
}

interface WechatLoginApiResponse {
  code: number
  msg: string
  data: WechatLoginSession
}

function ensureWechatResponse(payload: WechatLoginApiResponse) {
  if (payload.code !== 200 || !payload.data) {
    throw new Error(payload.msg || '微信登录接口返回异常')
  }
  return payload.data
}

export async function createWechatLoginSession(bizId: string): Promise<WechatLoginSession> {
  const response = await fetch(`${WECHAT_LOGIN_BASE_URL}/sessions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ bizId }),
  })

  const payload = (await response.json().catch(() => ({}))) as WechatLoginApiResponse
  if (!response.ok) {
    throw new Error(payload.msg || `HTTP ${response.status}`)
  }

  return ensureWechatResponse(payload)
}

export async function pollWechatLoginSession(bizId: string): Promise<WechatLoginSession> {
  const response = await fetch(`${WECHAT_LOGIN_BASE_URL}/sessions/by-biz/${encodeURIComponent(bizId)}`, {
    method: 'GET',
  })

  const payload = (await response.json().catch(() => ({}))) as WechatLoginApiResponse
  if (!response.ok) {
    throw new Error(payload.msg || `HTTP ${response.status}`)
  }

  return ensureWechatResponse(payload)
}

export async function pollWechatLoginSessionByUrl(pollUrl: string): Promise<WechatLoginSession> {
  const response = await fetch(pollUrl, {
    method: 'GET',
  })

  const payload = (await response.json().catch(() => ({}))) as WechatLoginApiResponse
  if (!response.ok) {
    throw new Error(payload.msg || `HTTP ${response.status}`)
  }

  return ensureWechatResponse(payload)
}
