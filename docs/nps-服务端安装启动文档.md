# nps 服务端安装启动文档

本文档面向当前仓库中的源码目录 [D:\git-projects\ai-company\projects\netunnel\src\nps](/D:/git-projects/ai-company/projects/netunnel/src/nps)。

## 1. 服务端是什么

- 服务端可执行入口：`cmd/nps/nps.go`
- 默认配置文件：`conf/nps.conf`
- Web 管理端默认账号密码：`admin / 123`
- Web 管理端默认端口：`8080`
- 客户端桥接默认端口：`8024`

## 2. 前置条件

- 已安装 Go，当前环境实测版本：`go1.23.2 windows/amd64`
- Windows 下如果直接使用默认配置，需要确认 `80`、`443`、`8080`、`8024` 端口可用
- 源码构建时建议显式关闭 VCS stamping，否则 Go 1.23 可能报错

## 3. 源码方式启动

在目录 [D:\git-projects\ai-company\projects\netunnel\src\nps](/D:/git-projects/ai-company/projects/netunnel/src/nps) 下执行：

```powershell
$env:GOCACHE = (Resolve-Path '.codex-cache\go-build')
$env:GOMODCACHE = (Resolve-Path '.codex-cache\go-mod')
go build -buildvcs=false -o nps.exe .\cmd\nps
.\nps.exe
```

说明：

- `nps` 会读取可执行文件所在目录下的 `conf/nps.conf`
- Web 静态资源和页面会从可执行文件所在目录下的 `web/static`、`web/views` 读取
- 因此如果把 `nps.exe` 拷贝到别的目录运行，需要同时带上 `conf/` 和 `web/`

## 4. 默认配置的关键端口

配置文件 [D:\git-projects\ai-company\projects\netunnel\src\nps\conf\nps.conf](/D:/git-projects/ai-company/projects/netunnel/src/nps/conf/nps.conf) 默认使用：

- `80`：HTTP 域名代理
- `443`：HTTPS 域名代理
- `8080`：Web 管理端
- `8024`：客户端桥接端口

如果只是本机调试，建议先改成高位端口，例如：

```ini
http_proxy_port=18081
https_proxy_port=18443
bridge_port=18024
web_port = 18080
```

## 5. 已验证的本地启动结果

本地于 `2026-03-20` 按源码方式验证通过，测试端口使用：

- `18024`：bridge
- `18080`：Web 管理端
- `18081`：HTTP 代理
- `18443`：HTTPS 代理

验证结果：

- 服务端前台启动成功
- `http://127.0.0.1:18080/` 可访问，返回 `200`
- 服务端日志出现 `web management start, access port is 18080`
- 客户端接入后，服务端日志出现 `clientId 2 connection succeeded`

## 6. 发行包/系统服务方式启动

如果走上游 README 的发布包模式：

```powershell
nps.exe install
nps.exe start
```

说明：

- Windows 需要管理员权限
- 安装后配置通常位于 `C:\Program Files\nps`
- 这套方式更适合长期运行，不适合当前这种源码调试

## 7. 登录与初始化

启动成功后：

1. 访问 `http://服务器IP:8080`，或你改过后的 Web 端口
2. 使用 `admin / 123` 登录
3. 第一时间修改默认密码
4. 在 Web 管理端创建客户端
5. 为客户端创建 TCP/UDP/HTTP/SOCKS5 等隧道

## 8. 常见问题

### 8.1 Web 能启动日志但页面打不开

优先检查：

- Web 端口是否被占用
- 是否在前台交互式运行
- 可执行文件目录下是否带了 `web/` 和 `conf/`

### 8.2 启动时报端口占用

修改 [D:\git-projects\ai-company\projects\netunnel\src\nps\conf\nps.conf](/D:/git-projects/ai-company/projects/netunnel/src/nps/conf/nps.conf) 中的端口后重启。

### 8.3 只复制了 `nps.exe` 就运行失败

源码运行不是只靠单个 exe，至少还要有：

- `conf/`
- `web/`
