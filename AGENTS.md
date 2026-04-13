# Netunnel Agents

## 项目结构

```
netunnel/                          # pnpm workspace 根目录
├── package.json                   # workspace 配置
├── pnpm-workspace.yaml            # workspace 成员声明
├── .github/
│   └── workflows/
│       └── release.yml            # GitHub 自动发布 workflow
├── src/
│   ├── netunnel-desktop-tauri/    # Tauri 桌面端 (Vue 3 + Tailwind + Pinia + TypeScript)
│   ├── netunnel-server/          # Go 后端服务 (pgx/v5 + 标准库)
│   └── netunnel-agent/           # Go Agent 客户端
└── designs/                      # 设计资源
```

---

## 开发命令

### pnpm workspace（根目录）

| 命令 | 说明 |
|------|------|
| `pnpm install` | 安装所有子项目依赖 |
| `pnpm --filter netunnel-desktop-tauri dev` | 启动桌面端开发服务器 |
| `pnpm --filter netunnel-desktop-tauri build` | 构建桌面端生产版本 |
| `pnpm --filter netunnel-desktop-tauri type-check` | TypeScript 类型检查 |
| `pnpm --filter netunnel-desktop-tauri tauri dev` | 启动 Tauri 开发模式 |
| `pnpm --filter netunnel-desktop-tauri tauri build` | 构建 Tauri 安装包 |

### 版本号更新

```bash
cd src/netunnel-desktop-tauri
node bump-version.cjs 2.12.6
```

这会自动更新：
- `package.json` version
- `src-tauri/tauri.conf.json` version
- `src-tauri/Cargo.toml` version
- `src-tauri/Cargo.lock` version

然后手动更新根目录 `package.json` 的 version。

### Go 后端 (`src/netunnel-server/`)

| 命令 | 说明 |
|------|------|
| `go build ./...` | 编译所有包 |
| `go test ./...` | 运行所有测试 |
| `go test ./internal/service -v` | 运行单个包测试 |
| `go vet ./...` | 代码检查 |
| `go fmt ./...` | 格式化代码 |

---

## 代码风格

### Vue / TypeScript (桌面端)

**格式化**: Prettier (`.prettierrc.json`)
- `semi: false` — 不使用分号
- `singleQuote: true` — 使用单引号
- `printWidth: 120` — 行宽 120 字符
- `tabWidth: 2` — 缩进 2 空格

**ESLint**: eslint-plugin-vue + @typescript-eslint

**导入顺序**:
1. Vue / framework imports (`vue`, `vue-router`, `pinia`)
2. `@/` 路径别名 imports (services, store, types, composables)
3. `@tauri-apps/` imports
4. 第三方库
5. 相对路径 imports

**类型**: 始终使用 TypeScript 类型，避免 `any`。使用 `interface` 定义对象类型，`type` 定义联合/别名。

**错误处理**: 服务层函数抛出具体 Error，调用方负责捕获和转换。

**组件结构**:
```
<script setup lang="ts">
  // 导入 → Props/Directives → Composables → State → Lifecycle → Methods
</script>

<template>
  <!-- 语义化 HTML + Tailwind class -->
</template>

<style scoped>
  /* 少量覆盖样式 */
</style>
```

**命名**:
- 组件文件: `PascalCase.vue`
- 组合式函数: `camelCase.ts` (以 `use` 开头，如 `useWindowControls.ts`)
- 类型文件: `camelCase.ts` (放在 `types/` 目录)
- CSS 变量: `kebab-case`

**模板**:
- 使用 `v-if` / `v-show` 而非 CSS display 切换
- 列表使用 `v-for` + `:key`
- 表单事件使用 `@submit.prevent`

### Go (服务端)

**格式化**: `go fmt` + `goimports`

**错误处理**: wrap errors with `fmt.Errorf("%w: ...", err)`

**包结构**:
```
internal/
├── domain/        # 领域模型 (不含框架依赖)
├── repository/    # 数据访问层
├── service/       # 业务逻辑层
├── transport/     # HTTP / TCP 传输层
└── config/        # 配置加载
```

**命名**:
- 包名: `snake_case` (如 `domain`, `tunnel_service`)
- 结构体: `PascalCase`
- 变量/函数: `camelCase`
- 接口: `PascalCase`，习惯以 `er` 结尾 (如 `Tunneler`)

**错误定义**: 在 service 包中定义 sentinel errors，如 `ErrInvalidArgument = errors.New("invalid argument")`

**数据库**: 使用 `pgx/v5`，查询参数用 `$1`, `$2` 占位符。

**上下文**: 入口函数接收 `context.Context`，层层传递，禁止在 domain model 中存储 ctx。

---

## 数据库

- PostgreSQL (Docker 环境变量: `POSTGRES_HOST=10.60.131.181`)
- 迁移文件在 `sql/` 目录，按序号执行 (`001_*.sql`, `002_*.sql`)
- Prisma schema 位于 `netunnel-server/internal/repository/schema.prisma`

---

## 环境变量

桌面端使用 Vite，默认前缀 `VITE_` / `TAURI_`。关键环境变量：
- 开发环境 `.env.development`：
  - `VITE_DEFAULT_HOME_URL=http://127.0.0.1:40061`
  - `VITE_DEFAULT_BRIDGE_ADDR=127.0.0.1:40062`
- 生产环境 `.env.production`：
- `VITE_DEFAULT_HOME_URL` — 桌面端默认服务地址（当前临时使用 `http://101.43.49.100:40061`，待域名备案完成后再切回域名）
- `VITE_DEFAULT_BRIDGE_ADDR` — 桌面端默认 Bridge 地址（生产当前为 `101.43.49.100:40062`）

服务端使用环境变量或 `config.yaml`，关键变量:
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- `SERVER_PORT` (默认 40061)
- `BRIDGE_PORT` (默认 40062)

### 生产环境中 IP 与域名的职责划分

以后替换服务器 IP 时，先区分“连接服务器的地址”与“对外暴露给用户的域名”，不要混改。

#### 应该使用 IP 的位置

- `deploy/deploy.config.mjs` 的 `ssh.host`
  - 作用：部署脚本 SSH 登录服务器
  - 当前值：`101.43.49.100`
- `src/netunnel-desktop-tauri/.env.production` 的 `VITE_DEFAULT_BRIDGE_ADDR`
  - 作用：桌面端连接 Agent Bridge TCP 入口
  - 当前值：`101.43.49.100:40062`
- 服务端生产配置里的 `public_host`
  - 文件位置：线上实际生效文件是 `/www/wwwroot/netunnel/shared/config.yaml`
  - 仓库发布源文件：`src/netunnel-server/config.production.yaml`
  - 作用：TCP 隧道访问地址，例如 `101.43.49.100:50001`
  - 当前值：`101.43.49.100`

#### 应该使用域名的位置

当前临时策略：因为新服务器域名备案还没完成，API 先走 `http://101.43.49.100:40061`，域名内网穿透暂时不使用。下面这些域名位点保留到重新启用域名穿透时再切回。

- `src/netunnel-desktop-tauri/.env.production` 的 `VITE_DEFAULT_HOME_URL`
  - 作用：桌面端默认 API / Web 入口
  - 备案未完成期间当前值：`http://101.43.49.100:40061`
  - 域名恢复后应切回：`https://nps1.tx07.cn`
- 服务端生产配置里的 `public_api_base_url`
  - 文件位置：线上实际生效文件是 `/www/wwwroot/netunnel/shared/config.yaml`
  - 仓库发布源文件：`src/netunnel-server/config.production.yaml`
  - 作用：服务端对外生成支付回调、平台访问基址等 HTTP API 地址
  - 备案未完成期间当前值：`http://101.43.49.100:40061`
  - 域名恢复后应切回：`https://nps1.tx07.cn`
- 服务端生产配置里的 `host_domain_suffix`
  - 文件位置：线上实际生效文件是 `/www/wwwroot/netunnel/shared/config.yaml`
  - 仓库发布源文件：`src/netunnel-server/config.production.yaml`
  - 作用：生成域名内网穿透地址后缀，后端按 `前缀 + "." + host_domain_suffix` 拼接
  - 当前值：`nps1.tx07.cn`
- Nginx 站点模板 `deploy/nginx/nps1.tx07.cn.conf`
  - 作用：公网域名 `nps1.tx07.cn` 与 `*.nps1.tx07.cn` 转发到后端 `127.0.0.1:40061`

#### 本次问题的根因

- 错误做法：把 `host_domain_suffix` 从 `nps1.tx07.cn` 改成了 `151.245.90.96`
- 结果：域名内网穿透被生成为 `https://a1775718020.151.245.90.96`
- 正确做法：
  - 只在 SSH、Bridge、`public_host` 这类“连接服务器或 TCP 入口”的配置里替换 IP
  - 不要改 `VITE_DEFAULT_HOME_URL`、`public_api_base_url`、`host_domain_suffix` 这类“对外域名入口”配置，除非业务上真的切换了主域名

#### 替换服务器 IP 时的检查清单

1. 更新 `deploy/deploy.config.mjs` 的 `ssh.host`
2. 更新 `src/netunnel-desktop-tauri/.env.production` 的 `VITE_DEFAULT_BRIDGE_ADDR`
3. 更新服务端生产配置里的 `public_host`
4. 若域名备案未完成，允许把 `VITE_DEFAULT_HOME_URL` 临时改为 `http://101.43.49.100:40061`
5. 若域名备案未完成，允许把 `public_api_base_url` 临时改为 `http://101.43.49.100:40061`
6. 保持 `host_domain_suffix` 为域名后缀，不要改成 IP
7. 检查线上 `/www/wwwroot/netunnel/shared/config.yaml` 是否与预期一致
8. 重启后端后验证：
   - `curl http://127.0.0.1:40061/api/v1/platform/config`
   - `curl http://101.43.49.100:40061/api/v1/platform/config`
   - 两者都应返回 `{"host_domain_suffix":"nps1.tx07.cn"}`

---

## 调试

- 桌面端: 登录后按 `F12` 打开 DevTools
- 服务端: `src/netunnel-server/server.40061.out.log` / `.err.log`
- Agent: `src/netunnel-agent/agent.40061.out.log` / `.err.log`

---

## 服务部署

### 重启服务端

```powershell
cd src/netunnel-server
powershell -ExecutionPolicy Bypass -File restart-backend.ps1
```

脚本会自动：停止旧进程 → 重新编译 `server-run.exe` → 启动新进程 → 等待健康检查通过。

### 构建服务端（不重启）

```bash
cd src/netunnel-server
go build -o server-run.exe ./cmd/server
```

### 服务配置文件

- 配置文件: `src/netunnel-server/config.yaml`
- 端口范围: `tcp_port_ranges`（支持多组范围，如 40000-45000、50000-60000）

### 服务端口

| 端口 | 协议 | 说明 |
|------|------|------|
| 40061 | HTTP | API 服务端口 |
| 40063 | HTTPS | HTTPS API 服务端口 |
| 40062 | TCP | Agent Bridge 端口（TCP 隧道数据转发）|

---

## GitHub Release

发布新版本流程：

1. 确保所有代码已提交
2. 在项目根目录运行：
   ```bash
   cd src/netunnel-desktop-tauri && node bump-version.cjs x.y.z
   ```
3. 更新根目录 `package.json` 的 version 字段
4. 提交并推送：
   ```bash
   git add .
   git commit -m "chore: bump version to x.y.z"
   git push
   git tag vx.y.z
   git push origin vx.y.z
   ```
5. GitHub Actions 会自动触发 `release.yml` workflow，构建并发布 Release

### 触发 Release 的方式

- Push `v*.*.*` 格式的 tag（如 `git push origin v2.12.0`）
- 或在 GitHub Actions 页面手动触发 `publish` workflow

---

## 后端部署

后端部署使用 `deploy/` 目录下的脚本，支持交叉编译 Linux 版本并上传到服务器。

### 部署配置

服务器信息在 `deploy/deploy.config.mjs` 中配置：

```js
ssh: {
   host: '101.43.49.100',
  port: 22,
  username: 'root',
  password: '你的服务器密码',
}
```

### 首次服务器准备

1. 复制服务文件到服务器：
```bash
sudo cp deploy/netunnel-server.service /etc/systemd/system/netunnel-server.service
```

2. 修改服务文件中的路径（如果部署目录不同）

3. 创建目录：
```bash
sudo mkdir -p /www/wwwroot/netunnel/shared
sudo mkdir -p /www/wwwroot/netunnel/releases
```

4. 启用服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable netunnel-server
```

### 发布后端到服务器

```bash
node ./deploy/deploy.mjs --target backend
```

或：

```bash
pnpm run deploy:server -- --target backend
```

### 发布完成后检查

```bash
# 检查服务状态
sudo systemctl status netunnel-server --no-pager

# 查看日志
sudo journalctl -u netunnel-server -n 100 --no-pager

# 检查健康接口
curl http://127.0.0.1:40061/healthz
```

### Nginx 配置

项目对应的站点配置模板：`deploy/nginx/nps1.tx07.cn.conf`（旧域名模板，如继续使用 IP 直连可忽略）

同步 Nginx 配置到服务器：

```bash
pnpm run sync:nginx:nps1
```

这会备份、上传新模板、测试配置并重载 Nginx。

### 关键路径

- 服务器配置文件：`/www/wwwroot/netunnel/shared/config.yaml`
- 部署根目录：`/www/wwwroot/netunnel`
- 当前版本链接：`/www/wwwroot/netunnel/current`
- 配置文件：`.env.production` 中的 `VITE_API_BASE_URL` 需指向公网地址
