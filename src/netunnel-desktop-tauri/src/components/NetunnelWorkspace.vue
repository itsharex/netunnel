<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { invoke, isTauri } from '@tauri-apps/api/core'
import { open as openExternal } from '@tauri-apps/plugin-shell'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { registerAgent } from '@/services/agents'
import { createApiClient } from '@/services/api'
import { manualRecharge, settleBilling } from '@/services/billing'
import { fetchDashboardSummary } from '@/services/dashboard'
import { bootstrapDevelopmentUser as createDevUser } from '@/services/dev'
import { fetchPlatformConfig } from '@/services/platform'
import { useStore } from '@/store'
import { createHostTunnel, createTcpTunnel, deleteDomainRoute, deleteTunnel, fetchDomainRoutes, fetchTunnels, updateTunnel } from '@/services/tunnels'
import { fetchUsageConnections, fetchUsageTraffic } from '@/services/usage'
import type { DashboardSummary, DomainRoute, SettlementResult, Tunnel, UsageConnection, UsageTrafficBucket } from '@/types/netunnel'

const props = defineProps<{
  page: 'tunnels'
}>()
const emit = defineEmits<{
  refreshSummary: []
}>()
const store = useStore()

const storageKey = 'netunnel-desktop-tauri-workspace'
const persisted = typeof window === 'undefined' ? null : JSON.parse(window.localStorage.getItem(storageKey) ?? '{}')

async function getOrCreateMachineCode(): Promise<string> {
  if (!isTauri()) {
    return persisted?.agentMachineCode ?? persisted?.tcpAgentId ?? crypto.randomUUID()
  }
  return invoke<string>('get_or_create_agent_id')
}

const baseUrl = computed({
  get: () => store.session.baseUrl,
  set: (value: string) => store.updateSession('baseUrl', value),
})

const userId = computed({
  get: () => store.session.userId,
  set: (value: string) => store.updateSession('userId', value),
})

const accessToken = computed({
  get: () => store.session.accessToken,
  set: (value: string) => store.updateSession('accessToken', value),
})
const loading = ref(false)
const actionMessage = ref('')
const actionError = ref('')
const summary = ref<DashboardSummary | null>(null)
const tunnels = ref<Tunnel[]>([])
const domainRoutes = ref<Record<string, DomainRoute[]>>({})
const usageConnections = ref<UsageConnection[]>([])
const usageTraffic = ref<UsageTrafficBucket[]>([])
const rechargeAmount = ref(persisted?.rechargeAmount ?? '1.0000')
const rechargeRemark = ref(persisted?.rechargeRemark ?? 'desktop manual recharge')
const usageLimit = ref(persisted?.usageLimit ?? '10')
const usageHours = ref(persisted?.usageHours ?? '24')
const usageTunnelId = ref(persisted?.usageTunnelId ?? '')
const hostDomainSuffix = ref(persisted?.hostDomainSuffix ?? '')
const lastSettlement = ref<{ chargedBytes: number; chargeAmount: string; transactionId?: string } | null>(null)
const registeredAgentId = ref(persisted?.registeredAgentId ?? '')
const registeredAgentUserId = ref(persisted?.registeredAgentUserId ?? '')
const agentMachineCode = ref(persisted?.agentMachineCode ?? persisted?.tcpAgentId ?? '')
const nativeState = ref({
  traySupported: isTauri(),
  agentRunning: false,
  agentExecutablePath: '',
  agentArguments: [] as string[],
  agentPid: null as number | null,
  lastExit: '' as string,
})

type NativeAgentStatus = {
  running: boolean
  executablePath: string
  arguments: string[]
  pid?: number | null
  lastExit?: string | null
}

let nativeAgentMonitorTimer: number | null = null

const tcpForm = reactive({
  agentId: '',
  name: persisted?.tcpName ?? '',
  localHost: persisted?.tcpLocalHost ?? '127.0.0.1',
  localPort: persisted?.tcpLocalPort ?? '',
})

const hostForm = reactive({
  agentId: '',
  name: persisted?.hostName ?? '',
  localHost: persisted?.hostLocalHost ?? '127.0.0.1',
  localPort: persisted?.hostLocalPort ?? '',
  domain: persisted?.hostDomain ?? '',
})
const commonTcpPorts = ['3389', '22', '3306', '5432', '6379']
const commonHostPorts = ['3389', '22', '3306', '5432', '6379']


const tcpDialogVisible = ref(false)
const hostDialogVisible = ref(false)
const tcpDialogError = ref('')
const hostDialogError = ref('')
const debugDialogVisible = ref(false)
const editingTunnelId = ref('')
const tcpNameInputRef = ref()
const hostNameInputRef = ref()
const inlineEditingTunnelId = ref('')
const inlineTunnelName = ref('')
const inlineSavingTunnelId = ref('')
const debuggingTunnel = ref<Tunnel | null>(null)
const debugRunning = ref(false)
const debugLogs = ref<Array<{ level: 'info' | 'success' | 'warn' | 'error'; message: string }>>([])

const bootstrapForm = reactive({
  email: persisted?.bootstrapEmail ?? '',
  nickname: persisted?.bootstrapNickname ?? 'desktop-dev-user',
  password: persisted?.bootstrapPassword ?? 'dev123456',
})

const agentForm = reactive({
  executablePath: persisted?.agentExecutablePath ?? '',
  serverUrl: persisted?.agentServerUrl ?? store.settings.homeUrl ?? 'http://127.0.0.1:40061',
  bridgeAddr: persisted?.agentBridgeAddr ?? store.settings.bridgeAddr ?? '127.0.0.1:40062',
  userId: persisted?.agentUserId ?? store.session.userId ?? '',
  agentName: persisted?.agentName ?? 'desktop-agent',
  machineCode: persisted?.agentMachineCode ?? 'machine-desktop',
  clientVersion: persisted?.agentClientVersion ?? '0.1.0',
  osType: persisted?.agentOsType ?? 'windows',
  syncInterval: persisted?.agentSyncInterval ?? store.settings.defaultSyncInterval ?? '10',
  extraArgsText: persisted?.agentExtraArgsText ?? '',
})

watch(
  [baseUrl, userId, accessToken, rechargeAmount, rechargeRemark, usageLimit, usageHours, usageTunnelId],
  () => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(
      storageKey,
      JSON.stringify({
        baseUrl: baseUrl.value,
        userId: userId.value,
        accessToken: accessToken.value,
        rechargeAmount: rechargeAmount.value,
        rechargeRemark: rechargeRemark.value,
        usageLimit: usageLimit.value,
        usageHours: usageHours.value,
        usageTunnelId: usageTunnelId.value,
        hostDomainSuffix: hostDomainSuffix.value,
        registeredAgentId: registeredAgentId.value,
        registeredAgentUserId: registeredAgentUserId.value,
        agentMachineCode: agentMachineCode.value,
        tcpAgentId: tcpForm.agentId,
        tcpName: tcpForm.name,
        tcpLocalHost: tcpForm.localHost,
        tcpLocalPort: tcpForm.localPort,
        hostAgentId: hostForm.agentId,
        hostName: hostForm.name,
        hostLocalHost: hostForm.localHost,
        hostLocalPort: hostForm.localPort,
        hostDomain: hostForm.domain,
        bootstrapEmail: bootstrapForm.email,
        bootstrapNickname: bootstrapForm.nickname,
        bootstrapPassword: bootstrapForm.password,
        agentExecutablePath: agentForm.executablePath,
        agentServerUrl: agentForm.serverUrl,
        agentBridgeAddr: agentForm.bridgeAddr,
        agentUserId: agentForm.userId,
        agentName: agentForm.agentName,
        agentClientVersion: agentForm.clientVersion,
        agentOsType: agentForm.osType,
        agentSyncInterval: agentForm.syncInterval,
        agentExtraArgsText: agentForm.extraArgsText,
      }),
    )
  },
  { deep: true },
)

const agentArgumentsPreview = computed(() => buildAgentArguments())
const apiClient = computed(() =>
  createApiClient({
    baseUrl: baseUrl.value,
    accessToken: accessToken.value,
  }),
)

const hostDomainSuffixPreview = computed(() => {
  return hostDomainSuffix.value.trim() || 'your-domain.example.com'
})

const hostFullDomainPreview = computed(() => {
  const prefix = hostForm.domain.trim().replace(/^\.+|\.+$/g, '').toLowerCase()
  if (!prefix) {
    return `自动生成.${hostDomainSuffixPreview.value}`
  }
  return `${prefix}.${hostDomainSuffixPreview.value}`
})

function resetStatus() {
  actionMessage.value = ''
  actionError.value = ''
}

function applyNativeAgentStatus(result: NativeAgentStatus, fallbackExecutablePath = '') {
  nativeState.value = {
    traySupported: true,
    agentRunning: result.running,
    agentExecutablePath: result.executablePath || fallbackExecutablePath,
    agentArguments: result.arguments ?? [],
    agentPid: result.pid ?? null,
    lastExit: result.lastExit ?? '',
  }
}

async function compensateAgentRegistrationIfNeeded(status: NativeAgentStatus) {
  if (!status.running || !userId.value.trim() || !baseUrl.value.trim()) {
    return
  }
  if (registeredAgentId.value && registeredAgentUserId.value === userId.value.trim()) {
    return
  }

  const agentId = await ensureRegisteredAgent(true)
  actionMessage.value = `本地 agent 运行中，已完成补偿注册：${agentId}`
}

async function loadAll() {
  loading.value = true
  resetStatus()
  try {
    const [summaryRes, tunnelRes, platformRes] = await Promise.allSettled([
      fetchDashboardSummary(apiClient.value, userId.value),
      fetchTunnels(apiClient.value, userId.value),
      fetchPlatformConfig(apiClient.value),
    ])

    if (summaryRes.status === 'fulfilled') {
      summary.value = summaryRes.value.summary
    } else {
      summary.value = null
    }

    if (tunnelRes.status === 'fulfilled') {
      tunnels.value = tunnelRes.value.tunnels
    } else {
      tunnels.value = []
    }

    if (platformRes.status === 'fulfilled') {
      hostDomainSuffix.value = platformRes.value.host_domain_suffix?.trim() ?? ''
    }

    const routeResults = await Promise.all(
      tunnels.value.filter((item) => item.type === 'http_host').map(async (item) => {
        const res = await fetchDomainRoutes(apiClient.value, item.id)
        return [item.id, res.routes] as const
      }),
    )
    domainRoutes.value = Object.fromEntries(routeResults)
    const [connRes, trafficRes] = await Promise.all([
      fetchUsageConnections(apiClient.value, {
        userId: userId.value,
        tunnelId: usageTunnelId.value || undefined,
        limit: Number(usageLimit.value) || 10,
      }),
      fetchUsageTraffic(apiClient.value, {
        userId: userId.value,
        tunnelId: usageTunnelId.value || undefined,
        hours: Number(usageHours.value) || 24,
      }),
    ])
    usageConnections.value = connRes.connections
    usageTraffic.value = trafficRes.usages
    const firstAgentId = tunnels.value[0]?.agent_id ?? ''
    if (firstAgentId && !tcpForm.agentId) tcpForm.agentId = firstAgentId
    if (firstAgentId && !hostForm.agentId) hostForm.agentId = firstAgentId
    if (!agentForm.userId && userId.value) {
      agentForm.userId = userId.value
    }
    const errors = [summaryRes, tunnelRes, platformRes]
      .filter((result) => result.status === 'rejected')
      .map((result) => (result as PromiseRejectedResult).reason instanceof Error ? (result as PromiseRejectedResult).reason.message : String((result as PromiseRejectedResult).reason))
    if (errors.length > 0) {
      actionError.value = errors.join(' | ')
    }
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : String(error)
  } finally {
    loading.value = false
  }
}

async function refreshWorkspace() {
  emit('refreshSummary')
  await loadAll()
}

async function ensureRegisteredAgent(forceRefresh = false) {
  const trimmedUserId = userId.value.trim()
  const trimmedBaseUrl = baseUrl.value.trim()
  if (!trimmedUserId) {
    throw new Error('请先登录并拿到 user_id。')
  }
  if (!trimmedBaseUrl) {
    throw new Error('请先配置服务端地址。')
  }

  const machineCode = (await getOrCreateMachineCode()).trim()
  if (!machineCode) {
    throw new Error('无法生成本机 agent machine code。')
  }

  agentMachineCode.value = machineCode
  agentForm.machineCode = machineCode

  const canReuse =
    !forceRefresh &&
    registeredAgentId.value &&
    registeredAgentUserId.value === trimmedUserId

  if (canReuse) {
    tcpForm.agentId = registeredAgentId.value
    hostForm.agentId = registeredAgentId.value
    return registeredAgentId.value
  }

  const response = await registerAgent(apiClient.value, {
    user_id: trimmedUserId,
    name: agentForm.agentName.trim() || 'desktop-agent',
    machine_code: machineCode,
    client_version: agentForm.clientVersion.trim() || '0.1.0',
    os_type: agentForm.osType.trim() || 'windows',
  })

  registeredAgentId.value = response.agent.id
  registeredAgentUserId.value = trimmedUserId
  tcpForm.agentId = response.agent.id
  hostForm.agentId = response.agent.id
  return response.agent.id
}

async function bootstrapDevelopmentUser() {
  await mutate(
    () => createDevUser(apiClient.value, {
      email: bootstrapForm.email,
      nickname: bootstrapForm.nickname,
      password: bootstrapForm.password,
    }),
    '开发用户创建成功。',
    true,
  )
}

async function createTcpTunnelAction() {
  const agentId = await ensureRegisteredAgent()
  await mutate(
    () => createTcpTunnel(apiClient.value, {
      user_id: userId.value,
      agent_id: agentId,
      name: tcpForm.name,
      local_host: tcpForm.localHost,
      local_port: Number(tcpForm.localPort),
    }),
    'TCP tunnel 创建成功。',
  )
}

async function saveTcpTunnelAction() {
  tcpDialogError.value = ''
  const agentId = await ensureRegisteredAgent()
  if (!editingTunnelId.value) {
    try {
      await createTcpTunnelAction()
      return true
    } catch (error) {
      tcpDialogError.value = error instanceof Error ? error.message : String(error)
      actionError.value = ''
      return false
    }
  }

  try {
    await mutate(
      () => updateTunnel(apiClient.value, editingTunnelId.value, {
        user_id: userId.value,
        agent_id: agentId,
        name: tcpForm.name,
        local_host: tcpForm.localHost,
        local_port: Number(tcpForm.localPort),
      }),
      'TCP tunnel 更新成功。',
    )
    return true
  } catch (error) {
    tcpDialogError.value = error instanceof Error ? error.message : String(error)
    actionError.value = ''
    return false
  }
}

async function createHostTunnelAction() {
  const agentId = await ensureRegisteredAgent()
  await mutate(
    () => createHostTunnel(apiClient.value, {
      user_id: userId.value,
      agent_id: agentId,
      name: hostForm.name,
      local_host: hostForm.localHost,
      local_port: Number(hostForm.localPort),
      domain_prefix: hostForm.domain,
    }),
    'Host tunnel 创建成功。',
  )
}

async function saveHostTunnelAction() {
  hostDialogError.value = ''
  const agentId = await ensureRegisteredAgent()
  if (!editingTunnelId.value) {
    try {
      await createHostTunnelAction()
      return true
    } catch (error) {
      hostDialogError.value = error instanceof Error ? error.message : String(error)
      actionError.value = ''
      return false
    }
  }

  try {
    await mutate(
      () => updateTunnel(apiClient.value, editingTunnelId.value, {
        user_id: userId.value,
        agent_id: agentId,
        name: hostForm.name,
        local_host: hostForm.localHost,
        local_port: Number(hostForm.localPort),
        domain: hostForm.domain,
      }),
      'Host tunnel 更新成功。',
    )
    return true
  } catch (error) {
    hostDialogError.value = error instanceof Error ? error.message : String(error)
    actionError.value = ''
    return false
  }
}

async function submitRecharge() {
  await mutate(
    () => manualRecharge(apiClient.value, {
      user_id: userId.value,
      amount: rechargeAmount.value,
      remark: rechargeRemark.value,
    }),
    '充值成功，数据已刷新。',
  )
}

async function mutate(requestFn: () => Promise<any>, successMessage: string, updateUserId = false) {
  loading.value = true
  resetStatus()
  try {
    const res = await requestFn()
    if (updateUserId && res.user?.id) userId.value = res.user.id
    if (updateUserId && res.user?.email) {
      store.user.email = res.user.email
    }
    if (updateUserId && res.user?.nickname) {
      store.user.name = res.user.nickname
    }
    if ((res as SettlementResult).charged_bytes !== undefined) {
      lastSettlement.value = { chargedBytes: res.charged_bytes, chargeAmount: res.charge_amount, transactionId: res.transaction?.id }
    }
    actionMessage.value = successMessage
    await loadAll()
    return res
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : String(error)
    throw error
  } finally {
    loading.value = false
  }
}

async function runSettlement() {
  await mutate(() => settleBilling(apiClient.value, userId.value), '结算完成。')
}
async function removeTunnel(tunnel: Tunnel) {
  await simpleAction(() => deleteTunnel(apiClient.value, tunnel.id, userId.value), `${tunnel.name} 已删除。`)
}
async function removeDomainRoute(route: DomainRoute) {
  await simpleAction(() => deleteDomainRoute(apiClient.value, route.id, userId.value), `${route.scheme}://${route.domain} 已删除。`)
}
function resetTcpDialog() {
  editingTunnelId.value = ''
  tcpDialogError.value = ''
  tcpForm.name = ''
  tcpForm.localHost = '127.0.0.1'
  tcpForm.localPort = ''
}

function resetHostDialog() {
  editingTunnelId.value = ''
  hostDialogError.value = ''
  hostForm.name = ''
  hostForm.localHost = '127.0.0.1'
  hostForm.localPort = ''
  hostForm.domain = ''
}

function openCreateTcpDialog() {
  resetTcpDialog()
  tcpDialogVisible.value = true
}

function openCreateHostDialog() {
  resetHostDialog()
  hostDialogVisible.value = true
}

watch(tcpDialogVisible, (visible) => {
  if (!visible) {
    tcpDialogError.value = ''
  }
})

watch(hostDialogVisible, (visible) => {
  if (!visible) {
    hostDialogError.value = ''
  }
})

function focusTunnelNameInput(kind: 'tcp' | 'host') {
  requestAnimationFrame(() => {
    const inputRef = kind === 'tcp' ? tcpNameInputRef.value : hostNameInputRef.value
    inputRef?.focus?.()
  })
}

function openEditTunnelDialog(tunnel: Tunnel) {
  editingTunnelId.value = tunnel.id
  if (tunnel.type === 'tcp') {
    tcpForm.name = tunnel.name
    tcpForm.localHost = tunnel.local_host
    tcpForm.localPort = String(tunnel.local_port)
    tcpDialogVisible.value = true
    return
  }

  const route = domainRoutes.value[tunnel.id]?.[0]
  hostForm.name = tunnel.name
  hostForm.localHost = tunnel.local_host
  hostForm.localPort = String(tunnel.local_port)
  hostForm.domain = route?.domain ? route.domain.split('.')[0] ?? '' : ''
  hostDialogVisible.value = true
}

function startInlineEditName(tunnel: Tunnel) {
  inlineEditingTunnelId.value = tunnel.id
  inlineTunnelName.value = tunnel.name
}

function cancelInlineEditName() {
  inlineEditingTunnelId.value = ''
  inlineTunnelName.value = ''
}

async function saveInlineTunnelName(tunnel: Tunnel) {
  const nextName = inlineTunnelName.value.trim()
  if (!nextName || nextName === tunnel.name || inlineSavingTunnelId.value === tunnel.id) {
    cancelInlineEditName()
    return
  }

  inlineSavingTunnelId.value = tunnel.id
  try {
    const payload: {
      user_id: string
      agent_id: string
      name: string
      local_host: string
      local_port: number
      domain?: string
    } = {
      user_id: userId.value,
      agent_id: tunnel.agent_id,
      name: nextName,
      local_host: tunnel.local_host,
      local_port: tunnel.local_port,
    }

    if (tunnel.type !== 'tcp') {
      const route = domainRoutes.value[tunnel.id]?.[0]
      payload.domain = route?.domain ?? ''
    }

    await updateTunnel(apiClient.value, tunnel.id, payload)
    actionMessage.value = `${nextName} 已保存。`
    await loadAll()
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : String(error)
  } finally {
    inlineSavingTunnelId.value = ''
    cancelInlineEditName()
  }
}

function isLocalTunnel(tunnel: Tunnel) {
  return tunnel.agent_id === registeredAgentId.value
}

function pushDebugLog(level: 'info' | 'success' | 'warn' | 'error', message: string) {
  debugLogs.value = [...debugLogs.value, { level, message }]
}

type ProbeResult = {
  ok: boolean
  address: string
  message: string
}

async function runTunnelDebug(tunnel: Tunnel) {
  debugRunning.value = true
  debugLogs.value = []
  debuggingTunnel.value = tunnel
  debugDialogVisible.value = true

  pushDebugLog('info', `开始诊断隧道：${tunnel.name}`)
  pushDebugLog('info', `基础信息：type=${tunnel.type} local=${tunnel.local_host}:${tunnel.local_port} enabled=${tunnel.enabled}`)

  try {
    const latestTunnels = await fetchTunnels(apiClient.value, userId.value)
    const latestTunnel = latestTunnels.tunnels.find((item) => item.id === tunnel.id)
    if (!latestTunnel) {
      pushDebugLog('error', '服务端一致性检查失败：未找到该隧道')
    } else {
      pushDebugLog('success', `服务端一致性检查：隧道存在，status=${latestTunnel.status} enabled=${latestTunnel.enabled}`)
      if (latestTunnel.agent_id !== tunnel.agent_id) {
        pushDebugLog('warn', `服务端最新 agent_id 与当前列表不一致：${latestTunnel.agent_id}`)
      }
      if (latestTunnel.name !== tunnel.name) {
        pushDebugLog('warn', `服务端最新名称与当前列表不一致：${latestTunnel.name}`)
      }
    }
  } catch (error) {
    pushDebugLog('error', `服务端一致性检查失败：${String(error)}`)
  }

  const targets = getTunnelAccessTargets(tunnel)
  if (targets.length > 0) {
    pushDebugLog('success', `外部映射：${targets.join(' , ')}`)
  } else {
    pushDebugLog('warn', '未发现可用的外部映射地址')
  }

  if (tunnel.type === 'tcp') {
    if (tunnel.remote_port) {
      pushDebugLog('success', `远端端口已分配：${tunnel.remote_port}`)
    } else {
      pushDebugLog('error', 'TCP 隧道未分配远端端口')
    }
  } else {
    let routes = domainRoutes.value[tunnel.id] || []
    try {
      const latestRoutes = await fetchDomainRoutes(apiClient.value, tunnel.id)
      routes = latestRoutes.routes
      pushDebugLog('success', `服务端一致性检查：拉取到 ${routes.length} 条域名映射`)
    } catch (error) {
      pushDebugLog('error', `域名映射拉取失败：${String(error)}`)
    }
    if (routes.length > 0) {
      pushDebugLog('success', `域名映射数量：${routes.length}`)
    } else {
      pushDebugLog('error', 'HTTP/HTTPS 隧道未配置域名映射')
    }
  }

  if (isLocalTunnel(tunnel)) {
    pushDebugLog('success', '该隧道绑定的是本机 Agent')
  } else {
    pushDebugLog('info', `该隧道绑定的不是当前本机 Agent，agent_id=${tunnel.agent_id}`)
  }

  if (isTauri()) {
    try {
      const localProbe = await invoke<ProbeResult>('probe_tcp_endpoint', {
        host: tunnel.local_host,
        port: tunnel.local_port,
      })
      pushDebugLog(localProbe.ok ? 'success' : 'error', `本地端口检查 ${localProbe.address}：${localProbe.message}`)
    } catch (error) {
      pushDebugLog('error', `本地端口检查失败：${String(error)}`)
    }

    try {
      const status = await invoke<NativeAgentStatus>('agent_status')
      if (status.running) {
        pushDebugLog('success', `本地 Agent 运行中，pid=${status.pid ?? '--'}`)
      } else {
        pushDebugLog('warn', `本地 Agent 未运行，lastExit=${status.lastExit ?? '--'}`)
      }
    } catch (error) {
      pushDebugLog('error', `读取本地 Agent 状态失败：${String(error)}`)
    }

    for (const target of targets) {
      try {
        if (target.startsWith('http://') || target.startsWith('https://')) {
          const result = await invoke<ProbeResult>('probe_http_endpoint', { url: target })
          pushDebugLog(result.ok ? 'success' : 'error', `外部映射检查 ${result.address}：${result.message}`)
        } else {
          const [host, portText] = target.split(':')
          const port = Number(portText)
          const result = await invoke<ProbeResult>('probe_tcp_endpoint', { host, port })
          pushDebugLog(result.ok ? 'success' : 'error', `外部映射检查 ${result.address}：${result.message}`)
        }
      } catch (error) {
        pushDebugLog('error', `外部映射检查失败 ${target}：${String(error)}`)
      }
    }
  } else {
    pushDebugLog('warn', '当前不是 Tauri 环境，无法检查本地 Agent 进程状态')
  }

  if (!store.session.baseUrl.trim()) {
    pushDebugLog('error', '服务端地址为空')
  } else {
    pushDebugLog('success', `服务端地址：${store.session.baseUrl}`)
  }

  if (!store.session.userId.trim()) {
    pushDebugLog('error', '当前登录 user_id 为空')
  } else {
    pushDebugLog('success', `当前用户：${store.session.userId}`)
  }

  pushDebugLog('info', '诊断完成。若隧道无法访问，请继续检查本地服务是否真的在监听该端口。')
  debugRunning.value = false
}

function handleTunnelAction(command: 'edit' | 'delete' | 'debug', tunnel: Tunnel) {
  if (command === 'edit') {
    openEditTunnelDialog(tunnel)
    return
  }
  if (command === 'debug') {
    void runTunnelDebug(tunnel)
    return
  }
  void removeTunnel(tunnel)
}

const sortedTunnels = computed(() => {
  return [...tunnels.value].sort((a, b) => {
    const aIsLocal = isLocalTunnel(a)
    const bIsLocal = isLocalTunnel(b)
    if (aIsLocal === bIsLocal) {
      return 0
    }
    return aIsLocal ? -1 : 1
  })
})

function getTunnelAccessTargets(tunnel: Tunnel) {
  if (tunnel.type === 'tcp') {
    if (!tunnel.access_target) {
      return []
    }
    return [tunnel.access_target]
  }
  return (domainRoutes.value[tunnel.id] || [])
    .map((route) => route.access_url || `${route.scheme}://${route.domain}`)
}

function findDomainRouteByTarget(tunnelId: string, target: string) {
  return (domainRoutes.value[tunnelId] || []).find((route) => (route.access_url || `${route.scheme}://${route.domain}`) === target)
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`已复制：${text}`)
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : String(error)
  }
}

async function openAccessTarget(target: string) {
  try {
    if (isTauri()) {
      await openExternal(target)
      return
    }
    window.open(target, '_blank', 'noopener,noreferrer')
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : String(error)
  }
}

async function simpleAction(requestFn: () => Promise<unknown>, successMessage: string) {
  loading.value = true
  resetStatus()
  try {
    await requestFn()
    actionMessage.value = successMessage
    await loadAll()
  } catch (error) {
    actionError.value = error instanceof Error ? error.message : String(error)
    throw error
  } finally {
    loading.value = false
  }
}

async function refreshAgentStatus(options?: { compensateRegistration?: boolean; silent?: boolean }) {
  if (!isTauri()) {
    nativeState.value = {
      traySupported: false,
      agentRunning: false,
      agentExecutablePath: '',
      agentArguments: [],
      agentPid: null,
      lastExit: '',
    }
    return
  }

  try {
    const result = await invoke<NativeAgentStatus>('agent_status')
    applyNativeAgentStatus(result, agentForm.executablePath)
    if (!agentForm.executablePath) {
      agentForm.executablePath = result.executablePath
    }
    if (options?.compensateRegistration) {
      await compensateAgentRegistrationIfNeeded(result)
    }
  } catch (error) {
    nativeState.value = {
      traySupported: true,
      agentRunning: false,
      agentExecutablePath: agentForm.executablePath,
      agentArguments: [],
      agentPid: null,
      lastExit: '',
    }
    if (!options?.silent) {
      actionError.value = error instanceof Error ? error.message : String(error)
    }
  }

  if (!agentForm.userId) {
    agentForm.userId = userId.value
  }
}

async function startLocalAgent() {
  if (!isTauri()) return
  loading.value = true
  resetStatus()
  try {
    const result = await invoke<NativeAgentStatus>('start_local_agent', {
      input: {
        executablePath: agentForm.executablePath,
        arguments: buildAgentArguments(),
      },
    })
    applyNativeAgentStatus(result, agentForm.executablePath)
    await compensateAgentRegistrationIfNeeded(result)
    actionMessage.value = '本地 agent 已启动。'
  } finally {
    loading.value = false
  }
}

async function stopLocalAgent() {
  if (!isTauri()) return
  loading.value = true
  resetStatus()
  try {
    const result = await invoke<NativeAgentStatus>('stop_local_agent')
    applyNativeAgentStatus(result, agentForm.executablePath)
    actionMessage.value = '本地 agent 已停止。'
  } finally {
    loading.value = false
  }
}

async function hideToTray() {
  if (!isTauri()) return
  await invoke('hide_to_tray')
}

async function showMainWindow() {
  if (!isTauri()) return
  await invoke('show_main_window_command')
}

function parseExtraAgentArguments() {
  return agentForm.extraArgsText
    .split(/\r?\n|(?<!\\)\s+/)
    .map((item: string) => item.trim())
    .filter(Boolean)
}

function buildAgentArguments() {
  const args = [
    '--server-url', agentForm.serverUrl,
    '--bridge-addr', agentForm.bridgeAddr,
    '--user-id', agentForm.userId,
    '--agent-name', agentForm.agentName,
    '--machine-code', agentForm.machineCode,
    '--client-version', agentForm.clientVersion,
    '--os-type', agentForm.osType,
    '--sync-interval', agentForm.syncInterval,
  ]

  return [...args, ...parseExtraAgentArguments()]
}

async function openAgentDirectory() {
  if (!isTauri()) return
  await invoke('open_agent_directory', {
    input: {
      executablePath: agentForm.executablePath,
    },
  })
}

function applySettingsDefaults() {
  baseUrl.value = store.settings.homeUrl
  agentForm.serverUrl = store.settings.homeUrl
  agentForm.bridgeAddr = store.settings.bridgeAddr
  agentForm.syncInterval = store.settings.defaultSyncInterval
  if (store.settings.agentExecutablePath) {
    agentForm.executablePath = store.settings.agentExecutablePath
  }
  actionMessage.value = '已应用设置里的默认值。'
}

function saveCurrentAsDefaults() {
  store.updateSetting('homeUrl', baseUrl.value)
  store.updateSetting('bridgeAddr', agentForm.bridgeAddr)
  store.updateSetting('defaultSyncInterval', agentForm.syncInterval)
  store.updateSetting('agentExecutablePath', agentForm.executablePath)
  actionMessage.value = '当前会话配置已保存为默认值。'
}

onMounted(() => {
  if (userId.value && !agentForm.userId) {
    agentForm.userId = userId.value
  }
  void ensureRegisteredAgent().catch((error) => {
    actionError.value = error instanceof Error ? error.message : String(error)
  })
  void loadAll()
  void refreshAgentStatus({ compensateRegistration: true })
  nativeAgentMonitorTimer = window.setInterval(() => {
    void refreshAgentStatus({ compensateRegistration: true, silent: true })
  }, 10000)
})

onBeforeUnmount(() => {
  if (nativeAgentMonitorTimer !== null) {
    window.clearInterval(nativeAgentMonitorTimer)
    nativeAgentMonitorTimer = null
  }
})

defineExpose({
  reload: loadAll,
})
</script>

<template>
  <div class="space-y-6">
    <el-alert v-if="actionError && !tcpDialogVisible && !hostDialogVisible" :closable="false" type="error" :title="actionError" />
    <template v-if="page === 'tunnels'">
      <section class="mb-4 flex items-center justify-between gap-3">
        <el-button :loading="loading" @click="refreshWorkspace">
          <span class="i-mdi-refresh mr-1"></span>
          刷新
        </el-button>
        <div class="flex gap-3">
        <el-button type="primary" @click="openCreateTcpDialog">创建 TCP Tunnel</el-button>
        <el-button type="primary" @click="openCreateHostDialog">创建 HTTP/HTTPS Tunnel</el-button>
        </div>
      </section>

      <el-dialog v-model="tcpDialogVisible" :title="editingTunnelId ? '编辑 TCP Tunnel' : '创建 TCP Tunnel'" width="500" @opened="focusTunnelNameInput('tcp')">
        <el-form label-width="100" @submit.prevent="async () => { if (await saveTcpTunnelAction()) tcpDialogVisible = false }">
          <el-alert v-if="tcpDialogError" :closable="false" type="error" :title="tcpDialogError" class="mb-4" />
          <el-form-item label="名称"><el-input ref="tcpNameInputRef" v-model="tcpForm.name" /></el-form-item>
          <el-form-item label="本地 Host"><el-input v-model="tcpForm.localHost" /></el-form-item>
          <el-form-item label="本地 Port">
            <div class="w-full space-y-2">
              <el-input v-model="tcpForm.localPort" />
              <div class="flex flex-wrap gap-2">
                <el-tag
                  v-for="port in commonTcpPorts"
                  :key="port"
                  class="cursor-pointer select-none"
                  :type="tcpForm.localPort === port ? 'primary' : 'info'"
                  @click="tcpForm.localPort = port"
                >
                  {{ port }}
                </el-tag>
              </div>
            </div>
          </el-form-item>
          <el-form-item><el-button type="primary" native-type="submit" :loading="loading">{{ editingTunnelId ? '保存' : '创建' }}</el-button></el-form-item>
        </el-form>
      </el-dialog>

      <el-dialog v-model="hostDialogVisible" :title="editingTunnelId ? '编辑 HTTP/HTTPS Tunnel' : '创建 HTTP/HTTPS Tunnel'" width="500" @opened="focusTunnelNameInput('host')">
        <el-form label-width="100" @submit.prevent="async () => { if (await saveHostTunnelAction()) hostDialogVisible = false }">
          <el-alert v-if="hostDialogError" :closable="false" type="error" :title="hostDialogError" class="mb-4" />
          <el-form-item label="名称"><el-input ref="hostNameInputRef" v-model="hostForm.name" /></el-form-item>
          <el-form-item label="本地 Host"><el-input v-model="hostForm.localHost" /></el-form-item>
          <el-form-item label="本地 Port">
            <div class="w-full space-y-2">
              <el-input v-model="hostForm.localPort" />
              <div class="flex flex-wrap gap-2">
                <el-tag
                  v-for="port in commonHostPorts"
                  :key="port"
                  class="cursor-pointer select-none"
                  :type="hostForm.localPort === port ? 'primary' : 'info'"
                  @click="hostForm.localPort = port"
                >
                  {{ port }}
                </el-tag>
              </div>
            </div>
          </el-form-item>
          <el-form-item label="域名前缀">
            <div class="w-full space-y-2">
              <el-input v-model="hostForm.domain" placeholder="留空则自动生成" />
              <div class="text-xs text-[var(--text-soft)]">
                最终域名预览：`{{ hostFullDomainPreview }}`。固定域名后缀由服务端配置控制。
              </div>
            </div>
          </el-form-item>
          <el-form-item><el-button type="primary" native-type="submit" :loading="loading">{{ editingTunnelId ? '保存' : '创建' }}</el-button></el-form-item>
        </el-form>
      </el-dialog>

      <section>
        <el-table :data="sortedTunnels" style="width: 100%">
          <el-table-column prop="name" label="名称" width="200">
            <template #default="{ row }">
              <div class="flex w-full items-center gap-2 min-w-0">
                <el-tooltip v-if="isLocalTunnel(row)" content="这个是本机的" placement="top">
                  <span class="inline-flex h-5 w-5 items-center justify-center rounded-full bg-emerald-100 text-xs font-semibold text-emerald-700">
                    本
                  </span>
                </el-tooltip>
                <el-input
                  v-if="inlineEditingTunnelId === row.id"
                  v-model="inlineTunnelName"
                  size="small"
                  autofocus
                  @blur="saveInlineTunnelName(row)"
                  @keyup.enter="saveInlineTunnelName(row)"
                  @keyup.esc="cancelInlineEditName"
                />
                <button
                  v-else
                  class="truncate text-left transition-colors hover:text-[var(--brand)]"
                  type="button"
                  :disabled="inlineSavingTunnelId === row.id"
                  @click="startInlineEditName(row)"
                >
                  {{ row.name }}
                </button>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="映射">
            <template #default="{ row }">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-[var(--text-strong)]">{{ row.local_host }}:{{ row.local_port }}</span>
                <span class="text-[var(--text-soft)]">→</span>
                <span v-if="getTunnelAccessTargets(row).length === 0" class="text-[var(--text-soft)]">--</span>
                <template v-for="target in getTunnelAccessTargets(row)" :key="target">
                  <el-tag v-if="row.type === 'tcp'">{{ target }}</el-tag>
                  <el-tag
                    v-else
                    closable
                    class="cursor-pointer select-none"
                    @click="openAccessTarget(target)"
                    @close="findDomainRouteByTarget(row.id, target) && removeDomainRoute(findDomainRouteByTarget(row.id, target)!)"
                  >
                    {{ target }}
                  </el-tag>
                  <el-button text type="primary" @click="copyText(target)">
                    <span class="i-mdi-content-copy text-sm"></span>
                  </el-button>
                </template>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="60">
            <template #default="{ row }">
              <div class="flex justify-center">
                <el-dropdown trigger="hover" @command="(command: string) => handleTunnelAction(command as 'edit' | 'delete' | 'debug', row)">
                  <button
                    class="flex h-8 w-8 items-center justify-center rounded-full text-[var(--text-soft)] transition-colors hover:bg-[var(--brand-soft)] hover:text-[var(--brand)]"
                    type="button"
                  >
                    <span class="i-mdi-dots-horizontal text-lg"></span>
                  </button>
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item command="debug">调试</el-dropdown-item>
                      <el-dropdown-item command="edit">编辑</el-dropdown-item>
                      <el-dropdown-item command="delete">删除</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <el-dialog v-model="debugDialogVisible" title="隧道调试" width="760">
        <div class="space-y-4">
          <div class="rounded-xl border border-[var(--line)] bg-[var(--surface)] px-4 py-3 text-sm">
            <div class="font-semibold text-[var(--text-strong)]">
              {{ debuggingTunnel?.name || '未选择隧道' }}
            </div>
            <div class="mt-1 text-[var(--text-soft)]">
              {{ debuggingTunnel ? `${debuggingTunnel.local_host}:${debuggingTunnel.local_port}` : '--' }}
            </div>
          </div>

          <div class="h-[420px] overflow-auto rounded-xl border border-[var(--line)] bg-slate-950 px-4 py-3 font-mono text-xs leading-6 text-slate-100">
            <div v-if="debugLogs.length === 0" class="text-slate-400">暂无调试日志</div>
            <div v-for="(logItem, index) in debugLogs" :key="index" class="break-all">
              <span
                :class="{
                  'text-sky-300': logItem.level === 'info',
                  'text-emerald-300': logItem.level === 'success',
                  'text-amber-300': logItem.level === 'warn',
                  'text-rose-300': logItem.level === 'error',
                }"
              >
                [{{ logItem.level.toUpperCase() }}]
              </span>
              <span class="ml-2">{{ logItem.message }}</span>
            </div>
          </div>

          <div class="flex justify-end gap-3">
            <el-button @click="debugDialogVisible = false">关闭</el-button>
            <el-button type="primary" :loading="debugRunning" :disabled="!debuggingTunnel" @click="debuggingTunnel && runTunnelDebug(debuggingTunnel)">
              重新诊断
            </el-button>
          </div>
        </div>
      </el-dialog>
    </template>
  </div>
</template>
