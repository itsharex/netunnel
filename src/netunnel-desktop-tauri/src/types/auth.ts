export interface AuthSessionState {
  baseUrl: string
  userId: string
  accessToken: string
  rememberedUsername: string
}

export interface WechatProfile {
  openId: string
  unionId?: string
  nickname: string
  avatarUrl?: string
  authorizedAt?: string
}
