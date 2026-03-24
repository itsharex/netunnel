function requireEnv(name: keyof ImportMetaEnv) {
  const value = import.meta.env[name]?.trim()
  if (!value) {
    throw new Error(`[runtimeEnv] Missing required env: ${name}`)
  }
  return value
}

const DEFAULT_HOME_URL = requireEnv('VITE_DEFAULT_HOME_URL')
const DEFAULT_BRIDGE_ADDR = requireEnv('VITE_DEFAULT_BRIDGE_ADDR')
const WECHAT_LOGIN_BASE_URL = requireEnv('VITE_WECHAT_LOGIN_BASE_URL')
const DEFAULT_LOGIN_USERNAME = requireEnv('VITE_DEFAULT_LOGIN_USERNAME')

export const runtimeEnv = {
  defaultHomeUrl: DEFAULT_HOME_URL,
  defaultBridgeAddr: DEFAULT_BRIDGE_ADDR,
  wechatLoginBaseUrl: WECHAT_LOGIN_BASE_URL,
  defaultLoginUsername: DEFAULT_LOGIN_USERNAME,
} as const
