# Netunnel Desktop AI Integration Doc

## Task goal

Implement the first desktop-side integration for `netunnel-desktop` against the current `netunnel-server`.

Current app name: `netunnel-desktop`

Current backend base URL: `http://127.0.0.1:40461`

Current backend HTTPS public entry: `https://127.0.0.1:40463` for host-based forwarding tests only, not for management API.

Current environment: local development on Windows.

Desktop first-screen goal:

1. Load dashboard summary for one user.
2. Show account balance, agent count, tunnel count, recent traffic, recent billing transactions.
3. Allow manual recharge.
4. Allow listing tunnels.
5. Allow enabling or disabling a tunnel.

Use query-string `user_id` for read APIs and JSON body `user_id` for write APIs exactly as shown below.

Current test user ID:

`79fe6216-98d3-41d3-b655-37591cbdb5f1`

## Interface 1 request

Dashboard summary.

HTTP method:

`GET`

Full URL:

`http://127.0.0.1:40461/api/v1/dashboard/summary?user_id=79fe6216-98d3-41d3-b655-37591cbdb5f1`

Request body:

None.

## Interface 1 complete response body

```json
{
  "summary": {
    "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
    "account": {
      "id": "44274b81-ebec-4f42-b4a4-673bf664b49c",
      "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
      "balance": "11811160064",
      "currency": "CNY",
      "status": "active",
      "created_at": "2026-03-21T10:33:22.994717+08:00",
      "updated_at": "2026-03-21T10:49:00.133137+08:00"
    },
    "total_agents": 1,
    "online_agents": 1,
    "total_tunnels": 3,
    "enabled_tunnels": 3,
    "disabled_billing_tunnels": 0,
    "recent_traffic_bytes_24h": 363,
    "unbilled_traffic_bytes_24h": 0,
    "recent_transactions": [
      {
        "id": "774142f1-ee84-4f28-8bb2-b1e955c707e0",
        "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
        "account_id": "44274b81-ebec-4f42-b4a4-673bf664b49c",
        "type": "recharge",
        "amount": "1073741824",
        "balance_before": "10737418240",
        "balance_after": "11811160064",
        "reference_type": "manual_recharge",
        "remark": "restore billing disabled tunnels",
        "created_at": "2026-03-21T10:49:00.133137+08:00"
      },
      {
        "id": "8d2873aa-8f41-4204-811a-450cddf94b60",
        "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
        "account_id": "44274b81-ebec-4f42-b4a4-673bf664b49c",
        "type": "recharge",
        "amount": "10737418240",
        "balance_before": "0",
        "balance_after": "10737418240",
        "reference_type": "manual_recharge",
        "remark": "dev manual recharge",
        "created_at": "2026-03-21T10:33:36.89332+08:00"
      }
    ],
    "recent_usages": [
      {
        "id": "469fd8d4-9c0d-417a-9705-d3824891004a",
        "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
        "agent_id": "6bef76c0-2f6e-4a5f-852a-d59680e3ff96",
        "tunnel_id": "648d2ef9-32cf-4d52-9a9d-0e326903539f",
        "bucket_time": "2026-03-21T10:00:00+08:00",
        "ingress_bytes": 0,
        "egress_bytes": 354,
        "total_bytes": 354,
        "billed_bytes": 354,
        "created_at": "2026-03-21T10:17:39.088732+08:00",
        "updated_at": "2026-03-21T10:49:50.387612+08:00"
      },
      {
        "id": "bf5bca7b-9566-4128-8ced-5df9d1cfd5cd",
        "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
        "agent_id": "6bef76c0-2f6e-4a5f-852a-d59680e3ff96",
        "tunnel_id": "114550b6-c273-4673-b690-ac1ac4cdef0a",
        "bucket_time": "2026-03-21T10:00:00+08:00",
        "ingress_bytes": 8,
        "egress_bytes": 1,
        "total_bytes": 9,
        "billed_bytes": 9,
        "created_at": "2026-03-21T10:17:38.93219+08:00",
        "updated_at": "2026-03-21T10:34:46.321147+08:00"
      }
    ]
  }
}
```

## Interface 2 request

Manual recharge.

HTTP method:

`POST`

Full URL:

`http://127.0.0.1:40461/api/v1/billing/recharge/manual`

Request body:

```json
{
  "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
  "amount": "1.0000",
  "remark": "desktop manual recharge"
}
```

## Interface 2 complete response body

```json
{
  "account": {
    "id": "44274b81-ebec-4f42-b4a4-673bf664b49c",
    "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
    "balance": "12884901888",
    "currency": "CNY",
    "status": "active",
    "created_at": "2026-03-21T10:33:22.994717+08:00",
    "updated_at": "2026-03-21T10:52:00.000000+08:00"
  },
  "transaction": {
    "id": "new-transaction-id",
    "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
    "account_id": "44274b81-ebec-4f42-b4a4-673bf664b49c",
    "type": "recharge",
    "amount": "1073741824",
    "balance_before": "11811160064",
    "balance_after": "12884901888",
    "reference_type": "manual_recharge",
    "remark": "desktop manual recharge",
    "created_at": "2026-03-21T10:52:00.000000+08:00"
  }
}
```

## Interface 3 request

Tunnel list.

HTTP method:

`GET`

Full URL:

`http://127.0.0.1:40461/api/v1/tunnels?user_id=79fe6216-98d3-41d3-b655-37591cbdb5f1`

Request body:

None.

## Interface 3 complete response body

```json
{
  "tunnels": [
    {
      "id": "648d2ef9-32cf-4d52-9a9d-0e326903539f",
      "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
      "agent_id": "6bef76c0-2f6e-4a5f-852a-d59680e3ff96",
      "name": "web-secure",
      "type": "http_host",
      "status": "active",
      "enabled": true,
      "local_host": "127.0.0.1",
      "local_port": 3000,
      "created_at": "2026-03-21T10:09:46.46261+08:00",
      "updated_at": "2026-03-21T10:49:00.158772+08:00"
    },
    {
      "id": "2d640de3-7222-4ad6-8b04-37632712fcc5",
      "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
      "agent_id": "6bef76c0-2f6e-4a5f-852a-d59680e3ff96",
      "name": "web-dev",
      "type": "http_host",
      "status": "active",
      "enabled": true,
      "local_host": "127.0.0.1",
      "local_port": 3000,
      "created_at": "2026-03-21T00:49:47.878711+08:00",
      "updated_at": "2026-03-21T10:49:00.158772+08:00"
    },
    {
      "id": "114550b6-c273-4673-b690-ac1ac4cdef0a",
      "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
      "agent_id": "6bef76c0-2f6e-4a5f-852a-d59680e3ff96",
      "name": "pg-dev",
      "type": "tcp",
      "status": "active",
      "enabled": true,
      "local_host": "127.0.0.1",
      "local_port": 5432,
      "remote_port": 41032,
      "created_at": "2026-03-21T00:29:12.909064+08:00",
      "updated_at": "2026-03-21T10:49:00.158772+08:00"
    }
  ]
}
```

## Interface 4 request

Tunnel enable or disable.

Enable:

`POST http://127.0.0.1:40461/api/v1/tunnels/114550b6-c273-4673-b690-ac1ac4cdef0a/enable?user_id=79fe6216-98d3-41d3-b655-37591cbdb5f1`

Disable:

`POST http://127.0.0.1:40461/api/v1/tunnels/114550b6-c273-4673-b690-ac1ac4cdef0a/disable?user_id=79fe6216-98d3-41d3-b655-37591cbdb5f1`

Request body:

None.

## Interface 4 complete response body

```json
{
  "tunnel": {
    "id": "114550b6-c273-4673-b690-ac1ac4cdef0a",
    "user_id": "79fe6216-98d3-41d3-b655-37591cbdb5f1",
    "agent_id": "6bef76c0-2f6e-4a5f-852a-d59680e3ff96",
    "name": "pg-dev",
    "type": "tcp",
    "status": "active",
    "enabled": true,
    "local_host": "127.0.0.1",
    "local_port": 5432,
    "remote_port": 41032,
    "created_at": "2026-03-21T00:29:12.909064+08:00",
    "updated_at": "2026-03-21T10:49:00.158772+08:00"
  }
}
```

## Status field location

- Dashboard load success is the HTTP status code `200`, and the real business payload is in `summary`.
- Manual recharge success is the HTTP status code `201`, and the real business payload is in `account` and `transaction`.
- Tunnel list success is the HTTP status code `200`, and the real business payload is in `tunnels`.
- Tunnel switch success is the HTTP status code `200`, and the real business payload is in `tunnel`.
- Insufficient balance for public traffic forwarding is returned as HTTP `402` with `{"error":"insufficient balance"}`.

## Success field extraction

- Account balance card: read `summary.account.balance`.
  This field is now a byte-unit integer string, not a GB decimal string.
- Currency label: read `summary.account.currency`.
- Total agent count: read `summary.total_agents`.
- Online agent count: read `summary.online_agents`.
- Total tunnel count: read `summary.total_tunnels`.
- Enabled tunnel count: read `summary.enabled_tunnels`.
- Billing-disabled tunnel count: read `summary.disabled_billing_tunnels`.
- Last 24h traffic total: read `summary.recent_traffic_bytes_24h`.
- Last 24h unbilled traffic: read `summary.unbilled_traffic_bytes_24h`.
- Recent billing table: read `summary.recent_transactions`.
- Recent traffic chart/table: read `summary.recent_usages`.
- Recharge latest balance: read `account.balance`.
  This field is now a byte-unit integer string, not a GB decimal string.
- Tunnel switch result: read `tunnel.enabled` and `tunnel.status`.

## Recommended implementation flow

1. On desktop app startup, store one backend base URL such as `http://127.0.0.1:40461`.
2. After the user is selected or logged in, request `/api/v1/dashboard/summary`.
3. Render four top cards first:
   - balance
   - total tunnels
   - online agents
   - recent traffic 24h
   Balance-related fields are returned in bytes and should be formatted on the desktop side for display.
4. Render recent billing transactions and recent usage list from the same summary response.
5. Load `/api/v1/tunnels` for the tunnel management page.
6. On tunnel enable/disable action, call the matching `/enable` or `/disable` endpoint, then refresh `/api/v1/tunnels`.
7. On manual recharge submit, call `/api/v1/billing/recharge/manual`, then refresh `/api/v1/dashboard/summary` and `/api/v1/tunnels`.
8. If `disabled_billing_tunnels > 0`, show a warning banner and expose a recharge action.
9. If any API returns HTTP `400`, show the `error` field as a form or page error.
10. If the public forwarding side shows HTTP `402`, treat it as account-balance exhaustion, not as a generic network error.

## Current business context

- Product name: `netunnel`
- Desktop shell: Tauri + Vue template planned
- Current supported tunnel types:
  - `tcp`
  - `http_host`
- Current billing model:
  - manual recharge
  - traffic usage aggregation
  - manual settle API
  - automatic periodic settlement
  - insufficient-balance auto disable
  - recharge auto restore for `disabled_billing` tunnels
  - account balance and business record balance fields now use bytes as the stored unit
- Current API auth model:
  - management APIs are still development-stage and use explicit `user_id`
  - no final desktop login/token model has been added yet

## Explicit execution instruction

Implement the desktop integration directly against these APIs.

Create:

1. one API client module for dashboard, billing, and tunnels
2. one home page using `/api/v1/dashboard/summary`
3. one tunnel list page using `/api/v1/tunnels`
4. one manual recharge dialog using `/api/v1/billing/recharge/manual`

Please start coding directly, do not only output a plan.
