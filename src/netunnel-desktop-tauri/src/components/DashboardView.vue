<script setup lang="ts">
import { ElMessage } from 'element-plus'
import QRCode from 'qrcode'
import NetunnelWorkspace from '@/components/NetunnelWorkspace.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import { createApiClient } from '@/services/api'
import { fetchDashboardSummary } from '@/services/dashboard'
import { fetchBillingProfile, fetchPricingRules, fetchBusinessRecords } from '@/services/billing'
import { createPaymentOrder, pollPaymentOrder, type PaymentOrderSnapshot } from '@/services/payments'
import { fetchUserProfile } from '@/services/users'
import { useWindowControls } from '@/composables/useWindowControls'
import { log } from '@/services/logger'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { invoke, isTauri } from '@tauri-apps/api/core'
import type { BillingProfile, PricingRule, UserBusinessRecord } from '@/types/netunnel'

const store = useStore()
const workspaceRef = ref<InstanceType<typeof NetunnelWorkspace> | null>(null)
const rechargeDialogVisible = ref(false)
const businessRecordsDialogVisible = ref(false)
const paymentDialogVisible = ref(false)
const billingLoading = ref(false)
const businessRecordsLoading = ref(false)
const pricingRules = ref<PricingRule[]>([])
const billingProfile = ref<BillingProfile | null>(null)
const businessRecords = ref<UserBusinessRecord[]>([])
const businessRecordsPage = ref(1)
const businessRecordsPageSize = ref(10)
const trafficRechargeOptions = [10, 50, 100] as const
const TRAFFIC_PAYMENT_PRODUCT_IDS: Record<(typeof trafficRechargeOptions)[number], string> = {
  10: 'cmn1vuv3k008p5cdwku0vbvmc',
  50: 'cmn1vv5bi008r5cdw7wq2xquv',
  100: 'cmn1vvk6w008t5cdwewom8m1z',
}
const MONTHLY_PAYMENT_PRODUCT_ID = 'cmn1vwkus008v5cdw15hyi1h1'
const YEARLY_PAYMENT_PRODUCT_ID = 'cmn1vwu70008x5cdwxhf3njnl'
const PAYMENT_POLL_INTERVAL_MS = 2500
const workspaceStorageKey = 'netunnel-desktop-tauri-workspace'
const localAgentState = ref({
  running: false,
  executablePath: '',
  pid: null as number | null,
  lastExit: '',
  registeredAgentId: '',
})
let agentStatusTimer: number | null = null
let paymentPollTimer: ReturnType<typeof setInterval> | null = null

const { isWindowMaximized, minimizeWindow, toggleMaximizeWindow, closeWindow, startDraggingWindow } = useWindowControls()

const getSessionIconClass = (icon: string) => `i-mdi-${icon}`

const currentQuotaBytes = computed(() => {
  if (!billingProfile.value) return 0
  if (billingProfile.value.pricing_rule.is_unlimited) return 0
  return billingProfile.value.pricing_rule.included_traffic_bytes
})

const trafficUsageLabel = computed(() => {
  if (!store.summary) {
    return '--'
  }
  if (!billingProfile.value) {
    return formatBytes(store.summary.recentTrafficBytes)
  }
  if (billingProfile.value.pricing_rule.is_unlimited) {
    return `${formatBytes(store.summary.recentTrafficBytes)} / 不限量`
  }
  if (billingProfile.value.pricing_rule.included_traffic_bytes > 0) {
    return `${formatBytes(store.summary.recentTrafficBytes)} / ${formatBytes(billingProfile.value.pricing_rule.included_traffic_bytes)}`
  }
  return `${formatBytes(store.summary.recentTrafficBytes)} / 按量计费`
})

const trafficUsagePercent = computed(() => {
  if (!store.summary || !billingProfile.value) return 0
  const quotaBytes = billingProfile.value.pricing_rule.included_traffic_bytes
  if (billingProfile.value.pricing_rule.is_unlimited || quotaBytes <= 0) {
    return 100
  }
  return Math.min((store.summary.recentTrafficBytes / quotaBytes) * 100, 100)
})

const agentStatusDotClass = computed(() => (localAgentState.value.running ? 'bg-green-500' : 'bg-red-500'))

const agentStatusTooltip = computed(() => {
  const lines = [localAgentState.value.running ? '本地 agent 运行中' : '本地 agent 未运行']

  if (localAgentState.value.executablePath) {
    lines.push(`可执行文件: ${localAgentState.value.executablePath}`)
  }
  if (localAgentState.value.pid !== null) {
    lines.push(`PID: ${localAgentState.value.pid}`)
  }
  if (localAgentState.value.registeredAgentId) {
    lines.push(`已注册 Agent ID: ${localAgentState.value.registeredAgentId}`)
    if (localAgentState.value.running) {
      lines.push(`本地 agent 运行中，已完成补偿注册：${localAgentState.value.registeredAgentId}`)
    }
  }
  if (localAgentState.value.lastExit) {
    lines.push(`最近退出: ${localAgentState.value.lastExit}`)
  }

  return lines.join('\n')
})

interface FixedPricingPlan {
  key: 'traffic' | 'month' | 'year'
  title: string
  description: string
  priceLabel: string
  actionLabel: string
  rule?: PricingRule
  current: boolean
}

const monthlyPricingRule = computed(() =>
  pricingRules.value.find(
    (rule) => rule.billing_mode === 'subscription' && rule.subscription_period === 'month' && rule.is_unlimited,
  ),
)

const yearlyPricingRule = computed(() =>
  pricingRules.value.find(
    (rule) => rule.billing_mode === 'subscription' && rule.subscription_period === 'year' && rule.is_unlimited,
  ),
)

const trafficPricingRule = computed(() =>
  pricingRules.value.find((rule) => rule.billing_mode === 'traffic') ??
  pricingRules.value.find((rule) => rule.name === 'default-traffic'),
)
const paymentQRCodeDataUrl = ref('')
const paymentSnapshot = ref<PaymentOrderSnapshot | null>(null)
const paymentStatus = ref<'idle' | 'pending' | 'paid' | 'expired' | 'closed' | 'error'>('idle')
const paymentMessage = ref('请选择套餐并发起支付')

const fixedPricingPlans = computed<FixedPricingPlan[]>(() => [
  {
    key: 'traffic',
    title: '按流量充值',
    description: '无到期时间，优先使用包年包月套餐。',
    priceLabel: `${formatSingleDecimalAmount(trafficPricingRule.value?.price_per_gb || '1')} 元 / GB`,
    actionLabel: '快捷充值',
    current: billingProfile.value?.pricing_rule.billing_mode === 'traffic',
  },
  {
    key: 'month',
    title: '不限量包月',
    description: '不限量包月套餐，固定 5 元。未到期续费，将会延长到期时间。',
    priceLabel: monthlyPricingRule.value ? `${formatPricingAmount(monthlyPricingRule.value.subscription_price)} / 月` : '--',
    actionLabel: monthlyPricingRule.value ? '续费购买' : '暂不可用',
    rule: monthlyPricingRule.value,
    current: billingProfile.value?.pricing_rule.id === monthlyPricingRule.value?.id,
  },
  {
    key: 'year',
    title: '不限量包年',
    description: '不限量包年套餐，固定 40 元。未到期续费，将会延长到期时间。',
    priceLabel: yearlyPricingRule.value ? `${formatPricingAmount(yearlyPricingRule.value.subscription_price)} / 年` : '--',
    actionLabel: yearlyPricingRule.value ? '续费购买' : '暂不可用',
    rule: yearlyPricingRule.value,
    current: billingProfile.value?.pricing_rule.id === yearlyPricingRule.value?.id,
  },
])

const handleHeaderMouseDown = (event: MouseEvent) => {
  const target = event.target as HTMLElement | null
  if (!target || target.closest('button, input, textarea, select, a')) {
    return
  }

  void startDraggingWindow()
}

async function loadSummary() {
  log('INFO', `loadSummary called, userId=${store.session.userId}, accessToken=${store.session.accessToken ? 'exists' : 'missing'}, baseUrl=${store.session.baseUrl}`)
  if (!store.session.userId || !store.session.accessToken) {
    log('WARN', 'loadSummary skipped: userId or accessToken is empty')
    return
  }
  try {
    log('INFO', `fetchDashboardSummary start, baseUrl=${store.session.baseUrl}`)
    const client = createApiClient({ baseUrl: store.session.baseUrl, accessToken: store.session.accessToken })
    const res = await fetchDashboardSummary(client, store.session.userId)
    log('INFO', `fetchDashboardSummary success: ${JSON.stringify(res.summary)}`)
    store.summary = {
      onlineAgents: res.summary.online_agents,
      totalAgents: res.summary.total_agents,
      enabledTunnels: res.summary.enabled_tunnels,
      totalTunnels: res.summary.total_tunnels,
      recentTrafficBytes: res.summary.recent_traffic_bytes_24h,
    }
  } catch (e) {
    log('ERROR', `fetchDashboardSummary failed: ${e}`)
  }
}

async function autoStartAgent() {
  if (!store.session.userId || !store.session.accessToken) {
    log('WARN', 'autoStartAgent skipped: not logged in yet')
    return
  }

  type NativeAgentStatus = {
    running: boolean
    executablePath?: string
    arguments?: string[]
    pid?: number | null
    lastExit?: string | null
  }

  async function launchAgent(machineCode: string) {
    const serverUrl = store.session.baseUrl || store.settings.homeUrl || 'http://127.0.0.1:40061'
    const bridgeAddr = store.settings.bridgeAddr || '127.0.0.1:40062'
    const userId = store.session.userId
    const args = [
      '-server-url', serverUrl,
      '-bridge-addr', bridgeAddr,
      '-user-id', userId,
      '-agent-name', 'desktop-agent',
      '-machine-code', machineCode,
      '-client-version', '0.1.0',
      '-os-type', 'windows',
      '-sync-interval', '10',
    ]
    const agentExe = 'D:/git-projects/ai-company/projects/netunnel/src/netunnel-agent/agent-run.exe'
    log('INFO', `autoStartAgent: starting agent with args: ${JSON.stringify(args)}`)
    await invoke('start_local_agent', {
      input: {
        executablePath: agentExe,
        arguments: args,
      },
    })
  }

  try {
    log('INFO', 'autoStartAgent: checking agent status')
    const status = await invoke<NativeAgentStatus>('agent_status')
    if (status.running) {
      log('INFO', 'autoStartAgent: agent already running')
      return
    }
  } catch {
    log('WARN', 'autoStartAgent: could not check status, trying to start')
  }

  try {
    const machineCode = await invoke<string>('get_or_create_agent_id')
    await launchAgent(machineCode)
    log('INFO', 'autoStartAgent: agent started successfully')
  } catch (e) {
    log('ERROR', `autoStartAgent failed: ${e}`)
  }
}

function readWorkspaceAgentId() {
  if (typeof window === 'undefined') {
    return ''
  }

  try {
    const raw = window.localStorage.getItem(workspaceStorageKey)
    if (!raw) {
      return ''
    }
    const parsed = JSON.parse(raw) as { registeredAgentId?: string }
    return parsed.registeredAgentId ?? ''
  } catch {
    return ''
  }
}

async function refreshLocalAgentStatus() {
  localAgentState.value.registeredAgentId = readWorkspaceAgentId()

  if (!isTauri()) {
    localAgentState.value.running = false
    localAgentState.value.executablePath = ''
    localAgentState.value.pid = null
    localAgentState.value.lastExit = ''
    return
  }

  try {
    const status = await invoke<{
      running: boolean
      executablePath?: string
      pid?: number | null
      lastExit?: string | null
    }>('agent_status')

    localAgentState.value.running = status.running
    localAgentState.value.executablePath = status.executablePath ?? ''
    localAgentState.value.pid = status.pid ?? null
    localAgentState.value.lastExit = status.lastExit ?? ''
  } catch (error) {
    localAgentState.value.running = false
    localAgentState.value.pid = null
    localAgentState.value.lastExit = String(error)
  }
}

function createBillingClient() {
  return createApiClient({ baseUrl: store.session.baseUrl, accessToken: store.session.accessToken })
}

async function loadBillingProfile() {
  if (!store.session.userId || !store.session.accessToken) {
    return
  }
  try {
    const profile = await fetchBillingProfile(createBillingClient(), store.session.userId)
    billingProfile.value = profile
    store.user.plan = formatPricingRuleLabel(profile.pricing_rule)
  } catch (error) {
    log('ERROR', `fetchBillingProfile failed: ${error}`)
  }
}

async function loadUserProfile() {
  if (!store.session.userId || !store.session.accessToken) {
    return
  }
  try {
    const profile = await fetchUserProfile(createBillingClient(), store.session.userId)
    store.user.name = profile.user.nickname || store.user.name
    store.user.avatarUrl = profile.user.avatar_url || ''
    if (profile.user.email) {
      store.user.email = profile.user.email
    }
  } catch (error) {
    log('ERROR', `fetchUserProfile failed: ${error}`)
  }
}

async function loadPricingPlans() {
  const response = await fetchPricingRules(createBillingClient())
  pricingRules.value = response.pricing_rules
}

function formatBusinessRecordType(recordType: string) {
  switch (recordType) {
    case 'traffic_recharge':
      return '流量充值'
    case 'subscription_purchase':
      return '套餐购买'
    case 'subscription_renew':
      return '套餐续费'
    case 'subscription_traffic_settlement':
      return '套餐内流量'
    case 'traffic_settlement':
      return '流量扣减'
    default:
      return recordType || '--'
  }
}

function isLegacyIncludedTrafficRecord(row: UserBusinessRecord) {
  return row.record_type === 'traffic_settlement' && Number(row.change_amount) === 0
}

function formatTrafficValue(bytes?: number) {
  if (!bytes) {
    return '--'
  }
  return formatTrafficAmount(bytes)
}

function formatTrafficBalance(value?: string | number) {
  if (value === undefined || value === null || value === '') {
    return '--'
  }
  const bytes = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(bytes)) {
    return String(value)
  }
  return formatTrafficAmount(bytes)
}

function formatDateTime(value?: string) {
  if (!value) {
    return '--'
  }
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) {
    return value
  }
  return parsed.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

async function openRechargeDialog() {
  if (!store.session.userId || !store.session.accessToken) {
    return
  }
  billingLoading.value = true
  try {
    await Promise.all([loadBillingProfile(), loadPricingPlans()])
    rechargeDialogVisible.value = true
  } finally {
    billingLoading.value = false
  }
}

async function openBusinessRecordsDialog() {
  if (!store.session.userId || !store.session.accessToken) {
    return
  }
  businessRecordsLoading.value = true
  try {
    const response = await fetchBusinessRecords(createBillingClient(), store.session.userId, 100)
    businessRecords.value = response.business_records
    businessRecordsPage.value = 1
    businessRecordsDialogVisible.value = true
  } catch (error) {
    ElMessage.error(String(error))
  } finally {
    businessRecordsLoading.value = false
  }
}

function stopPaymentPolling() {
  if (!paymentPollTimer) {
    return
  }
  clearInterval(paymentPollTimer)
  paymentPollTimer = null
}

async function renderPaymentQRCode(url: string) {
  paymentQRCodeDataUrl.value = await QRCode.toDataURL(url, {
    width: 220,
    margin: 1,
  })
}

function closePaymentDialog() {
  stopPaymentPolling()
  paymentDialogVisible.value = false
}

function formatPaymentAmount(amount?: number) {
  if (typeof amount !== 'number' || Number.isNaN(amount)) {
    return '--'
  }
  return `¥${(amount / 100).toFixed(2)}`
}

function paymentStatusLabel(status: typeof paymentStatus.value) {
  switch (status) {
    case 'pending':
      return '待支付'
    case 'paid':
      return '支付成功'
    case 'expired':
      return '已过期'
    case 'closed':
      return '已关闭'
    case 'error':
      return '异常'
    default:
      return '未开始'
  }
}

async function syncPaymentStatus(bizId: string, options?: { silent?: boolean }) {
  const snapshot = await pollPaymentOrder(createBillingClient(), bizId)
  paymentSnapshot.value = snapshot

  const sessionStatus = snapshot.session?.status ?? snapshot.order.platform_status
  paymentStatus.value = (sessionStatus as typeof paymentStatus.value) || 'pending'

  if (sessionStatus === 'paid') {
    if (snapshot.applied) {
      paymentMessage.value = '支付成功，套餐已生效。'
      stopPaymentPolling()
      await Promise.all([loadBillingProfile(), loadSummary(), loadPricingPlans()])
      ElMessage.success('支付成功')
      return
    }
    paymentMessage.value = snapshot.applyError || '支付成功，正在同步业务订单...'
    return
  }

  if (sessionStatus === 'expired') {
    paymentMessage.value = '二维码已过期，请重新发起支付。'
    stopPaymentPolling()
    return
  }

  if (sessionStatus === 'closed') {
    paymentMessage.value = '支付已关闭，请重新发起支付。'
    stopPaymentPolling()
    return
  }

  paymentMessage.value = '请使用微信扫码完成支付。'
  if (!options?.silent && snapshot.applyError) {
    ElMessage.warning(snapshot.applyError)
  }
}

async function startPaymentFlow(payload: {
  order_type: 'traffic_recharge' | 'pricing_rule'
  payment_product_id: string
  pricing_rule_id?: string
  recharge_gb?: number
}) {
  if (!store.session.userId) return

  billingLoading.value = true
  try {
    paymentSnapshot.value = null
    paymentQRCodeDataUrl.value = ''
    paymentStatus.value = 'pending'
    paymentMessage.value = '正在创建支付订单...'

    const snapshot = await createPaymentOrder(createBillingClient(), {
      user_id: store.session.userId,
      ...payload,
    })
    paymentSnapshot.value = snapshot
    rechargeDialogVisible.value = false
    paymentDialogVisible.value = true
    paymentStatus.value = snapshot.session?.status ?? 'pending'
    paymentMessage.value = '请使用微信扫码完成支付。'

    const qrSource = snapshot.session?.qrCodeUrl || snapshot.session?.checkoutUrl
    if (!qrSource) {
      throw new Error('支付二维码生成失败')
    }
    await renderPaymentQRCode(qrSource)

    stopPaymentPolling()
    paymentPollTimer = setInterval(() => {
      void syncPaymentStatus(snapshot.order.biz_id, { silent: true }).catch((error) => {
        paymentStatus.value = 'error'
        paymentMessage.value = String(error)
        stopPaymentPolling()
      })
    }, PAYMENT_POLL_INTERVAL_MS)
  } catch (error) {
    paymentStatus.value = 'error'
    paymentMessage.value = String(error)
    ElMessage.error(String(error))
  } finally {
    billingLoading.value = false
  }
}

async function purchaseTrafficPlan(amountGb: (typeof trafficRechargeOptions)[number]) {
  await startPaymentFlow({
    order_type: 'traffic_recharge',
    payment_product_id: TRAFFIC_PAYMENT_PRODUCT_IDS[amountGb],
    recharge_gb: amountGb,
  })
}

async function purchasePricingRule(rule?: PricingRule) {
  if (!rule) {
    ElMessage.warning('当前没有可购买的套餐，请稍后再试')
    return
  }

  await startPaymentFlow({
    order_type: 'pricing_rule',
    payment_product_id: rule.subscription_period === 'year' ? YEARLY_PAYMENT_PRODUCT_ID : MONTHLY_PAYMENT_PRODUCT_ID,
    pricing_rule_id: rule.id,
  })
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function formatIntegerAmount(amount: string | number) {
  const numericAmount = typeof amount === 'number' ? amount : Number(amount)
  if (!Number.isFinite(numericAmount)) {
    return String(amount)
  }
  return `${Math.round(numericAmount)}`
}

function formatPricingAmount(amount: string | number) {
  return `${formatIntegerAmount(amount)} 元`
}

function formatSingleDecimalAmount(amount: string | number) {
  const numericAmount = typeof amount === 'number' ? amount : Number(amount)
  if (!Number.isFinite(numericAmount)) {
    return String(amount)
  }
  return numericAmount.toFixed(1)
}

function formatTrafficAmount(bytes: number) {
  if (bytes <= 0) {
    return '0 B'
  }
  if (bytes < 1024) {
    return `${bytes} B`
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`
  }
  if (bytes < 1024 * 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`
  }
  return `${(bytes / 1024 / 1024 / 1024).toFixed(3)} GB`
}

const remainingTrafficLabel = computed(() => {
  if (!billingProfile.value) {
    return '--'
  }

  return formatTrafficBalance(billingProfile.value.account.balance)
})

const expiryLabel = computed(() => {
  if (!billingProfile.value) {
    return '--'
  }

  const expiresAt = billingProfile.value.subscription?.expires_at
  if (!expiresAt) {
    return '--'
  }

  const parsed = new Date(expiresAt)
  if (Number.isNaN(parsed.getTime())) {
    return expiresAt
  }
  return parsed.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
})

const pagedBusinessRecords = computed(() => {
  const start = (businessRecordsPage.value - 1) * businessRecordsPageSize.value
  return businessRecords.value.slice(start, start + businessRecordsPageSize.value)
})

function formatPricingRuleLabel(rule: PricingRule) {
  if (rule.display_name) {
    return rule.display_name
  }
  if (rule.billing_mode === 'traffic') {
    return `按量 ${rule.price_per_gb}/GB`
  }
  const periodLabel = rule.subscription_period === 'year' ? '包年' : '包月'
  if (rule.is_unlimited) {
    return `${periodLabel} 不限量`
  }
  return `${periodLabel} ${formatBytes(rule.included_traffic_bytes)}`
}

onMounted(() => {
  log('INFO', 'DashboardView mounted')
  void loadSummary()
  void loadBillingProfile()
  void loadUserProfile()
  void autoStartAgent()
  void refreshLocalAgentStatus()
  agentStatusTimer = window.setInterval(() => {
    void refreshLocalAgentStatus()
  }, 10000)
})

onBeforeUnmount(() => {
  if (agentStatusTimer !== null) {
    window.clearInterval(agentStatusTimer)
    agentStatusTimer = null
  }
  stopPaymentPolling()
})
</script>

<template>
  <div class="dashboard-shell">
    <aside class="dashboard-sidebar dashboard-sidebar--compact dashboard-sidebar--design">
      <div class="dashboard-brand">
        <div class="dashboard-brand__icon">
          <span class="i-mdi-transit-connection-variant text-xl"></span>
        </div>
        <h1 class="dashboard-brand__title">Netunnel Desktop</h1>
      </div>

      <div class="flex-1 overflow-auto px-2">
        <div class="space-y-3 py-2">
          <div class="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
            <p class="text-xs text-[var(--text-muted)]">在线 Agent</p>
            <p class="mt-1 text-xl font-bold">
              <span v-if="store.summary">{{ store.summary.onlineAgents }} / {{ store.summary.totalAgents }}</span>
              <span v-else class="text-[var(--text-muted)]">--</span>
            </p>
          </div>

          <div class="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
            <div class="flex items-center justify-between gap-3">
              <p class="text-xs text-[var(--text-muted)]">剩余流量</p>
              <button
                class="text-xs font-medium text-sky-600 transition-colors hover:text-sky-500"
                type="button"
                @click="openBusinessRecordsDialog"
              >
                明细
              </button>
            </div>
            <p class="mt-1 text-xl font-bold">
              {{ remainingTrafficLabel }}
            </p>
            <p class="mt-2 text-xs text-[var(--text-soft)]">
              不限额到期时间：{{ expiryLabel }}
            </p>
            <p class="mt-1 text-xs text-[var(--text-soft)]">
              套餐有效期内优先使用套餐，不扣减剩余流量余额。
            </p>
          </div>

          <button class="w-full rounded-xl bg-[var(--brand)] py-3 text-sm font-semibold text-white shadow-lg transition-all hover:opacity-90 active:scale-[0.98]" type="button" @click="openRechargeDialog">
            充值 / 购买套餐
          </button>

        </div>
      </div>

      <div class="border-t border-[var(--line)]">
        <button class="nav-link nav-link--design" type="button" @click="store.openSettingsModal()">
          <span class="i-mdi-cog-outline"></span>
          <span class="text-sm font-medium">设置</span>
        </button>

        <div class="flex items-center justify-between gap-3">
          <div class="flex items-center gap-3 min-w-0">
            <div class="w-8 h-8 rounded-full bg-[var(--brand-soft)] flex items-center justify-center overflow-hidden text-[var(--brand)]">
              <img v-if="store.user.avatarUrl" :src="store.user.avatarUrl" class="h-full w-full rounded-full object-cover" />
              <span v-else class="i-mdi-account-outline text-sm"></span>
            </div>
            <div class="flex flex-col min-w-0">
              <span class="text-xs font-semibold text-[var(--text-strong)]">{{ store.user.name }}</span>
            </div>
          </div>
          <button class="text-[11px] font-medium text-[var(--text-soft)] hover:text-[var(--brand)] transition-colors" type="button" @click="store.logout()">
            退出
          </button>
        </div>
      </div>
    </aside>

    <main class="dashboard-main dashboard-main--flat overflow-hidden" :class="{ 'is-blurred': store.isSettingsModalOpen }">
      <header class="dashboard-header dashboard-header--flat">
        <div class="dashboard-header__content flex items-center justify-between w-full" @mousedown.left="handleHeaderMouseDown">
          <div class="window-drag-region flex items-center gap-2 ml-4 flex-1 min-w-0" data-tauri-drag-region>
            <div class="flex min-w-0 flex-col">
              <span class="text-xs font-semibold uppercase tracking-[0.14em] text-[var(--text-muted)]">Workspace</span>
              <span class="text-sm font-semibold text-[var(--text-strong)]">{{ store.pageTitle }}</span>
            </div>
          </div>

          <div class="window-controls">
            <button class="window-control-button" type="button" @click="minimizeWindow">
              <span class="i-mdi-window-minimize"></span>
            </button>
            <button class="window-control-button" type="button" @click="toggleMaximizeWindow">
              <span :class="isWindowMaximized ? 'i-mdi-window-restore' : 'i-mdi-checkbox-blank-outline'"></span>
            </button>
            <button class="window-control-button window-control-button--close" type="button" @click="closeWindow">
              <span class="i-mdi-close"></span>
            </button>
          </div>
        </div>
      </header>

      <section
        v-if="store.updater.available && store.updater.promptVisible"
        class="mx-6 mt-4 rounded-3xl border border-emerald-500/20 bg-emerald-500/8 px-5 py-4 text-sm text-[var(--text-strong)]"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="space-y-1">
            <p class="font-semibold text-emerald-600">发现新版本 v{{ store.updater.available.version }}</p>
            <p class="text-[var(--text-soft)]">已在后台完成更新检查，你可以现在打开设置安装更新，也可以稍后再处理。</p>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <button class="nav-link nav-link--design" type="button" @click="store.openSettingsModal()">
              去安装
            </button>
            <button class="text-xs font-medium text-[var(--text-muted)] hover:text-[var(--text-strong)]" type="button" @click="store.dismissUpdatePrompt()">
              稍后
            </button>
          </div>
        </div>
      </section>

      <section class="min-h-0 flex-1 overflow-auto p-6">
        <NetunnelWorkspace ref="workspaceRef" :page="store.currentSession" @refresh-summary="loadSummary" />
      </section>

      <footer class="dashboard-footer dashboard-footer--flat">
        <div class="flex items-center gap-4">
          <el-tooltip placement="top-start">
            <template #content>
              <div class="whitespace-pre-line text-xs leading-6">{{ agentStatusTooltip }}</div>
            </template>
            <span class="flex items-center gap-1">
              <span class="w-2 h-2 rounded-full" :class="agentStatusDotClass"></span>
            </span>
          </el-tooltip>
        </div>
        <div class="flex items-center gap-4">
          <span>版本 {{ store.version }}</span>
          <span>QQ群：307460844</span>
        </div>
      </footer>
    </main>

    <div v-if="store.isSettingsModalOpen" class="modal-overlay" @click.self="store.closeSettingsModal()">
      <SettingsPanel mode="modal" @close="store.closeSettingsModal()" />
    </div>

    <el-dialog v-model="rechargeDialogVisible" title="充值与套餐购买" width="760">
      <div class="space-y-5">
        <section class="grid gap-4 md:grid-cols-2">
          <div class="rounded-2xl border border-[var(--line)] bg-[var(--surface)] p-4">
            <p class="text-xs uppercase tracking-[0.12em] text-[var(--text-muted)]">剩余流量</p>
            <p class="mt-2 text-2xl font-semibold text-[var(--text-strong)]">
              {{ remainingTrafficLabel }}
            </p>
            <p class="mt-2 text-sm text-[var(--text-soft)]">
              不限额到期时间：{{ expiryLabel }}
            </p>
            <p class="mt-1 text-xs text-[var(--text-soft)]">
              套餐有效期内优先使用套餐，不扣减剩余流量余额。
            </p>
          </div>
        </section>

        <section class="space-y-3">
          <div class="flex items-center justify-between">
            <p class="text-sm font-semibold text-[var(--text-strong)]">可购买套餐</p>
            <p class="text-xs text-[var(--text-muted)]">固定展示流量充值、包月、包年三档方案。</p>
          </div>

          <div class="grid gap-4 md:grid-cols-3">
            <article
              v-for="plan in fixedPricingPlans"
              :key="plan.key"
              class="rounded-2xl border border-[var(--line)] bg-[var(--surface)] p-4 transition-colors"
              :class="{ 'border-[var(--brand)]': plan.current }"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="text-base font-semibold text-[var(--text-strong)]">{{ plan.title }}</p>
                  <p class="mt-1 text-sm text-[var(--text-soft)]">{{ plan.description }}</p>
                </div>
              </div>

              <div v-if="plan.key === 'traffic'" class="mt-4 space-y-3">
                <div class="text-lg font-semibold text-[var(--brand)]">{{ plan.priceLabel }}</div>
                <div class="grid grid-cols-3 gap-2">
                  <el-button
                    v-for="amount in trafficRechargeOptions"
                    :key="amount"
                    type="primary"
                    plain
                    :loading="billingLoading"
                    @click="purchaseTrafficPlan(amount)"
                  >
                    {{ amount }}G
                  </el-button>
                </div>
              </div>

              <div v-else class="mt-4 flex items-center justify-between">
                <div class="text-lg font-semibold text-[var(--brand)]">{{ plan.priceLabel }}</div>
                <el-button
                  :type="plan.rule ? 'primary' : 'default'"
                  :disabled="!plan.rule"
                  :loading="billingLoading"
                  @click="purchasePricingRule(plan.rule)"
                >
                  {{ plan.current ? '续费购买' : plan.actionLabel }}
                </el-button>
              </div>
            </article>
          </div>
        </section>
      </div>
    </el-dialog>

    <el-dialog v-model="paymentDialogVisible" title="微信支付" width="420" @closed="closePaymentDialog">
      <div class="flex flex-col items-center gap-4 py-2">
        <div class="text-center">
          <p class="text-base font-semibold text-[var(--text-strong)]">
            {{ paymentSnapshot?.session?.paymentProduct.name || '待支付订单' }}
          </p>
          <p class="mt-1 text-sm text-[var(--text-soft)]">
            {{ paymentSnapshot?.session?.paymentProduct.description || paymentMessage }}
          </p>
          <p class="mt-1 text-sm font-medium text-[var(--brand)]">
            {{ formatPaymentAmount(paymentSnapshot?.session?.amount) }}
          </p>
        </div>

        <div class="flex h-[220px] w-[220px] items-center justify-center rounded-2xl border border-[var(--line)] bg-white p-3">
          <img v-if="paymentQRCodeDataUrl" :src="paymentQRCodeDataUrl" alt="支付二维码" class="h-full w-full object-contain" />
          <div v-else class="text-sm text-[var(--text-soft)]">二维码生成中...</div>
        </div>

        <div class="w-full rounded-2xl bg-[var(--brand-soft)]/40 px-4 py-3 text-sm text-[var(--text-soft)]">
          <p>状态：{{ paymentStatusLabel(paymentStatus) }}</p>
          <p v-if="paymentSnapshot?.session?.bizId" class="mt-1 break-all">订单号：{{ paymentSnapshot.session.bizId }}</p>
          <p v-if="paymentSnapshot?.session?.expiresAt" class="mt-1">过期时间：{{ paymentSnapshot.session.expiresAt }}</p>
          <p v-if="paymentSnapshot?.session?.paidAt" class="mt-1">支付时间：{{ paymentSnapshot.session.paidAt }}</p>
          <p class="mt-1 break-all">{{ paymentMessage }}</p>
        </div>

        <div class="flex w-full justify-end gap-3">
          <el-button @click="closePaymentDialog">关闭</el-button>
          <el-button
            type="primary"
            :disabled="!paymentSnapshot?.order?.biz_id || billingLoading"
            @click="paymentSnapshot?.order?.biz_id && syncPaymentStatus(paymentSnapshot.order.biz_id)"
          >
            刷新状态
          </el-button>
        </div>
      </div>
    </el-dialog>

    <el-dialog v-model="businessRecordsDialogVisible" title="用户业务记录" width="950">
      <div class="space-y-4">
        <el-table :data="pagedBusinessRecords" height="420" v-loading="businessRecordsLoading" style="width: 100%">
          <el-table-column prop="record_type" label="类型" min-width="120">
            <template #default="{ row }">{{ isLegacyIncludedTrafficRecord(row) ? '套餐内流量' : formatBusinessRecordType(row.record_type) }}</template>
          </el-table-column>
          <el-table-column prop="traffic_balance_after" label="剩余流量" min-width="110">
            <template #default="{ row }">{{ formatTrafficBalance(row.traffic_balance_after) }}</template>
          </el-table-column>
          <el-table-column prop="traffic_bytes" label="总结算流量" min-width="120">
            <template #default="{ row }">{{ formatTrafficValue(row.traffic_bytes) }}</template>
          </el-table-column>
          <el-table-column prop="billable_bytes" label="计费流量" min-width="120">
            <template #default="{ row }">{{ formatTrafficValue(row.billable_bytes) }}</template>
          </el-table-column>
          <el-table-column prop="package_expires_at" label="到期时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.package_expires_at) }}</template>
          </el-table-column>
          <el-table-column prop="created_at" label="时间" min-width="170">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>

        <div class="flex justify-end">
          <el-pagination
            v-model:current-page="businessRecordsPage"
            :page-size="businessRecordsPageSize"
            layout="prev, pager, next, total"
            :total="businessRecords.length"
          />
        </div>
      </div>
    </el-dialog>
  </div>
</template>
