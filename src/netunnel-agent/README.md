# netunnel-agent

客户端新骨架。

当前已完成：

- 配置读取
- Agent 注册
- 定时拉取服务端配置
- 为 TCP tunnel 维持桥接连接
- 打印已同步的 tunnel 列表

默认参数可通过环境变量或命令行覆盖：

- `NETUNNEL_SERVER_URL` / `-server-url`
- `NETUNNEL_BRIDGE_ADDR` / `-bridge-addr`
- `NETUNNEL_USER_ID` / `-user-id`
- `NETUNNEL_AGENT_NAME` / `-agent-name`
- `NETUNNEL_MACHINE_CODE` / `-machine-code`
- `NETUNNEL_CLIENT_VERSION` / `-client-version`
- `NETUNNEL_OS_TYPE` / `-os-type`
- `NETUNNEL_SYNC_INTERVAL` / `-sync-interval`

下一步将继续接入：

- TCP 实际转发
- tunnel 生命周期管理
- 流量统计上报
