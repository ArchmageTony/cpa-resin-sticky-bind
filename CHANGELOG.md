# Changelog

## 0.2.0

- 按凭证类型的代理配置: 新增 `proxy_by_type`（每类型完整代理 URL）、`other_proxy_url`（未匹配类型兜底）
- Resin 粘性写法: 每行用户名不含点时自动按凭证补全 `平台.账号`；含点视为完整身份整类共用；`sticky_by_type` 可按行关闭（普通代理）
- 解析优先级: `proxy_by_type[type]` → `other_proxy_url` → 旧链路 `resin_proxy_url` + 平台解析（v0.1 配置完全兼容）
- `rebind_on_change`（默认关闭）: 开启后改动代理会自动重写该类型已绑定凭证; 关闭时保持 v0.1 行为（只写空 `proxy_url`）
- 管理页面重做: 纯中文表单、浅色/深色主题（跟随系统 + 手动切换）、运行时类型发现（内置 CPA 类型 ∪ `host.auth.list` 实际类型 ∪ other 兜底）、凭证计数
- 页面保存走 CPA 管理 API `PATCH /v0/management/plugins/:id/config`（复用管理面板密钥，页面渲染始终脱敏令牌）
- 旧 `resin_proxy_url` 无令牌时不再阻断同步（仅禁用回退链路）
- 本地构建 SDK checkout 迁移到 `tmp/`（已 gitignore），`scripts/build-local.sh` 支持 `CPA_SRC` 覆盖

## 0.1.2

- 修复状态页 404: 接受 host 传入的完整 resource 路径
- 同时注册 Resources 与 legacy GET+Menu 路由

## 0.1.1

- 修复: CPA 要求至少一种 capability, 注册空 capabilities 导致 `invalid metadata or no capabilities`
- 声明 `management_api` 并提供状态页 / 手动 sync
- 支持 `socks5h://<token>@host:port` (token 写在 username)

## 0.1.0

- 首个可用版本
- Scheduler 风格周期对账: `host.auth.list` / `get` / `save`
- 完整 `resin_proxy_url` 配置 + `proxy_token_env`
- Account 策略: `auth_id` / `email` / `sub` / `filename`
- Platform: 全局默认 + provider / auth_id 覆盖
- 默认不覆盖已有 `proxy_url`
- token 脱敏日志
- 插件商店 `registry.json` 与多架构 CI 发版
