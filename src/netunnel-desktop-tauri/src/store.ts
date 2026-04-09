import { invoke, isTauri } from '@tauri-apps/api/core'
import { listen, type UnlistenFn } from '@tauri-apps/api/event'
import { acceptHMRUpdate, defineStore } from 'pinia'
import { runtimeEnv } from '@/config/env'
import { completeWechatBusinessLogin, loginWithNetunnel } from '@/services/auth'
import { log } from '@/services/logger'
import { resizeMainWindow } from '@/services/window'

const LOGIN_WINDOW_WIDTH = 360
const LOGIN_WINDOW_HEIGHT = 520
import type { AuthSessionState, WechatProfile } from '@/types/auth'

type ThemeMode = 'light' | 'dark' | 'system'
type RoundnessMode = 'sharp' | 'rounded' | 'soft'
type LayoutMode = '1x2' | '2x1' | 'grid'
type SessionKey = 'tunnels'

interface UserProfile {
  name: string
  plan: string
  email: string
  avatarUrl: string
}

interface SessionItem {
  id: SessionKey
  label: string
  icon: string
  hint: string
}

interface BrowserPanel {
  id: number
  title: string
  url: string
  summary: string
  tag: string
  accent: string
  status: 'online' | 'syncing'
}

interface SettingsState {
  theme: ThemeMode
  roundness: RoundnessMode
  acrylicEnabled: boolean
  transparency: number
  launchAtStartup: boolean
  homeUrl: string
  bridgeAddr: string
  defaultSyncInterval: string
  agentExecutablePath: string
  closeToTray: boolean
}

interface UpdaterStatusPayload {
  enabled: boolean
  reason: string | null
  currentVersion: string
  endpoints: string[]
}

interface UpdatePayload {
  version: string
  currentVersion: string
  date: string | null
  body: string | null
  readyToInstall: boolean
  downloading: boolean
}

interface UpdaterState {
  enabled: boolean
  reason: string | null
  endpoints: string[]
  checking: boolean
  installing: boolean
  progressPhase: 'idle' | 'downloading' | 'installing'
  downloadedBytes: number
  totalBytes: number | null
  progressPercent: number | null
  available: UpdatePayload | null
  lastCheckedAt: string | null
  lastError: string | null
  promptVisible: boolean
}

interface UpdateProgressPayload {
  phase: 'idle' | 'downloading' | 'downloaded' | 'installing' | 'failed'
  downloadedBytes: number
  totalBytes: number | null
  percent: number | null
  error: string | null
}

interface LogStatusPayload {
  directory: string
  filePath: string
}

interface LogState {
  directory: string | null
  filePath: string | null
  lastError: string | null
}

const versionString =
  import.meta.env.MODE === 'development' ? `${import.meta.env.VITE_APP_VERSION}-dev` : import.meta.env.VITE_APP_VERSION
const AUTO_UPDATE_INTERVAL_MS = 2 * 60 * 60 * 1000
const SETTINGS_STORAGE_KEY = 'netunnel-desktop-tauri-settings'
const SESSION_STORAGE_KEY = 'netunnel-desktop-tauri-session'

function isLegacyLocalHomeUrl(value: unknown) {
  if (typeof value !== 'string') {
    return false
  }

  const trimmed = value.trim()
  if (!trimmed) {
    return false
  }

  if (trimmed === '127.0.0.1:40061' || trimmed === 'localhost:40061') {
    return true
  }

  try {
    const normalized = trimmed.includes('://') ? trimmed : `http://${trimmed}`
    const parsed = new URL(normalized)
    return (
      (parsed.hostname === '127.0.0.1' || parsed.hostname === 'localhost') &&
      parsed.port === '40061'
    )
  } catch {
    return false
  }
}

function isLegacyLocalBridgeAddr(value: unknown) {
  if (typeof value !== 'string') {
    return false
  }

  const trimmed = value.trim()
  if (!trimmed) {
    return false
  }

  return trimmed === '127.0.0.1:40062' || trimmed === 'localhost:40062'
}

function migrateProductionHomeUrl(value: unknown) {
  if ((import.meta.env.MODE === 'production' || import.meta.env.MODE === 'development') && isLegacyLocalHomeUrl(value)) {
    return runtimeEnv.defaultHomeUrl
  }
  return typeof value === 'string' ? value : undefined
}

function migrateProductionBridgeAddr(value: unknown) {
  if (import.meta.env.MODE === 'production' && isLegacyLocalBridgeAddr(value)) {
    return runtimeEnv.defaultBridgeAddr
  }
  return typeof value === 'string' ? value : undefined
}

let autoUpdateTimer: ReturnType<typeof setInterval> | null = null
let updaterProgressUnlisten: UnlistenFn | null = null

const basePanels: BrowserPanel[] = [
  {
    id: 1,
    title: '飞书文档',
    url: 'https://feishu.cn/docx/team-space',
    summary: '项目周报、迭代安排与成员同步集中在这个窗格，适合工作模式快速切换。',
    tag: '工作空间',
    accent: 'from-sky-500/30 via-cyan-400/10 to-transparent',
    status: 'online',
  },
  {
    id: 2,
    title: '中国知网',
    url: 'https://scholar.cnki.net/advanced-search',
    summary: '研究资料检索与参考文献整理入口，模拟学术搜索结果页的阅读场景。',
    tag: '研究检索',
    accent: 'from-indigo-500/30 via-violet-400/10 to-transparent',
    status: 'syncing',
  },
  {
    id: 3,
    title: 'Gitee 探索',
    url: 'https://gitee.com/explore',
    summary: '开源项目趋势、Issue 追踪与代码片段浏览，用于模拟开发者工作台。',
    tag: '开源协作',
    accent: 'from-emerald-500/30 via-teal-400/10 to-transparent',
    status: 'online',
  },
]

function loadPersistedSettings(): Partial<SettingsState> {
  if (typeof window === 'undefined') {
    return {}
  }

  try {
    const raw = window.localStorage.getItem(SETTINGS_STORAGE_KEY)
    if (!raw) {
      return {}
    }
    return JSON.parse(raw) as Partial<SettingsState>
  } catch {
    return {}
  }
}

const persistedSettings = loadPersistedSettings()
const persistedHomeUrl = migrateProductionHomeUrl(persistedSettings.homeUrl)
const persistedBridgeAddr = migrateProductionBridgeAddr(persistedSettings.bridgeAddr)

function loadPersistedSession(): Partial<AuthSessionState> & { isAuthenticated?: boolean } {
  if (typeof window === 'undefined') {
    return {}
  }

  try {
    const raw = window.localStorage.getItem(SESSION_STORAGE_KEY)
    if (!raw) {
      return {}
    }
    return JSON.parse(raw) as Partial<AuthSessionState> & { isAuthenticated?: boolean }
  } catch {
    return {}
  }
}

const persistedSession = loadPersistedSession()
const persistedSessionBaseUrl = migrateProductionHomeUrl(persistedSession.baseUrl)

export const useStore = defineStore('main', {
  state: () => ({
    debug: import.meta.env.MODE === 'development',
    version: versionString,
    isInitialized: false,
    isAuthenticated: persistedSession.isAuthenticated ?? false,
    isLoginSubmitting: false,
    isSettingsModalOpen: false,
    currentSession: 'tunnels' as SessionKey,
    layoutMode: 'grid' as LayoutMode,
    loginForm: {
      username: persistedSession.rememberedUsername ?? runtimeEnv.defaultLoginUsername,
      password: '123456',
      remember: true,
    },
    loginError: '',
    session: {
      baseUrl: persistedSessionBaseUrl ?? persistedHomeUrl ?? runtimeEnv.defaultHomeUrl,
      userId: persistedSession.userId ?? '',
      accessToken: persistedSession.accessToken ?? '',
      rememberedUsername: persistedSession.rememberedUsername ?? runtimeEnv.defaultLoginUsername,
    } as AuthSessionState,
    user: {
      name: 'Netunnel Admin',
      plan: '开发联调',
      email: runtimeEnv.defaultLoginUsername,
      avatarUrl: '',
    } as UserProfile,
    sessions: [
      { id: 'tunnels', label: '隧道', icon: 'transit-connection-variant', hint: 'TCP / 域名映射' },
    ] as SessionItem[],
    panels: basePanels as BrowserPanel[],
    settings: {
      theme: persistedSettings.theme ?? 'system',
      roundness: persistedSettings.roundness ?? 'sharp',
      acrylicEnabled: persistedSettings.acrylicEnabled ?? true,
      transparency: persistedSettings.transparency ?? 85,
      launchAtStartup: persistedSettings.launchAtStartup ?? false,
      homeUrl: persistedHomeUrl ?? runtimeEnv.defaultHomeUrl,
      bridgeAddr: persistedBridgeAddr ?? runtimeEnv.defaultBridgeAddr,
      defaultSyncInterval: persistedSettings.defaultSyncInterval ?? '10',
      agentExecutablePath: persistedSettings.agentExecutablePath ?? '',
      closeToTray: persistedSettings.closeToTray ?? true,
    } as SettingsState,
    updater: {
      enabled: false,
      reason: null,
      endpoints: [],
      checking: false,
      installing: false,
      progressPhase: 'idle',
      downloadedBytes: 0,
      totalBytes: null,
      progressPercent: null,
      available: null,
      lastCheckedAt: null,
      lastError: null,
      promptVisible: false,
    } as UpdaterState,
    logs: {
      directory: null,
      filePath: null,
      lastError: null,
    } as LogState,
    notifications: [
      '桌面端正在基于 Tauri 模板改造为 netunnel 控制台。',
      '当前保留模板登录与设置壳，业务区域对接 netunnel 服务端。',
    ],
    summary: null as { totalUsers: number; onlineUsers: number; onlineAgents: number; totalAgents: number; enabledTunnels: number; totalTunnels: number; recentTrafficBytes: number } | null,
  }),

  getters: {
    currentRadius(state) {
      switch (state.settings.roundness) {
        case 'sharp':
          return 8
        case 'soft':
          return 24
        default:
          return 16
      }
    },
    activeSession(state) {
      return state.sessions.find((item) => item.id === state.currentSession) ?? state.sessions[0]
    },
    themeClass(state) {
      return state.settings.theme === 'dark' ? 'theme-dark' : 'theme-light'
    },
    pageTitle(state) {
      switch (state.currentSession) {
        case 'tunnels':
          return '隧道管理'
      }
    },
  },

  actions: {
    initApp() {
      if (this.isInitialized) {
        return
      }
      this.isInitialized = true
      if (!this.session.baseUrl) {
        this.session.baseUrl = this.settings.homeUrl
      }
      if (import.meta.env.MODE === 'production') {
        if (isLegacyLocalHomeUrl(this.session.baseUrl)) {
          this.session.baseUrl = runtimeEnv.defaultHomeUrl
        }
        if (isLegacyLocalHomeUrl(this.settings.homeUrl)) {
          this.settings.homeUrl = runtimeEnv.defaultHomeUrl
        }
        if (isLegacyLocalBridgeAddr(this.settings.bridgeAddr)) {
          this.settings.bridgeAddr = runtimeEnv.defaultBridgeAddr
        }
      }
      this.persistSettings()
      this.persistSession()
      void this.setupUpdaterProgressListener()
      void this.loadUpdaterStatus()
      void this.loadLogStatus()
    },
    async login() {
      const username = this.loginForm.username.trim()
      const password = this.loginForm.password.trim()
      const baseUrl = this.session.baseUrl.trim() || this.settings.homeUrl

      if (!username || !password) {
        this.loginError = '请输入登录账号和密码。'
        return
      }

      this.isLoginSubmitting = true
      this.loginError = ''

      try {
        this.session.baseUrl = baseUrl
        this.session.rememberedUsername = username
        const result = await loginWithNetunnel({
          baseUrl,
          username,
          password,
          existingUserId: this.session.userId,
        })

        this.session.userId = result.userId
        this.session.accessToken = result.accessToken
        this.user.name = result.nickname || 'Netunnel Admin'
        this.user.email = result.email || username
        this.user.plan = result.mode === 'auth_login' ? '正式会话' : '开发联调'
        this.user.avatarUrl = result.avatarUrl || ''

        this.isAuthenticated = true
        this.persistSession()
      } catch (error) {
        this.loginError = String(error)
      } finally {
        this.isLoginSubmitting = false
      }
    },
    async loginWithWechatProfile(profile: WechatProfile) {
      const baseUrl = this.session.baseUrl.trim() || this.settings.homeUrl
      this.isLoginSubmitting = true
      this.loginError = ''

      try {
        const result = await completeWechatBusinessLogin({
          baseUrl,
          profile,
        })
        log('INFO', `loginWithWechatProfile result: userId=${result.userId} accessToken=${result.accessToken ? 'exists' : 'missing'} mode=${result.mode}`)

        this.session.baseUrl = baseUrl
        this.session.rememberedUsername = profile.nickname
        this.session.userId = result.userId
        this.session.accessToken = result.accessToken
        this.user.name = result.nickname || profile.nickname || '微信用户'
        this.user.email = result.email
        this.user.plan = '微信登录'
        this.user.avatarUrl = result.avatarUrl || profile.avatarUrl || ''

        this.isAuthenticated = true
        this.persistSession()
      } catch (error) {
        this.loginError = String(error)
      } finally {
        this.isLoginSubmitting = false
      }
    },
    async logout() {
      if (isTauri()) {
        try {
          await invoke('stop_local_agent')
        } catch (error) {
          log('WARN', `stop_local_agent before logout failed: ${String(error)}`)
        }
      }
      this.isAuthenticated = false
      this.isSettingsModalOpen = false
      this.persistSession()
      void resizeMainWindow(LOGIN_WINDOW_WIDTH, LOGIN_WINDOW_HEIGHT)
    },
    setSession(sessionId: SessionKey) {
      this.currentSession = sessionId
    },
    setLayout(layout: LayoutMode) {
      this.layoutMode = layout
    },
    openSettingsModal() {
      this.isSettingsModalOpen = true
    },
    closeSettingsModal() {
      this.isSettingsModalOpen = false
    },
    updateTheme(theme: ThemeMode) {
      this.settings.theme = theme
      this.persistSettings()
    },
    updateRoundness(roundness: RoundnessMode) {
      this.settings.roundness = roundness
      this.persistSettings()
    },
    setTransparency(value: number) {
      this.settings.transparency = value
      this.persistSettings()
    },
    updateSetting<K extends keyof SettingsState>(key: K, value: SettingsState[K]) {
      this.settings[key] = value
      this.persistSettings()
      if (key === 'homeUrl' && !this.session.baseUrl) {
        this.session.baseUrl = value as string
        this.persistSession()
      }
    },
    persistSettings() {
      if (typeof window === 'undefined') {
        return
      }

      window.localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(this.settings))
    },
    updateSession<K extends keyof AuthSessionState>(key: K, value: AuthSessionState[K]) {
      this.session[key] = value
      if (key === 'baseUrl') {
        this.settings.homeUrl = value as string
        this.persistSettings()
      }
      this.persistSession()
    },
    persistSession() {
      if (typeof window === 'undefined') {
        return
      }

      const payload = {
        ...this.session,
        isAuthenticated: this.loginForm.remember ? this.isAuthenticated : false,
      }
      window.localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(payload))
    },
    addMockPanel() {
      const nextId = this.panels.length + 1
      this.panels.push({
        id: nextId,
        title: `新窗格 ${nextId}`,
        url: `${this.settings.homeUrl}?panel=${nextId}`,
        summary: '新建的模拟窗格，可用于展示更多站点或分屏内容。',
        tag: '新建内容',
        accent: 'from-amber-500/30 via-orange-400/10 to-transparent',
        status: 'online',
      })
    },
    async loadUpdaterStatus() {
      if (!isTauri()) {
        this.updater.enabled = false
        this.updater.reason = '仅 Tauri 桌面应用支持检查更新。'
        this.updater.endpoints = []
        return
      }

      try {
        const status = await invoke<UpdaterStatusPayload>('updater_status')
        this.version = import.meta.env.MODE === 'development' ? `${status.currentVersion}-dev` : status.currentVersion
        this.updater.enabled = status.enabled
        this.updater.reason = status.reason
        this.updater.endpoints = status.endpoints
      } catch (error) {
        this.updater.enabled = false
        this.updater.reason = `读取更新配置失败：${String(error)}`
        this.updater.endpoints = []
      }
    },
    async loadLogStatus() {
      if (!isTauri()) {
        this.logs.directory = null
        this.logs.filePath = null
        this.logs.lastError = '仅 Tauri 桌面应用支持本地日志。'
        return
      }

      try {
        const status = await invoke<LogStatusPayload>('logger_status')
        this.logs.directory = status.directory
        this.logs.filePath = status.filePath
        this.logs.lastError = null
      } catch (error) {
        this.logs.directory = null
        this.logs.filePath = null
        this.logs.lastError = String(error)
      }
    },
    async checkForUpdates(options?: { background?: boolean }) {
      if (!isTauri()) {
        this.updater.lastError = '当前不在 Tauri 桌面环境中，无法检查更新。'
        return
      }

      if (this.updater.checking || this.updater.installing) {
        return
      }

      this.updater.checking = true
      this.updater.lastError = null
      this.resetUpdaterProgress()

      try {
        const update = await invoke<UpdatePayload | null>('check_for_update')
        this.updater.available = update
        this.updater.lastCheckedAt = new Date().toISOString()
        this.updater.promptVisible = Boolean(update?.readyToInstall)

        if (update?.readyToInstall && options?.background) {
          const message = `更新已下载完成 v${update.version}，可在设置中直接安装。`
          if (!this.notifications.includes(message)) {
            this.notifications.unshift(message)
          }
        }
      } catch (error) {
        this.updater.available = null
        this.updater.lastError = String(error)
      } finally {
        this.updater.checking = false
      }
    },
    async installAvailableUpdate() {
      if (!isTauri()) {
        this.updater.lastError = '当前不在 Tauri 桌面环境中，无法安装更新。'
        return
      }

      if (!this.updater.available || this.updater.installing) {
        return
      }

      this.updater.installing = true
      this.updater.lastError = null
      this.resetUpdaterProgress()

      try {
        await invoke('install_update')
      } catch (error) {
        this.updater.lastError = String(error)
        this.updater.installing = false
        this.updater.progressPhase = 'idle'
      }
    },
    resetUpdaterProgress() {
      this.updater.progressPhase = 'idle'
      this.updater.downloadedBytes = 0
      this.updater.totalBytes = null
      this.updater.progressPercent = null
    },
    async setupUpdaterProgressListener() {
      if (!isTauri() || updaterProgressUnlisten) {
        return
      }

      updaterProgressUnlisten = await listen<UpdateProgressPayload>('updater://progress', (event) => {
        const payload = event.payload
        this.updater.progressPhase = payload.phase === 'downloaded' || payload.phase === 'failed' ? 'idle' : payload.phase
        this.updater.downloadedBytes = payload.downloadedBytes
        this.updater.totalBytes = payload.totalBytes
        this.updater.progressPercent = payload.percent === null ? null : Math.round(payload.percent)
        if (payload.error) {
          this.updater.lastError = payload.error
        }
        if (this.updater.available) {
          if (payload.phase === 'downloaded') {
            this.updater.available = {
              ...this.updater.available,
              downloading: false,
              readyToInstall: true,
            }
            this.updater.promptVisible = true
            const message = `更新已下载完成 v${this.updater.available.version}，可在设置中直接安装。`
            if (!this.notifications.includes(message)) {
              this.notifications.unshift(message)
            }
          } else if (payload.phase === 'downloading') {
            this.updater.available = {
              ...this.updater.available,
              downloading: true,
              readyToInstall: false,
            }
            this.updater.promptVisible = false
          } else if (payload.phase === 'failed') {
            this.updater.available = {
              ...this.updater.available,
              downloading: false,
              readyToInstall: false,
            }
            this.updater.promptVisible = false
          }
        }
      })
    },
    dismissUpdatePrompt() {
      this.updater.promptVisible = false
    },
    startAutoUpdateChecks() {
      if (!isTauri()) {
        return
      }

      this.stopAutoUpdateChecks()
      void this.checkForUpdates({ background: true })

      autoUpdateTimer = setInterval(() => {
        void this.checkForUpdates({ background: true })
      }, AUTO_UPDATE_INTERVAL_MS)
    },
    stopAutoUpdateChecks() {
      if (!autoUpdateTimer) {
        return
      }

      clearInterval(autoUpdateTimer)
      autoUpdateTimer = null
    },
    async openLogsDirectory() {
      if (!isTauri()) {
        this.logs.lastError = '当前不在 Tauri 桌面环境中，无法打开日志目录。'
        return
      }

      try {
        await invoke('open_logs_directory')
        this.logs.lastError = null
      } catch (error) {
        this.logs.lastError = String(error)
      }
    },
    async openDevtools() {
      if (!isTauri()) {
        return
      }

      try {
        await invoke('open_devtools')
      } catch (error) {
        this.logs.lastError = String(error)
      }
    },
  },
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useStore, import.meta.hot))
}
