# RFC 0001 — 设备置备与设备面多租户

| | |
|---|---|
| 状态 | **草案 · 待评审**（未实现） |
| 影响 | **SPEC.md §5（MQTT 拓扑）、§6（Broker 鉴权与 ACL）—— 均为 LOCKED** |
| 作者 | AlertHub 维护者 |
| 日期 | 2026-08-14 |

> 本文按项目规矩存在：`SPEC.md` 是锁定契约，**改动先改规范再改代码**。本 RFC 就是那份「先改」。
> 未获批准前不得实现。

## 1. 问题

控制面已经是多租户的（发布 / 历史 / 服务账号按 `org_id` 隔离，Postgres 档还有 RLS 兜底）。**设备与广播面不是**：

- `alerts/active`、`alerts/events` 是**全局 topic**。A 组织发布的紧急警报会广播到 B 组织的每一台设备。
- `GET /api/devices` 返回**全局**在线名册。
- 设备**没有组织归属**。broker 只有一个共享的 `client` 账号（`ALERTHUB_CLIENT_PW`），所有设备共用同一口令。
- 因此**吊销单台设备是做不到的**：改口令会踢掉所有设备。

对一个宣称多租户的告警平台，这是最大的架构不一致；对一个人身安全系统，「无法吊销一台丢失的设备」也是实打实的风险。

## 2. 非目标

- 不改告警信封与签名规范（`SPEC.md §2/§3` 不动）。**协议核心保持不变**。
- 不引入外部 broker（EMQX 属于另一条线）。
- 不做设备端 mTLS（列为后续可选加固档）。

## 3. 提案

### 3.1 每设备身份（借鉴 AWS IoT / Azure IoT 的 provisioning-by-claim）

新增 `devices` 表：`id, org_id, device_id, name, cred_hash, disabled, created_at, last_seen`。

置备流程：

1. 管理员在控制台创建**置备令牌**（claim token，短时效、可限次、绑定 org）。
2. 设备携带 claim token 调 `POST /api/devices/provision`。
3. 服务端签发**每设备唯一凭据**（即该设备的 MQTT 口令），**只显示一次**，并写入 `devices`（存 hash）。
4. 设备此后用 `device_id` 作 MQTT username、该凭据作 password 连接。

吊销 = 把该行 `disabled=1`，只影响这一台。

### 3.2 每租户 topic（**这是 §5 的改动**）

现状 → 提案：

| 现状（全局） | 提案（按 org） |
|---|---|
| `alerts/active` | `org/<orgID>/alerts/active` |
| `alerts/events` | `org/<orgID>/alerts/events` |
| `alerts/<id>/ack/<deviceId>` | `org/<orgID>/alerts/<id>/ack/<deviceId>` |
| `status/<deviceId>` | `org/<orgID>/status/<deviceId>` |
| `system/heartbeat` | `system/heartbeat`（**保持全局**，见下） |

**心跳保持全局**：它是 fail-loud 通道，不含租户数据，且必须在「租户解析都失败了」的情况下依然可达。

### 3.3 ACL（**这是 §6 的改动**）

`OnConnectAuthenticate` 改为按 `devices` 表查 `device_id` + 凭据 hash，并读出该设备的 `org_id`。`OnACLCheck` 相应变为：

```
订阅：org/<自己的orgID>/alerts/#, org/<自己的orgID>/status/#, system/#
发布：org/<自己的orgID>/alerts/+/ack/<自己的deviceId>
      org/<自己的orgID>/status/<自己的deviceId>
```

即在现有「设备只能写自己的 ack 与在线状态」之上，**再加一层组织前缀约束** —— 跨租户读写在 broker 层即被拒。

## 4. 兼容性与迁移

**这是破坏性的线路协议改动**：现有浏览器客户端订阅的是 `alerts/*`，改动后收不到任何告警。

迁移方案（二选一，评审时定）：

- **A. 双写过渡期** —— 服务端同时发到旧的全局 topic 和新的 `org/<id>/…`，客户端升级后移除旧路径。风险：过渡期内跨租户广播依然存在（隔离没真正生效）。
- **B. 一刀切 + 版本门** —— `/pubkey` 返回 `topic_scheme: 2`，旧客户端识别到自己不支持即**高调报错**而非静默失效。符合 fail-loud 原则，但要求同步升级所有设备。

**倾向 B**：过渡期内保留跨租户广播，等于让一个安全修复在最需要的时候不生效；而 fail-loud 的项目原则要求「宁可大声坏掉，不可静默失效」。

## 5. 影响面

- `SPEC.md §5/§6` 需修订并重新锁定（本 RFC 批准后）。
- `broker.authHook`：改为查库鉴权（当前是常量比较），需要缓存以免每次 CONNECT 都打库。
- `web/app.js`：订阅路径需带 org 前缀；`/pubkey` 需下发本设备的 org 与凭据。
- `api.handleDevices`：改为按 org 过滤（现为全局）。
- 新增置备 UI（Devices 视图加「置备新设备」）。

## 6. 待评审确认的问题

1. 迁移选 A 还是 B？（本文倾向 B）
2. claim token 的时效与次数上限？
3. 单台设备是否允许属于多个 org？（倾向否 —— 一台设备一个归属，简化 ACL）
4. 是否同期做 mTLS 档，还是留作后续？

---
*本 RFC 未获批准前，`SPEC.md §5/§6` 与相关代码保持现状。当前设备面仍是单租户，此事实已在 SPEC.md §0、SPEC-SAFETY.md §10.2、README 与 Devices 视图内明确标注。*
