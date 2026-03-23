import { computed, ref, watch } from "vue";
import {
  ApiError,
  bootstrapUser,
  createHttpHostTunnel,
  createTcpTunnel,
  deleteDomainRoute,
  deleteTunnel,
  getDashboardSummary,
  getDomainRoutes,
  getTunnels,
  getUsageConnections,
  getUsageTraffic,
  rechargeManual,
  settleBilling,
  setTunnelEnabled,
} from "./api";
import type { DashboardSummary, DomainRoute, TrafficUsage, Tunnel, TunnelConnection, User } from "./types";

const storagePrefix = "netunnel-desktop:";

function loadPersistedValue(key: string, fallback: string) {
  if (typeof window === "undefined") {
    return fallback;
  }
  return window.localStorage.getItem(`${storagePrefix}${key}`) ?? fallback;
}

const baseUrl = ref(loadPersistedValue("baseUrl", "http://127.0.0.1:40461"));
const userId = ref(loadPersistedValue("userId", "79fe6216-98d3-41d3-b655-37591cbdb5f1"));
const accessToken = ref(loadPersistedValue("accessToken", ""));
const rechargeAmount = ref(loadPersistedValue("rechargeAmount", "1.0000"));
const rechargeRemark = ref(loadPersistedValue("rechargeRemark", "desktop manual recharge"));
const loading = ref(false);
const actionMessage = ref("");
const actionError = ref("");
const summary = ref<DashboardSummary | null>(null);
const tunnels = ref<Tunnel[]>([]);
const domainRoutes = ref<Record<string, DomainRoute[]>>({});
const usageConnections = ref<TunnelConnection[]>([]);
const usageTraffic = ref<TrafficUsage[]>([]);
const lastSettlement = ref<{
  chargedBytes: number;
  chargeAmount: string;
  transactionId?: string;
} | null>(null);
const sessionMode = ref(loadPersistedValue("sessionMode", "development"));
const currentUser = ref<User | null>(null);
const bootstrapForm = ref({
  email: loadPersistedValue("bootstrapForm:email", ""),
  nickname: loadPersistedValue("bootstrapForm:nickname", "desktop-dev-user"),
  password: loadPersistedValue("bootstrapForm:password", "dev123456"),
});
const usageFilter = ref({
  tunnelId: loadPersistedValue("usageFilter:tunnelId", ""),
  limit: loadPersistedValue("usageFilter:limit", "10"),
  hours: loadPersistedValue("usageFilter:hours", "24"),
});
const tcpForm = ref({
  agentId: loadPersistedValue("tcpForm:agentId", ""),
  name: loadPersistedValue("tcpForm:name", "tcp-new"),
  localHost: loadPersistedValue("tcpForm:localHost", "127.0.0.1"),
  localPort: loadPersistedValue("tcpForm:localPort", "5432"),
  remotePort: loadPersistedValue("tcpForm:remotePort", "41050"),
});
const hostForm = ref({
  agentId: loadPersistedValue("hostForm:agentId", ""),
  name: loadPersistedValue("hostForm:name", "web-new"),
  localHost: loadPersistedValue("hostForm:localHost", "127.0.0.1"),
  localPort: loadPersistedValue("hostForm:localPort", "3000"),
  domain: loadPersistedValue("hostForm:domain", "demo2"),
});

const cards = computed(() => {
  if (!summary.value) {
    return [];
  }
  return [
    { label: "账户余额", value: `${summary.value.account.balance} ${summary.value.account.currency}` },
    { label: "在线 Agent", value: `${summary.value.online_agents}/${summary.value.total_agents}` },
    { label: "启用 Tunnel", value: `${summary.value.enabled_tunnels}/${summary.value.total_tunnels}` },
    { label: "24h 流量", value: `${summary.value.recent_traffic_bytes_24h} bytes` },
  ];
});

const sessionSummary = computed(() => ({
  mode: sessionMode.value,
  userId: userId.value,
  hasToken: accessToken.value.trim() !== "",
  nickname: currentUser.value?.nickname ?? "未绑定",
}));

function resetStatus() {
  actionMessage.value = "";
  actionError.value = "";
}

function toErrorMessage(error: unknown) {
  if (error instanceof ApiError) {
    return `${error.message} (HTTP ${error.status})`;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "未知错误";
}

async function loadAll() {
  loading.value = true;
  resetStatus();

  try {
    const [summaryResponse, tunnelsResponse] = await Promise.all([
      getDashboardSummary(baseUrl.value, userId.value),
      getTunnels(baseUrl.value, userId.value),
    ]);
    summary.value = summaryResponse.summary;
    tunnels.value = tunnelsResponse.tunnels;
    await Promise.all([reloadDomainRoutes(), reloadUsage()]);
    syncDefaultAgentId();
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function reloadSummary() {
  loading.value = true;
  resetStatus();

  try {
    const response = await getDashboardSummary(baseUrl.value, userId.value);
    summary.value = response.summary;
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function reloadTunnels() {
  loading.value = true;
  resetStatus();

  try {
    const response = await getTunnels(baseUrl.value, userId.value);
    tunnels.value = response.tunnels;
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function reloadDomainRoutes() {
  const httpHostTunnels = tunnels.value.filter((item) => item.type === "http_host");
  const results = await Promise.all(
    httpHostTunnels.map(async (item) => {
      const response = await getDomainRoutes(baseUrl.value, item.id);
      return [item.id, response.routes] as const;
    }),
  );
  domainRoutes.value = Object.fromEntries(results);
}

async function reloadUsage() {
  const [connectionsResponse, trafficResponse] = await Promise.all([
    getUsageConnections(
      baseUrl.value,
      userId.value,
      usageFilter.value.tunnelId,
      Number(usageFilter.value.limit) || 10,
    ),
    getUsageTraffic(
      baseUrl.value,
      userId.value,
      usageFilter.value.tunnelId,
      Number(usageFilter.value.hours) || 24,
    ),
  ]);
  usageConnections.value = connectionsResponse.connections;
  usageTraffic.value = trafficResponse.usages;
}

async function submitRecharge() {
  loading.value = true;
  resetStatus();

  try {
    await rechargeManual(baseUrl.value, {
      user_id: userId.value,
      amount: rechargeAmount.value,
      remark: rechargeRemark.value,
    });
    actionMessage.value = "充值成功，数据已刷新。";
    const [summaryResponse, tunnelsResponse] = await Promise.all([
      getDashboardSummary(baseUrl.value, userId.value),
      getTunnels(baseUrl.value, userId.value),
    ]);
    summary.value = summaryResponse.summary;
    tunnels.value = tunnelsResponse.tunnels;
    await Promise.all([reloadDomainRoutes(), reloadUsage()]);
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function bootstrapDevelopmentUser() {
  loading.value = true;
  resetStatus();

  try {
    const result = await bootstrapUser(baseUrl.value, {
      email: bootstrapForm.value.email,
      nickname: bootstrapForm.value.nickname,
      password: bootstrapForm.value.password,
    });
    currentUser.value = result.user;
    userId.value = result.user.id;
    actionMessage.value = `开发用户已创建并切换，nickname=${result.user.nickname}`;
    await loadAll();
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function runSettlement() {
  loading.value = true;
  resetStatus();

  try {
    const result = await settleBilling(baseUrl.value, { user_id: userId.value });
    lastSettlement.value = {
      chargedBytes: result.charged_bytes,
      chargeAmount: result.charge_amount,
      transactionId: result.transaction?.id,
    };
    actionMessage.value = `结算完成，charged_bytes=${result.charged_bytes}，charge_amount=${result.charge_amount}。`;
    await Promise.all([reloadSummary(), reloadUsage()]);
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function toggleTunnel(tunnel: Tunnel) {
  loading.value = true;
  resetStatus();

  try {
    await setTunnelEnabled(baseUrl.value, userId.value, tunnel.id, !tunnel.enabled);
    actionMessage.value = `${tunnel.name} 已${tunnel.enabled ? "停用" : "启用"}。`;
    const [summaryResponse, tunnelsResponse] = await Promise.all([
      getDashboardSummary(baseUrl.value, userId.value),
      getTunnels(baseUrl.value, userId.value),
    ]);
    summary.value = summaryResponse.summary;
    tunnels.value = tunnelsResponse.tunnels;
    await Promise.all([reloadDomainRoutes(), reloadUsage()]);
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function createTcp() {
  loading.value = true;
  resetStatus();

  try {
    await createTcpTunnel(baseUrl.value, {
      user_id: userId.value,
      agent_id: tcpForm.value.agentId,
      name: tcpForm.value.name,
      local_host: tcpForm.value.localHost,
      local_port: Number(tcpForm.value.localPort),
      remote_port: Number(tcpForm.value.remotePort),
    });
    actionMessage.value = "TCP tunnel 创建成功。";
    await Promise.all([reloadSummary(), reloadTunnels(), reloadUsage()]);
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function createHttpHost() {
  loading.value = true;
  resetStatus();

  try {
    await createHttpHostTunnel(baseUrl.value, {
      user_id: userId.value,
      agent_id: hostForm.value.agentId,
      name: hostForm.value.name,
      local_host: hostForm.value.localHost,
      local_port: Number(hostForm.value.localPort),
      domain_prefix: hostForm.value.domain,
    });
    actionMessage.value = "Host tunnel 创建成功。";
    await Promise.all([reloadSummary(), reloadTunnels(), reloadDomainRoutes(), reloadUsage()]);
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function removeTunnel(tunnel: Tunnel) {
  loading.value = true;
  resetStatus();

  try {
    await deleteTunnel(baseUrl.value, userId.value, tunnel.id);
    actionMessage.value = `${tunnel.name} 已删除。`;
    await Promise.all([reloadSummary(), reloadTunnels(), reloadDomainRoutes(), reloadUsage()]);
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

async function removeDomainRoute(route: DomainRoute) {
  loading.value = true;
  resetStatus();

  try {
    await deleteDomainRoute(baseUrl.value, userId.value, route.id);
    actionMessage.value = `${route.scheme}://${route.domain} 已删除。`;
    await Promise.all([reloadDomainRoutes(), reloadUsage()]);
  } catch (error) {
    actionError.value = toErrorMessage(error);
  } finally {
    loading.value = false;
  }
}

function syncDefaultAgentId() {
  const firstAgentId = tunnels.value[0]?.agent_id ?? "";
  if (firstAgentId !== "") {
    if (!tcpForm.value.agentId) {
      tcpForm.value.agentId = firstAgentId;
    }
    if (!hostForm.value.agentId) {
      hostForm.value.agentId = firstAgentId;
    }
  }
}

if (typeof window !== "undefined") {
  watch(baseUrl, (value) => window.localStorage.setItem(`${storagePrefix}baseUrl`, value), { immediate: true });
  watch(userId, (value) => window.localStorage.setItem(`${storagePrefix}userId`, value), { immediate: true });
  watch(accessToken, (value) => window.localStorage.setItem(`${storagePrefix}accessToken`, value), { immediate: true });
  watch(sessionMode, (value) => window.localStorage.setItem(`${storagePrefix}sessionMode`, value), { immediate: true });
  watch(rechargeAmount, (value) => window.localStorage.setItem(`${storagePrefix}rechargeAmount`, value), { immediate: true });
  watch(rechargeRemark, (value) => window.localStorage.setItem(`${storagePrefix}rechargeRemark`, value), { immediate: true });
  watch(
    bootstrapForm,
    (value) => {
      window.localStorage.setItem(`${storagePrefix}bootstrapForm:email`, value.email);
      window.localStorage.setItem(`${storagePrefix}bootstrapForm:nickname`, value.nickname);
      window.localStorage.setItem(`${storagePrefix}bootstrapForm:password`, value.password);
    },
    { deep: true, immediate: true },
  );
  watch(
    tcpForm,
    (value) => {
      window.localStorage.setItem(`${storagePrefix}tcpForm:agentId`, value.agentId);
      window.localStorage.setItem(`${storagePrefix}tcpForm:name`, value.name);
      window.localStorage.setItem(`${storagePrefix}tcpForm:localHost`, value.localHost);
      window.localStorage.setItem(`${storagePrefix}tcpForm:localPort`, value.localPort);
      window.localStorage.setItem(`${storagePrefix}tcpForm:remotePort`, value.remotePort);
    },
    { deep: true, immediate: true },
  );
  watch(
    hostForm,
    (value) => {
      window.localStorage.setItem(`${storagePrefix}hostForm:agentId`, value.agentId);
      window.localStorage.setItem(`${storagePrefix}hostForm:name`, value.name);
      window.localStorage.setItem(`${storagePrefix}hostForm:localHost`, value.localHost);
      window.localStorage.setItem(`${storagePrefix}hostForm:localPort`, value.localPort);
      window.localStorage.setItem(`${storagePrefix}hostForm:domain`, value.domain);
    },
    { deep: true, immediate: true },
  );
  watch(
    usageFilter,
    (value) => {
      window.localStorage.setItem(`${storagePrefix}usageFilter:tunnelId`, value.tunnelId);
      window.localStorage.setItem(`${storagePrefix}usageFilter:limit`, value.limit);
      window.localStorage.setItem(`${storagePrefix}usageFilter:hours`, value.hours);
    },
    { deep: true, immediate: true },
  );
}

const workspaceState = {
  baseUrl,
  userId,
  accessToken,
  sessionMode,
  currentUser,
  bootstrapForm,
  rechargeAmount,
  rechargeRemark,
  loading,
  actionMessage,
  actionError,
  summary,
  tunnels,
  domainRoutes,
  usageConnections,
  usageTraffic,
  lastSettlement,
  sessionSummary,
  usageFilter,
  tcpForm,
  hostForm,
  cards,
  loadAll,
  reloadSummary,
  reloadTunnels,
  reloadDomainRoutes,
  reloadUsage,
  submitRecharge,
  bootstrapDevelopmentUser,
  runSettlement,
  toggleTunnel,
  createTcp,
  createHttpHost,
  removeTunnel,
  removeDomainRoute,
  syncDefaultAgentId,
};

export function useWorkspaceState() {
  return workspaceState;
}
