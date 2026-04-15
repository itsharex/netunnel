# Netunnel Docs

Netunnel 相关文档入口。

## 当前重点文档

- [数据面迁移上线观察清单](./数据面迁移上线观察清单.md)
- [新内网穿透系统技术方案](./新内网穿透系统技术方案.md)
- [开发拆分与落地顺序](./开发拆分与落地顺序.md)
- [deployment](./deployment.md)
- [netunnel-桌面端接口集成文档](./netunnel-桌面端接口集成文档.md)
- [netunnel-桌面端原型说明](./netunnel-桌面端原型说明.md)

## 说明

- 当前项目已不是“规划中”状态，桌面端、服务端、agent 均已存在实际代码。
- 当前数据面已迁移为：TCP 与 `http_host` 走 `data session + substream`。
- 与迁移状态、灰度开关、日志观察相关的最新入口文档优先参考：
  - 根目录 `README.md`
  - 根目录 `AGENTS.md`
  - 本目录的《数据面迁移上线观察清单》
