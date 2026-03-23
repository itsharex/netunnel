# Netunnel Agents

## 项目结构

```
src/
├── netunnel-desktop-tauri/   # Tauri 桌面端 (Vue 3 + Tailwind + Pinia + TypeScript)
├── netunnel-server/          # Go 后端服务 (pgx/v5 + 标准库)
├── netunnel-agent/           # Go Agent 客户端
└── netunnel-desktop/         # 旧版前端原型 (废弃，勿用)
```

---

## 开发命令

### Tauri 桌面端 (`src/netunnel-desktop-tauri/`)

| 命令 | 说明 |
|------|------|
| `pnpm dev` | 启动开发服务器 |
| `pnpm build` | 类型检查 + 构建生产版本 |
| `pnpm type-check` | TypeScript 类型检查 |
| `pnpm test` | 运行所有测试 |
| `pnpm test src/services/api.test.ts` | 运行单个测试文件 |
| `pnpm test --watch src/services/api.test.ts` | 监听模式运行单个测试 |
| `pnpm tauri dev` | 启动 Tauri 开发模式 |
| `pnpm tauri build` | 构建 Tauri 安装包 |
| `pnpm check` | 运行 Cargo 检查 (Rust) |

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

桌面端使用 Vite，默认前缀 `VITE_` / `TAURI_`。

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
