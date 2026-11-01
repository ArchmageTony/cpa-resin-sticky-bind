# cpa-resin-sticky-bind

CLIProxyAPI 原生插件: 为每份凭证绑定稳定的 Resin sticky 代理 Account, 写入凭证级 `proxy_url`; 支持按凭证类型 (codex / antigravity / …) 配置不同代理, 在插件管理页面直接编辑.

| 项 | 值 |
|----|----|
| 插件 ID | `cpa-resin-sticky-bind` |
| 版本 | `0.2.0` |
| 协议 | MIT |

## 行为

1. 启动后与固定间隔枚举 CPA 凭证 (`host.auth.list`)
2. 为每份可持久化凭证解析目标代理并写入 `proxy_url`:

```text
socks5h://<Platform>.<Account>:<token>@resin:2260
```

3. 代理来源按优先级: `proxy_by_type[类型]` → `other_proxy_url` → 旧链路 (`resin_proxy_url` + 平台解析)
4. 默认 `rebind_on_change=false`: 已绑定凭证保持不动; 在页面勾选或配置开启后, 目标代理变化时自动重写 (Account 不变)
5. 日志与页面中 token 始终脱敏

## 按凭证类型配置代理

在插件管理页面 (管理中心 → Resin Sticky Bind) 直接编辑:

- 每个类型一行, 填完整代理 URL; 另有 `other` 行作为未单独配置类型的兜底
- Resin 粘性写法: `socks5h://平台名:token@resin:2260`, 用户名不含点时自动按凭证展开 `平台名.账号`
- 用户名已含点 (`平台.账号`) 则视为完整身份, 该类型所有凭证共用同一出口
- 普通代理填 `http://user:pass@host:port` 并取消勾选该行"粘性"
- 类型清单 = 内置 CPA 支持类型 ∪ 运行时发现的类型 ∪ `other`
- 浅色 / 深色主题: 跟随系统, 页面右上角可手动切换
- 保存走 CPA 管理 API, 自动复用管理面板已登录的密钥 (否则提示输入一次, 仅存于浏览器 sessionStorage)

等价配置 (页面保存后写入 CPA `config.yaml`):

```yaml
cpa-resin-sticky-bind:
  enabled: true
  proxy_by_type:
    codex: "socks5h://<platform-a>:<token>@resin:2260"
    antigravity: "http://<user>:<pass>@<proxy-host>:<port>"
  sticky_by_type:
    codex: true
    antigravity: false
    other: true
  other_proxy_url: "socks5h://<platform-b>:<token>@resin:2260"
  expand_sticky_account: true
  rebind_on_change: false  # 改代理后需自动重写已绑定凭证时改为 true
```

## 安装 (CPA 插件商店)

```yaml
plugins:
  enabled: true
  dir: "plugins"
  store-sources:
    - "https://raw.githubusercontent.com/ArchmageTony/cpa-resin-sticky-bind/main/registry.json"
  configs:
    cpa-resin-sticky-bind:
      enabled: true
      resin_proxy_url: "socks5h://resin:2260"
      proxy_token_env: "RESIN_PROXY_TOKEN"
      default_platform: "default"
      account_strategy: "auth_id"
      sync_interval_seconds: 30
      only_if_empty: true
      overwrite_existing: false
```

1. 将上述配置写入 CPA `config.yaml` 并重启
2. 确保 CPA 容器能访问 GitHub Release (或改用手动安装)
3. 管理中心 → 插件商店 → 安装 `CPA Resin Sticky Bind`
4. 向 CPA 注入 `RESIN_PROXY_TOKEN` (与 Resin 同值), 或在 `resin_proxy_url` 中写 password

## 手动安装

1. 从 Release 下载 `cpa-resin-sticky-bind_{version}_{goos}_{goarch}.zip`
2. 解压后将库文件放到:

```text
plugins/linux/amd64/cpa-resin-sticky-bind.so
```

3. 合并配置并重启 CPA

## 配置说明

| 字段 | 说明 |
|------|------|
| `resin_proxy_url` | 旧链路全局代理入口, 内网示例 `socks5h://resin:2260` |
| `proxy_token_env` | URL 无 password 时读取的环境变量名 |
| `proxy_by_type` | 按凭证类型的完整代理 URL (管理页面写入) |
| `sticky_by_type` | 按类型的粘性展开开关, key `other` 对应兜底行, 缺省 true |
| `other_proxy_url` | 未单独配置类型的兜底代理 |
| `expand_sticky_account` | 未列入 `sticky_by_type` 的类型默认粘性展开 |
| `rebind_on_change` | 目标代理变化时自动重写已绑定凭证 (默认 false, 需显式开启) |
| `default_platform` | 旧链路默认 Platform |
| `platform_by_provider` | 旧链路按 provider 覆盖 Platform |
| `platform_by_auth_id` | 旧链路按 auth id 覆盖 Platform (最高优先级) |
| `account_strategy` | `auth_id` / `email` / `sub` / `filename` |
| `only_if_empty` | 仅空 `proxy_url` 写入 (关闭 `rebind_on_change` 时生效) |
| `overwrite_existing` | 是否允许覆盖已有 `proxy_url` |

不需要配置 CPA Management 地址. 凭证读写走进程内 Host Callback.

## 本地开发

```bash
git clone --depth 1 https://github.com/router-for-me/CLIProxyAPI ./tmp/CLIProxyAPI-src
# go.mod 已含 replace => ./tmp/CLIProxyAPI-src (可用 CPA_SRC 环境变量覆盖)

go test ./internal/stickybind/ -count=1
CGO_ENABLED=1 go build -buildmode=c-shared -o bin/cpa-resin-sticky-bind.so .
# 或直接: scripts/build-local.sh
```

## 安全

- 禁止提交真实 `RESIN_PROXY_TOKEN` / 含 password 的生产配置
- 日志与错误输出不打印 token
- 示例配置仅使用占位符
