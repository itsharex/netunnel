import type {
  Account,
  BillingSettlementResult,
  DashboardSummary,
  DomainRoute,
  TrafficUsage,
  Tunnel,
  TunnelConnection,
  User,
} from "./types";

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

function authHeaders(accessToken?: string) {
  if (!accessToken) {
    return {};
  }
  return {
    Authorization: `Bearer ${accessToken}`,
  };
}

async function request<T>(baseUrl: string, path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new ApiError(payload.error ?? `Request failed: ${response.status}`, response.status);
  }
  return payload as T;
}

export function getDashboardSummary(baseUrl: string, userId: string) {
  return request<{ summary: DashboardSummary }>(
    baseUrl,
    `/api/v1/dashboard/summary?user_id=${encodeURIComponent(userId)}`,
  );
}

export function bootstrapUser(
  baseUrl: string,
  payload: {
    email: string;
    nickname: string;
    password: string;
  },
) {
  return request<{ user: User }>(baseUrl, "/api/v1/dev/bootstrap-user", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function getTunnels(baseUrl: string, userId: string) {
  return request<{ tunnels: Tunnel[] }>(
    baseUrl,
    `/api/v1/tunnels?user_id=${encodeURIComponent(userId)}`,
  );
}

export function setTunnelEnabled(baseUrl: string, userId: string, tunnelId: string, enabled: boolean) {
  const action = enabled ? "enable" : "disable";
  return request<{ tunnel: Tunnel }>(
    baseUrl,
    `/api/v1/tunnels/${tunnelId}/${action}?user_id=${encodeURIComponent(userId)}`,
    { method: "POST" },
  );
}

export function deleteTunnel(baseUrl: string, userId: string, tunnelId: string) {
  return request<{ deleted: boolean; tunnel_id: string }>(
    baseUrl,
    `/api/v1/tunnels/${tunnelId}?user_id=${encodeURIComponent(userId)}`,
    { method: "DELETE" },
  );
}

export function createTcpTunnel(
  baseUrl: string,
  payload: {
    user_id: string;
    agent_id: string;
    name: string;
    local_host: string;
    local_port: number;
    remote_port: number;
  },
) {
  return request<{ tunnel: Tunnel }>(baseUrl, "/api/v1/tunnels/tcp", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function createHttpHostTunnel(
  baseUrl: string,
  payload: {
    user_id: string;
    agent_id: string;
    name: string;
    local_host: string;
    local_port: number;
    domain_prefix: string;
  },
) {
  return request<{ tunnel: Tunnel; route: DomainRoute }>(baseUrl, "/api/v1/tunnels/http-host", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function getDomainRoutes(baseUrl: string, tunnelId: string) {
  return request<{ routes: DomainRoute[] }>(
    baseUrl,
    `/api/v1/domain-routes?tunnel_id=${encodeURIComponent(tunnelId)}`,
  );
}

export function rechargeManual(baseUrl: string, payload: { user_id: string; amount: string; remark: string }) {
  return request<{ account: Account }>(baseUrl, "/api/v1/billing/recharge/manual", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function getUsageConnections(baseUrl: string, userId: string, tunnelId: string, limit: number) {
  const tunnelQuery = tunnelId ? `&tunnel_id=${encodeURIComponent(tunnelId)}` : "";
  return request<{ connections: TunnelConnection[] }>(
    baseUrl,
    `/api/v1/usage/connections?user_id=${encodeURIComponent(userId)}${tunnelQuery}&limit=${limit}`,
  );
}

export function getUsageTraffic(baseUrl: string, userId: string, tunnelId: string, hours: number) {
  const tunnelQuery = tunnelId ? `&tunnel_id=${encodeURIComponent(tunnelId)}` : "";
  return request<{ usages: TrafficUsage[] }>(
    baseUrl,
    `/api/v1/usage/traffic?user_id=${encodeURIComponent(userId)}${tunnelQuery}&hours=${hours}`,
  );
}

export function settleBilling(baseUrl: string, payload: { user_id: string }) {
  return request<BillingSettlementResult>(baseUrl, "/api/v1/billing/settle", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function deleteDomainRoute(baseUrl: string, userId: string, routeId: string) {
  return request<{ deleted: boolean; route_id: string }>(
    baseUrl,
    `/api/v1/domain-routes/${routeId}?user_id=${encodeURIComponent(userId)}`,
    { method: "DELETE" },
  );
}
