# netunnel-server

## Air

开发期推荐使用 `air` 自动重编译并热重启 Go 后端。

首次安装：

```powershell
go install github.com/air-verse/air@latest
```

启动方式：

```powershell
cd src/netunnel-server
.\run-air.ps1
```

`run-air.ps1` 会在启动前自动释放 `40061`、`40062`、`40063` 上已有的旧监听进程，避免新实例因为端口占用而启动失败。

如果 PowerShell 拦执行脚本，可以先执行：

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\run-air.ps1
```

当前配置文件：

- `.air.toml`

开发期产物会写到：

- `.air/`
- `.gocache/`
