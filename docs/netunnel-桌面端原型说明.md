# netunnel 桌面端原型说明

## 当前状态

当前已在 [src/netunnel-desktop](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop) 下搭好一套 `Vue 3 + TypeScript + Vite` 的桌面端原型。

这套原型的目的不是替代最终的 `Tauri + Vue` 桌面壳，而是先把后端接口、页面结构和交互链路跑通，后续再平移进你指定的 `tauri-vue-template`。

当前已经新增一套基于模板副本的正式桌面端目录：[src/netunnel-desktop-tauri](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop-tauri)。

- [src/netunnel-desktop](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop)：联调原型，便于快速实验接口
- [src/netunnel-desktop-tauri](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop-tauri)：基于 `tauri-vue-template` 复制出来的正式桌面端起点

## 已接通能力

- 概览首页：读取 `/api/v1/dashboard/summary`
- Tunnel 列表：读取 `/api/v1/tunnels`
- Tunnel 启用/停用：调用 `/api/v1/tunnels/{id}/enable`、`/disable`
- Tunnel 创建/删除：调用 `/api/v1/tunnels/tcp`、`/api/v1/tunnels/http-host`、`DELETE /api/v1/tunnels/{id}`
- 域名展示与删除：调用 `/api/v1/domain-routes`、`DELETE /api/v1/domain-routes/{id}`
- 手工充值：调用 `/api/v1/billing/recharge/manual`
- 手动结算：调用 `/api/v1/billing/settle`
- 证书管理：调用 `/api/v1/certificates`
- 连接记录与流量明细：调用 `/api/v1/usage/connections`、`/api/v1/usage/traffic`
- 本地配置记忆：当前会把 `API Base URL`、`User ID` 和主要表单字段写入浏览器 `localStorage`

## 本地运行

工作目录：

`D:\git-projects\ai-company\projects\netunnel\src\netunnel-desktop`

安装依赖：

```bash
pnpm install
```

开发模式：

```bash
pnpm dev
```

生产构建：

```bash
pnpm build
```

我已在 2026-03-21 实际执行过一次 `pnpm build`，构建通过。

## 当前默认联调参数

- 后端管理 API：`http://127.0.0.1:40461`
- 测试用户 ID：`79fe6216-98d3-41d3-b655-37591cbdb5f1`

这些值目前直接写在 [App.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop/src/App.vue) 里，后续迁到 Tauri 时可以改成本地配置或登录态注入。

## 关键文件

- 页面入口：[App.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop/src/App.vue)
- API 封装：[api.ts](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop/src/api.ts)
- 共享状态：[state.ts](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop/src/state.ts)
- 类型定义：[types.ts](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop/src/types.ts)
- 样式：[style.css](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop/src/style.css)

## 后续迁移建议

1. 先保留这套页面与 API client，不急着重写。
2. 等 `tauri-vue-template` 真正初始化后，把 `src/` 下页面和请求层平移过去。
3. 再补桌面端专属能力：
   - 登录态与 token
   - 本地持久化配置
   - Tauri 原生窗口与托盘
   - agent 安装、启动、日志查看
