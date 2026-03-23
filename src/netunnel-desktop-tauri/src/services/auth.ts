import { apiRequest } from '@/services/api'
import type { WechatProfile } from '@/types/auth'

export interface LoginRequest {
  baseUrl: string
  username: string
  password: string
  existingUserId?: string
}

export interface LoginResult {
  userId: string
  email: string
  nickname: string
  accessToken: string
  avatarUrl?: string
  mode: 'auth_login' | 'dev_bootstrap' | 'wechat_dev_bootstrap'
}

export async function loginWithNetunnel(request: LoginRequest): Promise<LoginResult> {
  const normalizedBaseUrl = request.baseUrl.trim().replace(/\/+$/, '')
  const username = request.username.trim()
  const password = request.password.trim()

  try {
    const authPayload = await apiRequest<any>(
      '/api/v1/auth/login',
      { baseUrl: normalizedBaseUrl },
      {
        method: 'POST',
        body: JSON.stringify({
          username,
          password,
        }),
      },
    )

    if (authPayload.user?.id) {
      return {
        userId: authPayload.user.id,
        email: authPayload.user.email ?? username,
        nickname: authPayload.user.nickname ?? username.split('@')[0] ?? 'netunnel',
        accessToken: authPayload.access_token ?? authPayload.token ?? '',
        avatarUrl: authPayload.user.avatar_url ?? authPayload.user.avatarUrl ?? '',
        mode: 'auth_login',
      }
    }
  } catch {
    // Keep the desktop side tolerant while the backend has not exposed /auth/login yet.
  }

  if (request.existingUserId?.trim()) {
    return {
      userId: request.existingUserId.trim(),
      email: username,
      nickname: username.split('@')[0] ?? 'netunnel',
      accessToken: '',
      mode: 'dev_bootstrap',
    }
  }

  const bootstrapPayload = await apiRequest<any>(
    '/api/v1/dev/bootstrap-user',
    { baseUrl: normalizedBaseUrl },
    {
      method: 'POST',
      body: JSON.stringify({
        email: username.includes('@') ? username : `${username}@netunnel.local`,
        nickname: (username.includes('@') ? username : `${username}@netunnel.local`).split('@')[0],
        password,
      }),
    },
  )

  const email = bootstrapPayload.user?.email ?? (username.includes('@') ? username : `${username}@netunnel.local`)
  const nickname = bootstrapPayload.user?.nickname ?? email.split('@')[0]

  return {
    userId: bootstrapPayload.user?.id ?? '',
    email,
    nickname,
    accessToken: '',
    mode: 'dev_bootstrap',
  }
}

export async function completeWechatBusinessLogin(request: {
  baseUrl: string
  profile: WechatProfile
  existingUserId?: string
}): Promise<LoginResult> {
  const normalizedBaseUrl = request.baseUrl.trim().replace(/\/+$/, '')
  const openId = request.profile.openId.trim()
  const unionId = request.profile.unionId?.trim() || openId
  const email = `wx_${openId}@wechat.local`
  const nickname = request.profile.nickname?.trim() || `微信用户-${openId.slice(0, 6)}`

  const bootstrapPayload = await apiRequest<any>(
    '/api/v1/dev/bootstrap-user',
    { baseUrl: normalizedBaseUrl },
    {
      method: 'POST',
      body: JSON.stringify({
        email,
        nickname,
        avatar_url: request.profile.avatarUrl,
        password: unionId,
        wechat_openid: openId,
      }),
    },
  )

  return {
    userId: bootstrapPayload.user?.id ?? bootstrapPayload.user_id ?? '',
    email: bootstrapPayload.user?.email ?? email,
    nickname: bootstrapPayload.user?.nickname ?? nickname,
    accessToken: bootstrapPayload.access_token ?? '',
    avatarUrl: request.profile.avatarUrl,
    mode: 'wechat_dev_bootstrap',
  }
}
