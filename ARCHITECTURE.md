# AlertHub — 企业级架构（ARCHITECTURE）

> AlertHub 从"自托管家庭警报"升级为**企业级开源 mass-notification / 紧急告警平台**（可自托管的 Everbridge/xMatters/Alertus 替代）。
> 本文件是**企业架构契约**；协议细节见 [SPEC.md](SPEC.md)（签名/MQTT/ACL）与 [SPEC-SAFETY.md](SPEC-SAFETY.md)（fail-loud/冗余/EEW/CAP）。
> 依据：2026-06 对 Everbridge / xMatters / AlertMedia / Alertus / Singlewire / PagerDuty 及 SaaS 架构实践的联网调研。

---

## 0. 定位与市场切口

- **补充官方渠道，不取代**（cell broadcast / 认证烟感）；IPAWS/WEA 是政府 Alerting Authority 专属，我们只做 **CAP 1.2 互通**。
- **两个品类的中间**：MNS（广播+物理端点+安全确认，Everbridge/Alertus）× incident（on-call+升级，PagerDuty/xMatters）。AlertHub 两边都借鉴。
- **真实切口**：**Opsgenie 正被砍**（2027-04 EOS）、**Grafana OnCall OSS 已归档**（2026-03）——最好的开源 on-call/告警工具刚消失，留下空位。
- **差异化武器**：**开源 + 可自托管 + FAIL-LOUD + 单二进制上手**（竞品全是闭源 SaaS）。反 sso.tax：安全功能不付费墙。

---

## 1. 部署分层（同一套代码，按规模降级/升级）

| 档 | 存储 | broker | 形态 | 适用 |
|---|---|---|---|---|
| **Starter** | sqlite（单文件） | 内嵌 mochi | **单二进制 / docker-compose** | 自托管、homelab、单楼、试用（差异化卖点）|
| **Enterprise** | **Postgres**（主从）| **外部 EMQX 集群** | **Helm/K8s**，多副本无状态 | 多租户、万级~50万端点、HA |

单二进制 + go:embed 是真优势，**主推**；Postgres/EMQX 是规模档的可插拔升级，不改设备协议。

> ⚠️ **本表是目标形态，不是现状。** 当前仓库**没有** Dockerfile / compose / Helm / kustomize，也**没有**可插拔 Bus——两档都跑内嵌 mochi 单进程，Postgres 主从与 EMQX 集群均未接入。已实现的只有「sqlite ↔ Postgres 双驱动 + RLS」。逐项对照见 §11。

---

## 2. 多租户 + 数据模型

- **行级 `org_id`**（不是 schema-per-tenant，不是 db-per-tenant）+ **Postgres RLS 兜底**（仅 Postgres 档）。单组织自托管 = 自动建一个 `default` org，迁移纯增量、对小用户透明。
- **铁律**：`org_id` 必须是每张租户表 PK/复合索引的**首列**（否则 RLS 慢 ~100×）；`SET LOCAL app.current_org` 只来自服务端鉴权上下文，绝不取用户输入。
- **身份模型**：`user` **全局**（一套登录/passkey/TOTP），经 `membership(org_id,user_id,base_role)` 挂到多个组织（Grafana/GitLab 同款）——SSO/SCIM 配一次身份，MSP/顾问一账号服务多组织，super-admin = `user.is_superadmin`。

核心实体（除 `org`/`user`/`permission` 目录外都带 `org_id`）：
```
org ─┬ membership(+custom roles)
     ├ site → zone                 (楼/层/区，物理定向)
     ├ user_profile                (org 内属性，驱动动态群组)
     ├ group → group_member / group_rule(JSON AST)
     ├ device → endpoint           (设备 / 可达通道)
     ├ role → role_permission      (自定义角色)
     ├ template / scenario         (场景模板)
     ├ alert → alert_recipient → ack  (物化定向结果=投递worklist+ack名册)
     └ audit_event                 (append-only)
```

---

## 3. RBAC（Grafana + GitLab 取长）

- **两层**：基础角色（`owner/org_admin/dispatcher/operator/viewer`，每 membership 必有）+ 自定义角色（挑一个 base_role 再加 `resource:action` 权限袋）。冲突时**更宽松者胜**（GitLab 规则）。`is_superadmin` 绕过 org 作用域。
- 权限目录 `resource:action`（种子、非每租户）：`alert:publish/cancel/read`、`scenario:run/manage`、`device:provision/manage`、`group:manage`、`site:manage`、`template:manage`、`member:manage`、`role:manage`、`org:billing`、`audit:read`、`key:rotate`。
- **按请求加载 + 缓存，不塞进 JWT**（权限变更要即时生效；多组织成员 token 会膨胀）。JWT 只带 `uid/tid/token_version/is_superadmin`。中间件 `RequirePerm("alert:publish")`。
- 手写 ~200 行 RBAC（一个 `Authorizer` 接口）即可；只有将来要 ABAC（"只能发给自己负责的站点"）才上 Casbin。

---

## 4. 目标定向（send-time 解析）

定向是**选择器**，发布时解析成具体 **endpoint 集**，物化进 `alert_recipient`：
```
selector{ groups[], sites[], zones[], geo{center,radius}, rule(JSON AST), all }
  → 并集解析成 user 集（静态群组 ∪ 动态规则匹配 ∪ 站点/区 ∪ geo(PostGIS ST_DWithin)）
  → 展开成 endpoint（mqtt/ntfy/email/sms/push）→ 去重
  → 按 severity 排通道（SPEC 呈现策略）→ 批量 INSERT alert_recipient(state=queued)
  → 签名一次（SPEC §3）→ 发 MQTT + 扇出独立通道
```
- 动态群组规则存**类型化 JSON AST**（不发明字符串 DSL），send-time 编译成带 `org_id` 首列索引的 `WHERE`（权威），可选物化缓存加速 UI 计数。
- `alert_recipient` 一处物化 = 投递 worklist + ack 名册 + 升级判据，且可审计"到底发给了谁"。

---

## 5. 存储层（可插拔）

- **`sqlc` + `Store` 接口 + 两实现（sqlite / postgres）**，不用 GORM（要手调 SQL + RLS 索引纪律 + 可测）。`golang-migrate` 分方言迁移目录。
```
internal/store/  store.go(接口)  queries/  sqlite/  postgres/(pgx+RLS)  migrations/{sqlite,postgres}/
```
- 方言漂移（UUID/jsonb/geography/RLS/ON CONFLICT/ANY($1)）→ 两套 `.sql` 各自地道；共享逻辑（签名、定向数学）在接口之上。
- 诚实代价：双方言是持续维护成本；逃生口=Postgres-only（homelab 也跑 Postgres）。**只要"单二进制"是产品目标就保留 sqlite**，但 Postgres 是企业 canonical，sqlite 档文档化功能缺口（无 RLS、无 geo 定向）。

---

## 6. 规模 / 可靠性 / 可观测

- **broker 可插拔 `Bus` 接口**：内嵌 mochi（默认，≤~5万）→ **EMQX 集群**（企业；验证过 100M 连接、MQTT5 共享订阅、cluster-wide retained）。**设备始终说 MQTT，只换 broker 不换协议。** NATS/Kafka 只用于**内部投递流水线**，不放设备边缘。
- **投递流水线**：单事务写 `alerts` + **outbox**（`delivery_jobs`）→ 每通道 worker（push/sms/voice/email/mqtt/ntfy），at-least-once + 幂等键 + 每通道限速/退避。**队列自研，不用 River**——River 只支持 Postgres，而 Starter 档承诺单二进制 + sqlite，两档必须共用同一份投递语义（Postgres 侧用 `FOR UPDATE SKIP LOCKED`）。超 ~1万事件/s 才把内部跳转移到 NATS JetStream。
- **HA**：无状态 Go 多副本（worker 用 `FOR UPDATE SKIP LOCKED` 竞争，无需 leader）；Postgres 主从；EMQX 集群（≥3）。单例任务（TTL 清扫、升级计时、演练、心跳）用 **Postgres advisory-lock leader 选举**（不引 etcd/Consul）。
- **FAIL-LOUD 升级到集群**：leader 签发 `system/heartbeat`；leader 死则秒级接管；整集群死则客户端看门狗触发；**外部 off-cluster dead-man's-switch** 经 ntfy 报警（看门狗不能与被看者共命运）；心跳要编码"投递流水线健康"。
- **可观测**：Prometheus（`alert_fanout_seconds`、`delivery_latency_seconds{channel}`、`ack_latency_seconds{severity}`、`queue_depth{queue}`、`broker_connections`，全用 histogram）+ OTel 全发送链路追踪 + slog JSON + `/healthz`+`/readyz`。SLA 按 severity 的 **p99 signed→acked** 定义。
- **签名**：确认 **per-alert 签一次广播**（不是 per-recipient）；扇出成本在 I/O/队列，不在签名；设备侧 verify 缓存 expanded pubkey。

---

## 7. 鉴权 / 身份（两条完全分开的线）

### 7a. 人（Admin/用户）—— 照 Passwall + 多租户化
- 已建：密码 + HS256 JWT（access/refresh）+ `TokenVersion` 吊销 + bcrypt。
- 加：**每租户 SSO**（OIDC `coreos/go-oidc` + SAML `crewjam/saml`，单 ACS/redirect_uri 按 state/RelayState 路由到该租户连接；租户解析=vanity URL / 邮箱域 HRD(需 DNS TXT 验证) / IdP-initiated）+ **JIT + IdP 组→角色映射（每次登录重算）**；passkey（`go-webauthn`）+ TOTP（`pquerna/otp`）+ 恢复码。
- **SCIM 2.0**（`elimity-com/scim`，别手写 PATCH/filter）：`/scim/v2` 每租户 bearer token（存 hash、可多 token 轮换、token 推断租户=隔离边界）；`active:false`/DELETE → 软停用 + `BumpTokenVersion` + 踢 session（不硬删，保审计）；存 `externalId`。**这是大客户的 table-stakes（离职即时下线）**。

### 7b. 机器（CAP 源 / webhook / 集成）
- **Scoped API key**（实现为 `ahk_<rand>`——无租户段，org 由数据库行决定；存 SHA-256 hash，CAP ingest 默认）+ **OAuth2 client-credentials**（短时 JWT，企业策略要求）。最小权限 scope（`alerts:ingest` 仅此）；每租户 + 每 token 限速（防一个坏客户端 DoS 扇出）；≥2 活跃 secret 轮换；secret 只显示一次。

### 7c. 设备舰队（AWS IoT 模式，扩到万级）
- **provisioning-by-claim**：批量 claim 凭据（**每批共享、非全舰队**）→ 设备连上跑 **fleet provisioning 模板** → 发**每设备唯一凭据** + 钉死该 org 公钥 + 端点。
- 身份 = `(org_id, site_id, device_id)`（设备属站点、站点属租户 → "大阪办公室所有接收端"可定向）。
- **mTLS 证书（硬化物理端：壁挂屏/警号）vs 每设备 token（浏览器/PWA/Android 默认）双轨**，按 `identity_type` 选；token 吊销用 TokenVersion 式 bump，证书用 CRL。

---

## 8. 安全 / 合规（SOC2 / ISO 买家会查）

- **审计日志**：append-only `audit_event`，覆盖所有 auth/SCIM/token/角色/设备/发布事件；**哈希链防篡改**（复用我们 Ed25519 的直觉）；可导出到客户 SIEM（syslog/JSON webhook）。
- **会话**：加 **idle 超时**（默认 15min，可配）；logout-everywhere（已有 `BumpTokenVersion`）接到 SCIM 停用/改密/管理员强踢；并发会话可见 + 可撤。
- **密钥**：IdP secret/SAML key/SCIM/API token **静态加密**（AES-GCM + KEK from env/KMS）；所有 bearer/API/设备 token **只存 hash**；JWT 签名密钥支持 **kid 轮换**。**Ed25519 私钥是皇冠明珠**——一等公民密钥 + 文档化轮换（经签名的 key-rotation alert / JWKS 式端点让客户端滚动）。
- **自托管 OSS 定位**：我们不必自己拿 SOC2（自托管者才是数据处理者），但要提供"让自托管者通过自己审计的技术控制"，文档明示对应 SOC2 CC6/CC7。

---

## 9. 通道与来源
- **MQTT**（自有客户端：Android/Tauri/壁挂/web，富+即时+可 ack）+ **ntfy**（独立兜底）+ **push/SMS/voice/email**（outbox worker，企业 table-stakes 四通道；**目前只实现 webhook + email**）+ Teams/Slack（现代 table-stakes）。
- **来源**：**CAP 1.2 ingest API**（对接 HA/JMA 衍生/任意应急工具）+ 日本 EEW 双源（SPEC-SAFETY §6）+ 手动场景模板 + webhook。
- 物理端点（桌面接管/IP 广播/数字标牌/panic button）= 差异化（edu/医疗/政府垂直），分阶段。

---

## 10. 开源产品化

- **许可证：实际采用 AGPL-3.0 + 商业双许可**（见 `LICENSE` / `NOTICE`；Grafana/Mattermost 模式：社区版 AGPL，内部政策禁用 AGPL 的组织走商业授权）。这**推翻了本节原先的 Apache-2.0 建议**，代价照实记录：AGPL 会被部分大厂直接排除在采购之外，商业授权通道就是为此留的逃生口。仍然不要 FSL/BSL（丢"开源"标签，与"可自查供应链"的信任叙事冲突）。贡献用 **DCO + CLA**（CLA 授予维护者提供上述双许可的权利）。
- **将来商业化 = open-core**：核心永久 AGPL，企业功能放单独 `/ee` 目录另立商业许可（GitLab/Mattermost/Authentik 模式）；**永不 relicense 核心**。
- **反 sso.tax 立场**（安全产品的关键）：**SSO/MFA/基础 RBAC/基础审计/全部告警路径永远免费**；只对**规模/分析/联邦/审计留存导出/企业集成/支持**收费。这与"fail-loud、补充官方渠道"的价值观一致，也是对 Everbridge/xMatters 的营销锐角。
- **打包**：多架构 distroless 镜像（GHCR）+ 单二进制（GitHub Releases，签名+SBOM）+ docker-compose（小型）+ **Helm chart**（企业；注意 MQTT TCP/WS 需要 TCP Service/LB 或 Ingress passthrough）+ Kustomize（禁 Helm 的安全部门）。`golang-migrate` 启动时 leader-lock 下迁移；备份/恢复命令；**air-gapped bundle**（国防/电力/医疗常需）。
- **供应链信任信号**（买家会查）：CI 跑 `golangci-lint`/`govulncheck`/`gosec`/Trivy/CodeQL/Dependabot + **SBOM(Syft)** + **Cosign keyless 签名** + **SLSA provenance** + OpenSSF Scorecard 徽章。`SECURITY.md`（私密披露 + SLA）、`CONTRIBUTING/CODE_OF_CONDUCT/GOVERNANCE`、文档站（含**威胁模型** + CAP API 规范 + provisioning 指南 + SLO 文档）。

仓库布局：
```
cmd/alerthub/  internal/{broker,alert,store,auth,scim,delivery,...}  internal/ee/(Phase2)
migrations/  web-admin/  web-client/  clients/  deploy/{compose,helm,kustomize}  docs/  api/(OpenAPI+CAP)
```

---

## 11. 企业功能基线 · 差距对照

**Table-stakes（没有就进不了企业评估）**：多租户+RBAC、联系人/群组+**动态属性群组**、**SCIM+SSO**、push/SMS/voice/email 四通道、双向 ack+问责、升级、模板/场景、定时、多语言、分析、**审计+留存**、REST API+webhook、**SOC2 控制**。geo/站点定向在物理安全垂直是 table-stakes。

> 下表以**代码实况**为准（2026-08，逐条经构建/测试核对），不是计划表。
> ✅ 已实现（有测试覆盖）｜🟡 部分（能用，但有明确缺口）｜🔴 未实现。
> **一句话现状：控制面（org / 成员 / RBAC / 告警数据）已多租户，设备与广播面仍单租户。**

| 能力 | 状态 | 实况 |
|---|---|---|
| 协议核心（签名/接受门/MQTT/撤销）| ✅ | Ed25519 over 锁定的 13 字段规范形式；接受门 = 签名 + ±120s 时钟窗 + `id`+`nonce` 去重 + TTL；签名两步撤销 + 5s TTL 清扫；broker ACL。跨语言一致性测试（Go 签 → `web/verify.js` 验，篡改必拒）|
| FAIL-LOUD A 层 | ✅ | `system/heartbeat` 每 10s 签名心跳（域标签 `hb1`）+ 浏览器看门狗（ok → 30s degraded → 60s offline）+ 时钟漂移横幅 |
| 多租户 + RBAC | ✅ | `orgs` + `memberships`；`alerts` / `service_accounts` 带 `org_id`；9 项权限（`alert:publish/cancel/read`、`device:read/provision`、`sa:manage`、`member:manage`、`org:manage`、`settings:manage`）**按请求从 membership 解析、不入 JWT**（`requirePerm` 中间件）；`is_superadmin` 与静态 admin token 绕过。**sqlite 与 postgres 两档都有**（同一 `Store` 代码路径）|
| Postgres RLS | ✅ **已上线** | `Store.BeginOrg` 开事务并 `SET LOCAL app.current_org`（`set_config(...,true)`），`api.Server.inOrg` 包住每次 alerts 读写；策略 `org_isolation ON alerts` 的 `USING` 读墙与 `WITH CHECK` 写墙均在**真 Postgres + 最小权限角色**下验证（`TestPostgres_RLS_LiveViaBeginOrg` / `_WriteCheck` / `PublishHistoryE2E`）。GUC 未设 → NULL → 零行（fail-closed）|
| 存储可插拔 | ✅ | 一套 `Store` 接口跑 sqlite（modernc，Starter 档）与 PostgreSQL（pgx，Enterprise 档）；方言差异（RLS、`FOR UPDATE SKIP LOCKED`、占位符）在实现内部消化 |
| 耐久投递（outbox）| ✅ 机制 / 🟡 通道 | 单事务写 `alerts` + `delivery_jobs`（**自研 transactional outbox**）→ worker 领取；at-least-once、按 `(alert, channel, target)` 幂等、指数退避、租约式崩溃恢复、死信；Postgres 用 `FOR UPDATE SKIP LOCKED`；Dashboard 展示 sent/pending/dead + 近期死信。**通道只有 webhook + SMTP** |
| 人的鉴权 | ✅ | JWT access(2h)/refresh(7d) + 每用户 `TokenVersion` 吊销 + bcrypt；passkey/WebAuthn（usernameless）；TOTP 2FA + 一次性恢复码（AES-256-GCM 静态加密）；OIDC（PKCE + nonce + JIT）；SAML 2.0（IdP-initiated 默认关闭）；凭据端点每 IP 限速 10/min → 429 + Retry-After；OIDC state 常量时间比较 |
| 机器鉴权 | ✅ | scoped API key（`ahk_` 前缀，只存 SHA-256 hash）+ `requireScope`；CAP ingest 用 `alerts:ingest`，最小权限 |
| 独立兜底通道 | ✅ | ntfy：自托管实例（全等级）+ 公共 ntfy.sh（仅 critical/emergency，无 PII）|
| HTTP 传输 | ✅ | gzip 压缩（实测 ~3.1×）+ 非对称缓存策略：`/admin/assets/*` immutable（内容哈希），告警客户端钉死 `no-cache`（它**不带哈希**，浏览器启发式新鲜度可能喂出过期的生命安全客户端），所有 JSON `no-store` |
| Admin 控制台 | ✅ | React 19 + Ant Design 6 + Vite 8，M3 动态取色（6 预设 / 明暗/跟随），zh-en i18n，`go:embed` 进单二进制 |
| CI | ✅ | `.github/workflows/ci.yml` 四关卡：sqlite 门（gofmt+vet+build+test）、Postgres service 跑 RLS（带防静默跳过 grep）、Node 22 上的密封签名一致性、SPA typecheck+build |
| CAP 1.2 | 🟡 | ingest 与 **Cancel** 都已实现——ingest 用确定性 id `cap.AlertID(sender, identifier)`，Cancel 的 `<references>` 重算出同一 id → `CancelByID`，**无需映射表**。**缺 Update-as-supersede、缺 `<area>`/geocode 定向** |
| SSO | 🟡 **单租户** | OIDC 与 SAML 各**一份全局配置**（来自环境变量），整套安装共用一个 IdP。§7a 设想的**每租户 SSO**（按 state/RelayState 路由到租户连接、vanity URL / 邮箱域 HRD、IdP 组→角色映射）**未实现**；JIT 有 |
| 来源 | 🟡 | 日本 EEW 经 Wolfx WebSocket（按 EventID 去重），默认关闭（`ALERTHUB_EEW=wolfx`）。**单源**——SPEC-SAFETY §6.1 要求的第二源（P2PQuake）与 renew/serial 升级未做 |
| 可观测 | 🟡 | Prometheus 计数器（alerts_published / cap_ingest / auth_logins / heartbeats / delivery_enqueued / delivery_attempts）+ `/healthz` + `/readyz`。**全是 counter**——§6 要求的 histogram（扇出 / 投递 / ack 延迟）与 OTel 全链路追踪都没有 |
| 打包 / 供应链 | 🟡 | 有上面的 CI 与 `make ci` / `make fmt-check`。**无 Dockerfile / compose / Helm / Kustomize，无 SBOM(Syft) / Cosign / SLSA / govulncheck / Scorecard** |
| 设备 / 广播面 | 🔴 **仍单租户** | `device` 表与 provisioning 未建，设备**没有 `org_id`**；`/api/devices` 返回**全局**在线名册（进程内 map）；MQTT `alerts/active`、`alerts/events` 是**全局主题**。控制面多租户 ≠ 设备面多租户——**这是通向真多租户隔离的头号缺口** |
| 定向（群组 / 站点 / geo）| 🔴 | §2 的 `site/zone/group/user_profile/alert_recipient` 实体与 §4 的 send-time 解析全部未建；当前发布 = 全局广播 |
| ack + 升级 | 🔴 | 无 T1/T2/T3 升级状态机、无每设备升级主题、无名册视图、无周演练 cron |
| push / SMS / voice | 🔴 | outbox 只有 webhook 与 email 两个 sender；四通道 table-stakes 还差 push / SMS / voice 三个 |
| 自定义角色 | 🔴 | 只有固定基础角色（owner/org_admin/admin/dispatcher/operator/viewer/user）；§3 的 `role` / `role_permission` 权限袋未建 |
| SCIM 2.0 | 🔴 | 无 `internal/scim`；"离职即时下线"目前只能手工 |
| 审计日志 | 🟡 | **已实现**：`audit_log` 表（两方言）+ **SHA-256 哈希链**（`hash = SHA256(prev_hash‖canonical(entry))`，改行/删行均可检出并定位到具体条目）+ append-only（无 update/delete 方法）+ `GET /api/audit`（perm `settings:manage`，按 org 过滤）+ `GET /api/audit/verify`（**仅超管**，链是全局的）。已记录 publish/cancel/登录成败/服务账号创建/组织创建。**SIEM 导出已实现**：`internal/siem` 按**持久化游标**至少一次外送（采集端故障时游标不前进，恢复后补齐；条目携带 `prev_hash`/`hash` 供采集端独立验链），`ALERTHUB_SIEM_URL` 配置。覆盖面已扩到 2FA 启停、passkey 增删、服务账号吊销、SSO 登录。**仍缺**：留存与归档策略、角色变更与设备事件、syslog 格式 |
| 规模 / HA | 🔴 | 无 EMQX 可插拔 `Bus`、无 advisory-lock leader 选举；仍是内嵌 mochi 单进程 |
| 外部看门狗（off-cluster）| ✅ | `internal/watchdog`：**反向** dead-man's-switch（本机健康时才打点，沉默即告警，无需入站路径）。健康 → `<url>`；自检降级 → `<url>/fail`；进程死亡 → 静默 → 对端超时告警。健康判据与签名心跳同源，A/B 两层不会互相矛盾。**需配置 `ALERTHUB_WATCHDOG_URL`**（第三方 check），未配置启动 WARN |
| 原生端 | 🔴 | Android / Tauri 桌面 / iOS 均未开始；Admin SPA 的 `/devices`、`/history`、`/sources` 仍是 "Coming soon" 占位 |

两条刻意的设计取舍，记在这里免得被当成 bug：
- **`service_accounts` 不进 RLS**：API-key 鉴权必须先按 `token_hash` 查到 key **才知道 org**，先有鸡还是先有蛋——org 作用域策略会让鉴权把自己锁死。它的租户隔离靠 create/list/delete 三处的应用层 `org_id` 过滤（有测试）。因此目前 RLS 只覆盖 `alerts` 一张表。
- **outbox 是自研的，不是 River**：River 只支持 Postgres，而 Starter 档的产品承诺是**单二进制 + sqlite**。为让两档跑同一份投递语义，队列落成 `Store` 之上的一张 `delivery_jobs` 表（见 §6）。

---

## 12. 修订路线图（企业优先级）

| 阶段 | 内容 | 状态（2026-08） |
|---|---|---|
| **E0 多租户地基** | `org_id` 全表 + `org/membership/group` + 默认 org 迁移 + **Store 接口(sqlite+postgres)** + RBAC 权限模型 + `RequirePerm` + 审计日志骨架 | 🟡 **大部完成**：org/membership/RBAC/`requirePerm`、双方言 `Store`、Postgres RLS 已上线。**未做**：审计日志骨架、`site/zone/group/user_profile` 等实体表、自定义角色。偏离设计一处：JWT 只带 `uid/upn/role/tv`，**活动 org 不在 token 里**，由 `X-Org-Id` 头 > API key 的 org > 默认 org 解析 |
| **E1 Admin 鉴权补全** | passkey → TOTP+恢复码 → **per-tenant SSO + JIT + 组角色映射** | 🟡 passkey、TOTP+恢复码、OIDC（PKCE+nonce+JIT）、SAML 2.0 全部已建，凭据端点已限速。**但 SSO 仍是单租户**（一份全局 IdP 配置），IdP 组→角色映射未做 |
| **E2 SCIM + 服务账号** | SCIM 2.0 + scoped API key / OAuth2-CC + CAP ingest API | 🟡 scoped API key（hash 存储 + scope）与 CAP 1.2 ingest/Cancel 已建。**SCIM 2.0 与 OAuth2 client-credentials 未做** |
| **E3 投递流水线 + 定向** | outbox + 每通道 worker（push/SMS/email/voice）+ 动态群组 + 站点/geo 定向 + 确认送达升级 + 名册 | 🟡 **自研 outbox + worker 已建**（幂等、退避、租约恢复、死信），通道 = webhook + SMTP。**push/SMS/voice、动态群组、站点/geo 定向、ack 升级、名册全部未做** |
| **E4 规模 + 可观测 + 打包** | EMQX 可插拔 Bus + advisory-lock leader + Prometheus/OTel + Helm + CI 供应链(SBOM/Cosign/SLSA) | 🟡 **刚起步**：Prometheus 计数器 + `/healthz`/`/readyz` + 四关卡 CI。**EMQX Bus、leader 选举、histogram/OTel、Helm/compose、SBOM/Cosign/SLSA 都没有** |
| **E5 原生端 + 物理端点** | Tauri 桌面接管 / Android FSI / DIY beacon | 🔴 未开始 |

**下一步（按"欠得最贵的债先还"排）**：
1. **设备面多租户**——给 `device` 加 `org_id` + provisioning，并把 `alerts/active` / `alerts/events` 拆成按 org 的主题。这是唯一还在打脸"多租户"承诺的结构性缺口，而且已上线设备越多、迁移越贵。
2. **审计日志**（E0 遗留）——SOC2/ISO 评估的硬门槛；append-only + 哈希链的性质决定了它**不能事后补写历史**，越晚做覆盖面越残缺。
3. ~~**外部看门狗**~~——已完成（`internal/watchdog` 反向打点 + 心跳讲真话）。P0 现在是**代码完成**，剩下的是使用者在第三方建 check 并配置 `ALERTHUB_WATCHDOG_URL` 这一步部署动作。
4. **每租户 SSO**（E1 收尾）+ **SCIM**（E2）——大客户 table-stakes，且两者都要改 §7a 的单 ACS/redirect 路由假设，宜一起做。

---

## 13. 诚实的取舍
- 双 SQL 方言是持续成本，逃生口 = Postgres-only。
- RLS 性能依赖 `org_id` 首列索引——用 schema lint 强制，别靠自觉。
- sqlite 档无 RLS/geo 定向——文档化缺口，不假装。
- 每 org 签名密钥（强隔离）需 SPEC v2 的 `kid` 纳入签名（SPEC §8 已预留）。
- mTLS 全设备诱人但 CRL/OCSP 分发是万级规模的运维税——默认 token、证书可选。
- 永不为"开源"标签牺牲信任：核心永久开源（AGPL-3.0），绝不 relicense 为闭源；商业授权只是并行通道。

---
*本文件随实现迭代；架构级改动先改这里。研究全文存档于会话 transcript。*
