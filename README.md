# Netunnel

内网穿透项目，包含桌面端、服务端和 agent。当前数据面已经从“每 tunnel 一个待命 bridge”迁移到“agent 级 data session + substream”模式。

## 项目结构

```text
netunnel/
├── package.json
├── pnpm-workspace.yaml
├── src/
│   ├── netunnel-desktop-tauri/    # Tauri 桌面端（Vue 3 + TypeScript）
│   ├── netunnel-server/           # Go 服务端
│   └── netunnel-agent/            # Go Agent
├── docs/
└── designs/
```

## 常用命令

### 根目录

- `pnpm install`
- `pnpm --filter netunnel-desktop-tauri dev`
- `pnpm --filter netunnel-desktop-tauri build`

### 服务端

- `cd src/netunnel-server && go build ./...`
- `cd src/netunnel-server && go test ./...`

### Agent

- `cd src/netunnel-agent && go build ./...`
- `cd src/netunnel-agent && go test ./...`

## 服务端配置

服务端配置文件位于：

- `src/netunnel-server/config.yaml`
- `src/netunnel-server/config.production.yaml`

当前常用配置项：

- `listen_addr`: HTTP API 监听地址，默认 `:40061`
- `bridge_listen_addr`: agent 连接入口，默认 `:40062`
- `tcp_port_ranges`: TCP tunnel 可分配端口范围
- `public_host`: TCP 对外地址
- `public_api_base_url`: HTTP API 对外地址
- `host_domain_suffix`: 域名隧道后缀

## 数据面迁移状态

当前数据面状态：

- TCP：走 `data session + substream`
- `http_host`：走 `data session + substream`

## 运行观察

重点日志：

- 服务端 TCP：`tcp runtime summary`
- 服务端 data session：
  - `data session summary`
  - `data session counters`
  - `data session per-agent streams`
  - `data session per-tunnel streams`
- 服务端 HTTP：`public http summary`
- Agent：`agent data session summary`

建议重点看这些字段：

- TCP：`data_session_successes`、`data_session_failures`、`data_session_acquire_failures`
- HTTP：`data_session_successes`、`data_session_failures`、`data_stream_failures`
- Agent：`retries`、`active_streams`、`open_failures`、`write_fail_closes`

## 相关文档

- `AGENTS.md`
- `src/netunnel-server/README.md`
- `src/netunnel-agent/README.md`
- `src/netunnel-desktop-tauri/README.md`
