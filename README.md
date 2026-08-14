# AlertHub

> 可自托管的**企业级 mass-notification / 紧急告警平台**：把一条告警（地震、火灾、系统故障…）签名后实时、可靠地推送到组织内所有端点，并按紧急程度以不同强度呈现——从安静的 toast 到强制接管的全屏 + 警报音 + 强制确认。
>
> 开源可自托管的 Everbridge / xMatters / Alertus 替代品。Go **单二进制**内嵌 MQTT broker + HTTP API + 告警客户端 + React 管理控制台；零外部依赖（无需 Docker / Mosquitto / 独立前端服务）。

**核心取向**：Ed25519 端到端签名 + 防重放、**FAIL-LOUD**（系统自己坏了必须让人知道）、独立兜底通道、单二进制上手、安全功能不付费墙（反 sso.tax）。

> ⚠️ **诚实定位**：AlertHub **补充**官方渠道（运营商緊急地震速報、认证烟感、IPAWS/WEA），**不取代**。iPhone 无法保证唤醒静音机；睡眠/关机设备收不到；EEW 来自免费第三方中继，大震时最易挂。上线前必读 [SPEC-SAFETY.md](SPEC-SAFETY.md) §0 / §11。

---

## 快速开始

```bash
make web-install   # 首次：安装管理控制台依赖（npm）
make web-build     # 首次：构建 React SPA 到 go:embed 目录（不构建则 /admin 空白）
make run           # = go run ./server，启动 broker + API + 告警客户端 + /admin
```

然后：

1. **告警客户端** <http://localhost:8080/> → 点「启用警报」
2. **发布面板** <http://localhost:8080/publish.html> → 点「发送警报」→ 客户端立刻全屏 + 警报音 + 强制确认
3. **管理控制台** <http://localhost:8080/admin/> → 用首次启动时终端打印的 `admin` 账号密码登录

首次启动自动在 `keys/` 生成 Ed25519 签名密钥、JWT secret、密文落盘 KEK（均已 gitignore）；未设 `ALERTHUB_ADMIN_PASS` 时随机生成初始管理员密码并**只打印一次**。

```bash
make build   # web-build + go build -o bin/alerthub ./server（发布用单二进制）
make ci      # 本地跑完整 CI 门禁：gofmt + vet + build + test + 跨语言签名一致性
```

> 注：Go module path 目前是 `github.com/kazuha/alerthub`，与仓库地址不一致，因此 `go install` 不可用——请用 `make build` 或克隆后自行构建。

---

## 两个部署档

同一套代码、同一套设备协议，按规模升级：

| 档 | 存储 | broker | 形态 | 适用 |
|---|---|---|---|---|
| **Starter** | SQLite 单文件（modernc，纯 Go） | 内嵌 mochi-mqtt | **单二进制** | 自托管、homelab、单楼、试用 |
| **Enterprise** | **PostgreSQL**（pgx）+ **行级安全 RLS** | 内嵌 mochi-mqtt | 单二进制（镜像需自行构建） | 多租户、需要数据库层租户隔离兜底 |

切换只需 `ALERTHUB_DB_DRIVER=postgres` + `ALERTHUB_DB_DSN`。RLS 的启用方式、最小权限角色与验证方法见 [docs/POSTGRES.md](docs/POSTGRES.md)。

> ARCHITECTURE.md 里规划的**外部 EMQX 集群档**尚未实现——当前所有档位都跑内嵌 broker，单节点。

---

## 配置（环境变量）

共 46 个运行时变量（另有 1 个测试专用）。标 **⚠️生产必改** 的项使用不安全的开发默认值。

### 核心

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_HTTP_ADDR` | `:8080` | HTTP 监听（API + 告警客户端 + /admin） |
| `ALERTHUB_MQTT_TCP` | `:1883` | MQTT/TCP 监听 |
| `ALERTHUB_MQTT_WS` | `:1884` | MQTT/WebSocket 监听（浏览器客户端连这个） |
| `ALERTHUB_WEB_DIR` | `web` | 告警客户端静态目录 |
| `ALERTHUB_KEY_DIR` | `keys` | Ed25519 密钥 / JWT secret / KEK 存放目录 |

### 存储

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_DB_DRIVER` | `sqlite` | `sqlite`（Starter）或 `postgres`（Enterprise） |
| `ALERTHUB_DB_PATH` | `alerthub.db` | SQLite 文件路径 |
| `ALERTHUB_DB_DSN` | （空） | Postgres 连接串；`DB_DRIVER=postgres` 时必填 |

### Broker 凭据

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_MQTT_USER` | `publisher` | 发布者账号（服务端自用） |
| `ALERTHUB_MQTT_PW` | `dev-publisher-pw` | 发布者密码 **⚠️生产必改** |
| `ALERTHUB_CLIENT_USER` | `client` | 订阅端账号（下发给浏览器客户端） |
| `ALERTHUB_CLIENT_PW` | `dev-client-pw` | 订阅端密码 **⚠️生产必改** |

### 管理员 / 静态令牌

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_ADMIN_TOKEN` | `dev-admin-token` | 脚本用静态 Bearer token，绕过 RBAC **⚠️生产必改** |
| `ALERTHUB_ADMIN_USER` | `admin` | 首次启动播种的管理员用户名（同时是平台 super-admin） |
| `ALERTHUB_ADMIN_PASS` | （空=随机） | 留空则随机生成并在日志中**只打印一次** |

### Passkey / WebAuthn

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_RP_ID` | `localhost` | WebAuthn RP ID；取自配置，**绝不取 Host 头** |
| `ALERTHUB_RP_ORIGIN` | `http://localhost:8080` | WebAuthn 允许的 origin |

### ntfy 独立兜底通道

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_NTFY_URL` | （空=禁用） | 自托管 ntfy 基址，转发全部 severity |
| `ALERTHUB_NTFY_TOKEN` | （空） | 自托管 ntfy bearer token |
| `ALERTHUB_NTFY_TOPIC_PREFIX` | `alerthub-` | 自托管 ntfy topic 前缀 |
| `ALERTHUB_NTFY_SH_TOPIC` | （空=禁用） | 公共 ntfy.sh 兜底 topic；**仅 critical/emergency**，文案通用不含 PII |

### 送达通道（持久化 outbox）

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_WEBHOOK_URLS` | （空=禁用） | Webhook 地址，逗号分隔；全部 severity |
| `ALERTHUB_SMTP_HOST` | （空=禁用） | SMTP 服务器 |
| `ALERTHUB_SMTP_PORT` | `587` | SMTP 端口 |
| `ALERTHUB_SMTP_USER` | （空） | SMTP 用户名 |
| `ALERTHUB_SMTP_PASS` | （空） | SMTP 密码 |
| `ALERTHUB_SMTP_FROM` | （空） | 发件人地址 |
| `ALERTHUB_ALERT_EMAILS` | （空） | 收件人，逗号分隔；**仅 critical/emergency** 触发 |

### 告警源

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_EEW` | （空=关闭） | 设为 `wolfx` 启用日本 EEW（Wolfx WebSocket，按 EventID 去重） |

### OIDC 单点登录

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_OIDC_ENABLED` | （空=关闭） | 设为 `true` 启用；discovery 失败会自动降级为关闭 |
| `ALERTHUB_OIDC_ISSUER` | （空） | IdP issuer URL（用于 discovery） |
| `ALERTHUB_OIDC_CLIENT_ID` | （空） | client id |
| `ALERTHUB_OIDC_CLIENT_SECRET` | （空） | client secret |
| `ALERTHUB_OIDC_REDIRECT` | `http://localhost:8080/api/auth/oidc/callback` | 回调地址 |
| `ALERTHUB_OIDC_DEFAULT_ROLE` | `admin` | JIT 建号时的默认角色，**OIDC 与 SAML 共用**（生产建议下调，如 `viewer`） |

### SAML 2.0 单点登录

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_SAML_ENABLED` | （空=关闭） | 设为 `true` 启用 |
| `ALERTHUB_SAML_ENTITY_ID` | `http://localhost:8080/api/auth/saml/metadata` | SP EntityID |
| `ALERTHUB_SAML_ACS` | `http://localhost:8080/api/auth/saml/acs` | Assertion Consumer Service URL |
| `ALERTHUB_SAML_METADATA_URL` | 同 `ENTITY_ID` | SP metadata 对外地址 |
| `ALERTHUB_SAML_IDP_METADATA` | （空） | IdP metadata（内联 XML） |
| `ALERTHUB_SAML_IDP_METADATA_URL` | （空） | IdP metadata 拉取地址 |
| `ALERTHUB_SAML_ATTR_USERNAME` | `nameid` | 用户名断言属性 |
| `ALERTHUB_SAML_ATTR_EMAIL` | `email` | 邮箱断言属性 |
| `ALERTHUB_SAML_ATTR_NAME` | `displayName` | 显示名断言属性 |
| `ALERTHUB_SAML_ATTR_GROUPS` | `groups` | 组断言属性 |
| `ALERTHUB_SAML_ALLOW_IDP_INITIATED` | （空=**关闭**） | IdP-initiated SSO 是 CSRF/重放面，默认关闭，需显式设 `true` |

### 测试专用

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_TEST_PG_DSN` | （空） | 指向一次性数据库；不设则 Postgres/RLS 集成测试干净跳过 |

---

## 状态

诚实版。**没做的一律标没做**——人身安全产品上，夸大的文档比没有文档更糟。

### SPEC-SAFETY §10 路线图

| 阶段 | 内容 | 状态 |
|---|---|---|
| MVP | 内嵌 broker + Ed25519 签名 + web 客户端/面板 + 全屏接管 | ✅ 完成（跨语言一致性测试通过） |
| **P0** | fail-loud 心跳 + ntfy 独立通道 | ⚠️ **部分**：A 层签名心跳（10s）、浏览器看门狗（30s degraded / 60s offline）、时钟漂移横幅、ntfy 自托管 + ntfy.sh 扇出均已完成；服务端自检**已闭环**（心跳带签名 `health`，存储或 broker 故障即自报降级，客户端显示独立横幅）。**仍缺外部 dead-man switch**（P0-B / P4），进程整体死亡时无人能看见 |
| P1 | CAP 1.2 ingest + 场景模板 | ⚠️ **部分**：CAP 1.2 XML ingest 与 CAP Cancel 已完成（确定性 id `cap.AlertID(sender,identifier)` 让 `<references>` 精确召回）；**Update-as-supersede、`<area>`/geocode 定向未做**；出向 CAP、场景模板未做 |
| P2 | 日本 EEW 双源 + renew 去重 | ⚠️ **部分**：Wolfx 单源已接（按 EventID 去重，默认关闭）；**P2PQuake 第二源未接，renew/serial 升级未做**——SPEC-SAFETY §6.1 要求的双源尚未满足 |
| P3 | 确认送达升级状态机 + 名册视图 + 每周演练 cron | ❌ 未开始 |
| P4 | **外部看门狗**（healthchecks.io dead-man + CF Worker） | ❌ 未开始——**这是 P0 不能算完成的原因**：进程整个死掉时，A 层心跳跟着一起死，没有系统外的东西来喊 |
| P5 / P6 / P7 | Android 原生端 / Tauri 桌面壳 / iOS + Critical Alerts | ❌ 未开始 |

### 企业平台层

| 能力 | 状态 |
|---|---|
| 认证：JWT（access 2h / refresh 7d，含 TokenVersion 吊销位）、bcrypt、passkey/WebAuthn（usernameless）、TOTP 2FA + 一次性恢复码（AES-256-GCM 落盘）、OIDC（PKCE + nonce + JIT）、SAML 2.0 | ✅ 但**单租户**：全局一份 IdP 配置（来自环境变量），无每租户 SSO、无 IdP 组→角色映射 |
| 凭据端点每 IP 限流（10 次/分，429 + Retry-After）、OIDC state 常量时间比较 | ✅ |
| 多租户：org + membership + RBAC（9 项权限，`X-Org-Id` > API key 所属 org > 默认 org） | ✅ |
| **Postgres RLS 已上线**：`Store.BeginOrg` 设置事务级 `app.current_org`，读墙（USING）与写墙（WITH CHECK）在真实 PG + 最小权限角色下均已验证 | ✅ |
| 持久化送达：事务性 outbox、至少一次、按 (alert, channel, target) 幂等、指数退避、租约式崩溃恢复、死信；Postgres 用 `FOR UPDATE SKIP LOCKED` | ✅ |
| 可观测性：Prometheus 指标 + `/healthz` + `/readyz` | ✅ |
| HTTP：gzip 压缩（实测 ~3.1×）+ 非对称缓存策略（`/admin/assets/*` immutable；告警客户端钉死 `no-cache`；JSON 全部 `no-store`） | ✅ |
| 管理控制台：React 19 + Ant Design 6 + Vite 8，M3 动态取色（6 套预设 / 明暗自动），中英 i18n，go:embed 内嵌 | ✅（`/devices`、`/history`、`/sources` 三个视图仍是「敬请期待」占位） |
| CI：4 个 job（SQLite 门禁 / Postgres RLS / Node 22 签名一致性 / SPA typecheck + build） | ✅ |
| **设备与广播面仍是单租户**：`/api/devices` 返回全局在线名册，`alerts/active`、`alerts/events` 是全局 topic，设备没有 `org_id`（设备下发未建）。**即：控制面多租户，设备面单租户** | ❌ |
| SCIM 2.0、审计日志、EMQX 集群总线、leader 选举 | ❌ 未开始 |

---

## 文档

| 文件 | 内容 |
|---|---|
| [SPEC.md](SPEC.md) | **v1 协议权威契约**：13 字段规范化信封、Ed25519 签名、防重放接收门、MQTT 拓扑、两步撤回、broker ACL |
| [SPEC-SAFETY.md](SPEC-SAFETY.md) | 人身安全级硬化：冗余多通道、fail-loud、确认送达/升级、日本 EEW、ntfy、CAP、各平台唤醒、**§11 诚实限制** |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 企业架构契约：部署分层、多租户数据模型、RBAC、SSO、规模化路径 |
| [docs/API.md](docs/API.md) | HTTP API 参考：全部路由、鉴权方式、权限矩阵、请求/响应 |
| [docs/POSTGRES.md](docs/POSTGRES.md) | Enterprise 档：Postgres 接入、RLS 策略、最小权限角色、集成测试 |
| [docs/DESIGN.md](docs/DESIGN.md) | 最初的设计讨论存档 |

> 阅读顺序：线路协议看 SPEC.md，人身安全设计与**真实建设状态**看 SPEC-SAFETY.md §10/§11，企业架构与差距对照看 ARCHITECTURE.md §11，接口细节看 docs/API.md。各文档已与代码对齐；SPEC.md §0 明确标注了其中尚未落地的条款。

---

## 测试

```bash
make test-go   # Go 单元测试（SQLite 路径）
make test      # 跨语言签名一致性：Go 签名 ↔ web/verify.js 验签，含篡改拒绝
make ci        # 完整门禁：gofmt-check + vet + build + test + conformance
```

Postgres / RLS 集成测试需要一个一次性数据库：

```bash
ALERTHUB_TEST_PG_DSN="postgres://$USER@localhost:5432/alerthub_test?sslmode=disable" make test-pg
```

不设 `ALERTHUB_TEST_PG_DSN` 时这些用例干净跳过（CI 里有反静默跳过的 grep 断言）。

覆盖诚实说明：除 `metrics`、`webadmin`、`main` 外每个包都有测试。`store` 同时覆盖两种方言——`store_test.go` 跑 SQLite（默认门，74.5%），`pg_test.go` 跑 Postgres + RLS（需 DSN，合计 85.7%）。

---

## 许可证

AGPL-3.0（社区版）+ 商业双授权。核心告警链路、SSO、MFA、RBAC 永久免费开源（反 sso.tax）。详见 [LICENSE](LICENSE) 与 [NOTICE](NOTICE)。
