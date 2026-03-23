# nps 客户端安装启动文档

本文档面向当前仓库中的源码目录 [D:\git-projects\ai-company\projects\netunnel\src\nps](/D:/git-projects/ai-company/projects/netunnel/src/nps)。

## 1. 客户端是什么

- 客户端可执行入口：`cmd/npc/npc.go`
- 默认配置文件：`conf/npc.conf`
- 作用：连接 `nps` 服务端，并把本地服务暴露给服务端管理的隧道

## 2. 前置条件

- 服务端已启动
- 已知道服务端桥接地址，例如 `127.0.0.1:8024`
- 已知道验证密钥 `vkey`

当前仓库默认示例值：

- 服务端地址：`127.0.0.1:8024`
- `vkey`：`123`

## 3. 源码构建

在目录 [D:\git-projects\ai-company\projects\netunnel\src\nps](/D:/git-projects/ai-company/projects/netunnel/src/nps) 下执行：

```powershell
$env:GOCACHE = (Resolve-Path '.codex-cache\go-build')
$env:GOMODCACHE = (Resolve-Path '.codex-cache\go-mod')
go build -buildvcs=false -o npc.exe .\cmd\npc\npc.go
```

说明：

- 当前仓库里 `cmd/npc/` 同时存在 `npc.go` 和 `sdk.go`
- 直接执行 `go build .\cmd\npc` 会因为两个 `main` 冲突而失败
- 实际可运行的客户端构建方式是只编译 `cmd/npc/npc.go`

## 4. 最简单的启动方式

### 方式 A：命令行直连

```powershell
.\npc.exe -server=127.0.0.1:8024 -vkey=123 -type=tcp -debug=true
```

如果服务端桥接端口改成了 `18024`，则改成：

```powershell
.\npc.exe -server=127.0.0.1:18024 -vkey=123 -type=tcp -debug=true
```

### 方式 B：配置文件启动

```powershell
.\npc.exe
```

默认会读取：

- Windows：可执行文件目录下的 `conf/npc.conf`

## 5. 最小可用配置示例

仓库自带的 [D:\git-projects\ai-company\projects\netunnel\src\nps\conf\npc.conf](/D:/git-projects/ai-company/projects/netunnel/src/nps/conf/npc.conf) 是演示配置，里面包含大量样例任务，不适合直接原样在本机使用。

建议先从最小配置开始：

```ini
[common]
server_addr=127.0.0.1:8024
conn_type=tcp
vkey=123
auto_reconnection=true
crypt=true
compress=true
disconnect_timeout=60
```

如果服务端桥接端口改成了 `18024`，同步改为：

```ini
server_addr=127.0.0.1:18024
```

## 6. 已验证的本地启动结果

本地于 `2026-03-20` 已验证：

- 客户端可成功连接到 `127.0.0.1:18024`
- 客户端日志出现 `Successful connection with server 127.0.0.1:18024`
- 服务端日志出现 `clientId 2 connection succeeded`

## 7. 为什么示例配置会报错

仓库默认 `npc.conf` 里有多组演示任务，例如：

- `file`
- `udp`
- `p2p`
- `secret`

这些演示项在当前机器上直接跑，容易出现：

- 目标地址不存在
- 本地目录不存在
- 端口冲突
- 某些字段为空导致 `strconv.Atoi` 报错

因此推荐流程是：

1. 先只保留 `[common]`
2. 先确认客户端能连上服务端
3. 再按实际业务逐个添加隧道配置

## 8. 发行包/系统服务方式启动

如果走发布包方式：

```powershell
npc.exe install
npc.exe start
```

说明：

- Windows 需要管理员权限
- 更适合长期驻留
- 源码调试阶段建议先用前台运行观察日志

## 9. 常见问题

### 9.1 客户端连不上服务端

检查：

- `server_addr` 是否正确
- 服务端 `bridge_port` 是否已启动
- `vkey` 是否一致
- 服务端防火墙是否放行桥接端口

### 9.2 客户端一启动就刷错误日志

优先检查是否直接用了仓库自带的完整示例 `npc.conf`。如果是，先精简到只保留 `[common]` 再试。

### 9.3 想从 Web 管理端生成启动命令

这是更推荐的方式：

1. 先登录服务端 Web 管理端
2. 创建客户端
3. 点击客户端前面的 `+`
4. 复制页面生成的启动命令
