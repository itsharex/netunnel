# Netunnel Desktop Tauri

## 特性

- 基于 `tauri-vue-template` 复制出的桌面端工程
- 保留模板原有登录页、设置页、更新与日志能力
- 业务区已改为 `netunnel` 控制台
- 当前已接通 `dashboard / tunnels / usage / billing / session` 能力
- 登录页已接入开发期会话模型：
  - 统一管理 `API Base URL / userId / accessToken / 登录态`
  - 优先尝试正式 `/api/v1/auth/login`
  - 正式接口未接通时自动回退到开发用户创建
  - 会话信息支持本地记住
- 业务请求已统一走 API client：
  - 自动拼接服务端地址
  - 自动附带 `Authorization` 头
  - 后续切正式 token 鉴权时无需逐页改请求
- 业务接口已按模块拆分到独立 service：
  - `dashboard`
  - `tunnels`
  - `billing`
  - `usage`
  - `dev`
- 已补 `Tauri` 原生能力：
  - 托盘菜单
  - 隐藏到托盘 / 显示主窗口
  - 关闭按钮按设置切换为“关闭到托盘”
  - 本地 agent 启动、停止、状态检测
  - 本地 agent 可执行路径配置
  - 本地 agent 启动参数配置
  - 设置页默认 API / Bridge / Agent 路径 / 同步间隔
  - 会话页与设置页默认值双向联动
  - 打开 agent 所在目录

## 准备环境

- 导出 `github.com` 证书并导入“受信任的发布者”是必须先完成的环境准备，否则依赖下载、NSIS 下载或应用内更新访问 GitHub 时可能失败，具体操作见 [常见问题](./常见问题.md)

## 快速开始

1. 安装依赖：

```sh
pnpm install
```

2. 启动开发：

```sh
pnpm tauri dev
```

仅做前端构建验证：

```sh
pnpm build
```

仅做 Rust 侧检查：

```sh
pnpm check
```

## 命令

- `pnpm tauri dev` - 启动开发
- `pnpm tauri build` - 构建生产版本
- `pnpm test` - 运行测试

## 当前目录角色

- [src/App.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop-tauri/src/App.vue)
  - 保留模板壳
- [src/components/LoginView.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop-tauri/src/components/LoginView.vue)
  - 保留模板登录页风格，文案已改为 `netunnel`
- [src/components/SettingsPanel.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop-tauri/src/components/SettingsPanel.vue)
  - 保留模板设置页风格，文案已改为 `netunnel`
- [src/components/DashboardView.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop-tauri/src/components/DashboardView.vue)
  - 模板业务区已替换为 `netunnel` 控制台入口
- [src/components/NetunnelWorkspace.vue](/D:/git-projects/ai-company/projects/netunnel/src/netunnel-desktop-tauri/src/components/NetunnelWorkspace.vue)
  - `netunnel` 业务工作区

## 应用内更新

模板已接入 Tauri v2 updater，设置页里可以直接检查更新并安装。

本地开发或普通构建不会默认生成 updater 产物；只有 GitHub Release 工作流会额外加载 `src-tauri/tauri.updater.conf.json` 来生成 `latest.json` 和签名文件。

发布前需要准备这些 GitHub 配置：

- `secrets.TAURI_SIGNING_PRIVATE_KEY`
- `secrets.TAURI_SIGNING_PRIVATE_KEY_PASSWORD`
- `vars.TAURI_UPDATER_PUBLIC_KEY`

可以先用 Tauri CLI 生成签名密钥：

```sh
pnpm tauri signer generate
```

发布工作流会自动把更新地址设置成当前仓库的 GitHub Releases 最新下载地址：

```text
https://github.com/<owner>/<repo>/releases/latest/download/latest.json
```

## 项目结构

- `src/` - 前端代码 (Vue)
- `src-tauri/` - 后端代码 (Rust)
