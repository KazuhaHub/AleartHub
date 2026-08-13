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
- **投递流水线**：单事务写 `alerts` + **outbox**（Postgres）→ **River**（Postgres 任务队列）每通道 worker（push/sms/voice/email/mqtt/ntfy），at-least-once + 幂等键 + 每通道限速/退避。超 ~1万事件/s 才把内部跳转移到 NATS JetStream。
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
- **Scoped API key**（`ahk_<tenant>_<rand>`，存 hash，CAP ingest 默认）+ **OAuth2 client-credentials**（短时 JWT，企业策略要求）。最小权限 scope（`alerts:ingest` 仅此）；每租户 + 每 token 限速（防一个坏客户端 DoS 扇出）；≥2 活跃 secret 轮换；secret 只显示一次。

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
- **MQTT**（自有客户端：Android/Tauri/壁挂/web，富+即时+可 ack）+ **ntfy**（独立兜底）+ **push/SMS/voice/email**（River worker，企业 table-stakes 四通道）+ Teams/Slack（现代 table-stakes）。
- **来源**：**CAP 1.2 ingest API**（对接 HA/JMA 衍生/任意应急工具）+ 日本 EEW 双源（SPEC-SAFETY §6）+ 手动场景模板 + webhook。
- 物理端点（桌面接管/IP 广播/数字标牌/panic button）= 差异化（edu/医疗/政府垂直），分阶段。

---

## 10. 开源产品化

- **许可证：Apache-2.0**（含专利授权，适合密码/安全产品；MIT 不提专利）。**不要 AGPL**（Google 等大厂明令禁用，正好赶走你的买家）、不要 FSL/BSL（丢"开源"标签，与"可自查供应链"的信任叙事冲突）。贡献用 **DCO**（必要时轻量 CLA 保留 relicense 余地）。
- **将来商业化 = open-core**：核心永久 Apache，企业功能放单独 `/ee` 目录另立商业许可（GitLab/Mattermost/Authentik 模式）；**永不 relicense 核心**。
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

| 状态 | 项 |
|---|---|
| ✅ 已有 | MQTT(retained active/presence/ack 一体)、Ed25519 签名、FAIL-LOUD 心跳、ntfy 兜底、CAP-as-API(设计)、设备 provisioning(设计)、Admin 密码/JWT 鉴权地基、React 后台壳+登录、场景模板雏形 |
| 🟡 计划 | passkey/TOTP、per-tenant SSO、EEW 双源、确认送达升级、呈现策略、Tauri/Android 原生端 |
| 🔴 缺(table-stakes) | **多租户/org_id**、**RBAC 权限模型**、**SCIM**、**动态群组+geo/站点定向**、**push/SMS/voice/email worker**、**审计日志**、**Postgres+sqlc 可插拔**、**EMQX 可插拔**、可观测、Helm/供应链 |

---

## 12. 修订路线图（企业优先级）

| 阶段 | 内容 | 价值 |
|---|---|---|
| **E0 多租户地基** | `org_id` 全表 + `org/membership/group` + 默认 org 迁移 + **Store 接口(sqlc, sqlite+postgres)** + RBAC 权限模型 + JWT 加 `tid` + `RequirePerm` + 审计日志骨架 | 一切的前提；不先做后期最贵 |
| **E1 Admin 鉴权补全** | passkey → TOTP+恢复码 → **per-tenant SSO + JIT + 组角色映射** | 解锁首批企业试点 |
| **E2 SCIM + 服务账号** | SCIM 2.0（elimity-com/scim）+ scoped API key / OAuth2-CC + CAP ingest API | 解锁大客户（离职下线）+ 集成 |
| **E3 投递流水线 + 定向** | outbox + River worker（push/SMS/email/voice）+ 动态群组 + 站点/geo 定向 + 确认送达升级 + 名册 | 真正的 MNS 能力 |
| **E4 规模 + 可观测 + 打包** | EMQX 可插拔 Bus + advisory-lock leader + Prometheus/OTel + Helm + CI 供应链(SBOM/Cosign/SLSA) | 企业可运维 |
| **E5 原生端 + 物理端点** | Tauri 桌面接管 / Android FSI / DIY beacon | 物理安全垂直差异化 |

> E0 直接改动已建的鉴权地基（加 `tid`/membership/RequirePerm）和 store（接口化）。这是下一步。

---

## 13. 诚实的取舍
- 双 SQL 方言是持续成本，逃生口 = Postgres-only。
- RLS 性能依赖 `org_id` 首列索引——用 schema lint 强制，别靠自觉。
- sqlite 档无 RLS/geo 定向——文档化缺口，不假装。
- 每 org 签名密钥（强隔离）需 SPEC v2 的 `kid` 纳入签名（SPEC §8 已预留）。
- mTLS 全设备诱人但 CRL/OCSP 分发是万级规模的运维税——默认 token、证书可选。
- 永不为"开源"标签牺牲信任：核心永久 Apache，绝不 relicense。

---
*本文件随实现迭代；架构级改动先改这里。研究全文存档于会话 transcript。*
