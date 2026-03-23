# netunnel-desktop

一个用于联调 `netunnel-server` 的桌面端前端原型，当前技术栈为 `Vue 3 + TypeScript + Vite`。

## 目的

- 先把管理 API 和页面交互跑通
- 后续再迁移进正式的 `Tauri + Vue` 模板

## 已实现

- Dashboard 概览
- 账户余额展示
- 手工充值
- Tunnel 列表
- Tunnel 启用/停用
- TCP Tunnel 创建
- HTTP/HTTPS Host Tunnel 创建
- Tunnel 删除
- 域名路由删除
- 证书上传与删除
- 连接记录与流量明细
- API 地址和用户 ID 本地记忆
- 手动结算

## 运行

```bash
pnpm install
pnpm dev
```

生产构建：

```bash
pnpm build
```

## 当前默认后端

- API: `http://127.0.0.1:40461`
- User ID: `79fe6216-98d3-41d3-b655-37591cbdb5f1`

默认值定义在 [src/App.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop/src/App.vue)。
