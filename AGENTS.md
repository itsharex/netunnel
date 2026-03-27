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
- `VITE_API_BASE_URL` — API 基础地址（默认 http://localhost:40061）
- `VITE_WS_URL` — WebSocket 地址（默认 ws://localhost:40061）

服务端使用环境变量或 `config.yaml`，关键变量:
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- `SERVER_PORT` (默认 40061)
- `BRIDGE_PORT` (默认 40062)

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
  host: '110.42.111.221',
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

项目对应的站点配置模板：`deploy/nginx/nps1.tx07.cn.conf`

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
