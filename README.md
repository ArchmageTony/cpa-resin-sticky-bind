# cpa-resin-sticky-bind

CLIProxyAPI 原生插件: 为每份凭证绑定稳定的 Resin sticky 代理 Account, 写入凭证级 `proxy_url`; 支持按凭证类型 (codex / antigravity / …) 配置不同代理, 在插件管理页面直接编辑.

| 项 | 值 |
|----|----|
| 插件 ID | `cpa-resin-sticky-bind` |
| 版本 | `0.2.4` |
| 协议 | MIT |

## 行为

1. 启动后与固定间隔枚举 CPA 凭证 (`host.auth.list`)
2. 按凭证类型解析目标代理并写入凭证级 `proxy_url`:

```text
socks5h://<Platform>.<Account>:<token>@resin:2260
```

3. 代理来源按优先级: `proxy_by_type[类型]` → `other_proxy_url`（全局兜底）→ 弃用的旧链路（仅为兼容 v0.1 配置保留）
4. 默认 `rebind_on_change=false`: 已绑定凭证保持不动; 在页面勾选或配置开启后, 目标代理变化时自动重写 (Account 不变)
5. 日志与页面中 token 始终脱敏
6. 单活实例: 同步循环启动时写入租约文件 `lease_file` (默认 `plugins/data/cpa-resin-sticky-bind/instance.lease`), 热升级后旧实例发现租约易主即停止同步, 避免新旧实例互相覆写
7. 外部回写保护 (`write_backoff`) 有上限: 连续退让 10 轮后重试一次, 并输出脱敏 warn

## 按凭证类型配置代理

在插件管理页面 (管理中心 → Resin Sticky Bind) 直接编辑, 每个类型一行完整代理地址, **不同类型可自由混用 Resin 与普通供应商代理**:

- **粘性代理**: 地址中包含 `{account}` 占位符即为账号级粘性, 占位符按 Account 策略逐凭证替换:

  ```text
  socks5h://US.{account}:<token>@resin:2260
  → socks5h://US.<账号>:<token>@resin:2260   (每份凭证独立出口)
  ```

- **非粘性**: 不含 `{account}` 的地址整类凭证共用, 原样直连:

  ```text
  http://<user>:<pass>@<proxy-host>:<port>
  ```

- `other` 行是未单独配置类型的全局兜底, 同样支持 `{account}`
- 类型清单 = 内置 CPA 支持类型 ∪ 运行时发现的类型 ∪ `other`
- 浅色 / 深色主题: 跟随系统, 页面右上角可手动切换
- 保存走 CPA 管理 API, 自动复用管理面板已登录的密钥 (否则提示输入一次, 仅存于浏览器 sessionStorage)

等价配置 (页面保存后写入 CPA `config.yaml`):

```yaml
cpa-resin-sticky-bind:
  enabled: true
  proxy_by_type:
    codex: "socks5h://<platform>.{account}:<token>@resin:2260"
    antigravity: "http://<user>:<pass>@<proxy-host>:<port>"
  other_proxy_url: "socks5h://<platform>.{account}:<token>@resin:2260"
  account_strategy: "auth_id"
  rebind_on_change: false
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
| `proxy_by_type` | 按凭证类型的代理地址 (管理页面写入); 含 `{account}` = 粘性, 不含 = 整类共用 |
| `other_proxy_url` | 未单独配置类型的全局兜底, 同样支持 `{account}` |
| `rebind_on_change` | 目标代理变化时自动重写已绑定凭证 (默认 false, 需显式开启) |
| `account_strategy` | `auth_id` / `email` / `sub` / `filename`, 决定 `{account}` 的取值 |
| `account_prefix` | 可选的 Account 前缀 |
| `sync_interval_seconds` | 凭证对账间隔 |
| `only_if_empty` / `overwrite_existing` | 旧开关, 关闭 `rebind_on_change` 时生效 |
| `include_providers` / `exclude_providers` | 类型过滤 |
| `resin_proxy_url` / `default_platform` / `platform_by_provider` / `platform_by_auth_id` / `proxy_token_env` | **弃用**: v0.1 旧链路, 仅在 other 为空时兜底, 仅为兼容保留 |

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
