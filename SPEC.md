# AlertHub — v1 协议规范（LOCKED）

> 本文件是 AlertHub 的**权威协议规范**。代码以此为准；任何改动先改这里再改代码。
> 配套：原始设计讨论见 [docs/DESIGN.md](docs/DESIGN.md)；评审记录见本文件末尾「决策裁决」。

---

## 0. 定位（先把话说清楚）

**本文件只规定线路协议（wire protocol）**：消息信封、规范化与签名、客户端接受门、MQTT 拓扑，以及承载它们的**告警面** HTTP 端点（§7）。控制面/管理后台的完整 HTTP 接口见 [docs/API.md](docs/API.md)；多租户、RBAC、SSO、持久化投递等企业侧架构见 [ARCHITECTURE.md](ARCHITECTURE.md)。

**协议核心已达生产标准。** 下列机制均已实现并锁定：Ed25519 覆盖 13 字段规范化形式、四步接受门（验签 / ±120s 时钟窗 / `id`+`nonce` 去重 / TTL）、retained 快照 + 事件流拓扑、已签名的两步撤回、TTL 自愈清扫、broker ACL。跨语言逐字节一致性由自动化 conformance 测试守护（Go 签名 → 浏览器 `web/verify.js` 验签；篡改任一字段必须被拒）。

**但 §2–§9 并非全部落地。** 本文件是**目标规范**，以下条款目前仍是规范而非现状：

| 条款 | 规范要求 | 现状 |
|---|---|---|
| §5.2 | `emergency` 由服务端**续期**（同 `id` + 新 `issued_at`/`ttl` 重发）| ❌ 未实现。清扫器只会清空过期的 active 槽，不重发。 |
| §5.3 | 面板订阅 `alerts/+/ack/#` 构建「谁已确认」名册 | ❌ 未实现。客户端**会写** ack（`web/app.js`），但服务端只订阅 `status/#`，无人收集 ack。 |
| §8 | 客户端接受**有序公钥列表**以支持轮换 | ❌ 未实现。客户端只持单一公钥；轮换需停机换钥。 |
| §9 | Arm 手势一并申请 `Notification.requestPermission()` | ❌ 未实现。只解锁了音频。 |

**但仍有诚实的边界，别 over-claim：**
- **没有原生客户端**（Android / 桌面 / iOS 均未开始，见 [SPEC-SAFETY.md §10](SPEC-SAFETY.md) 的 P5/P6/P7）→ 无法保证唤醒睡眠或锁屏的设备，也无法盖在其它应用之上。浏览器客户端只在「人在机器前、页面开着」时有效。
- **设备/广播面仍是单租户**：`alerts/active`、`alerts/events` 是全局 topic，设备没有 org 归属。多租户目前只覆盖控制面（发布、历史、服务账号）。
- **单 broker 单点**：无集群、无 leader election。
- **缺少外部 dead-man switch**：服务端自检已闭环（心跳携带签名的 `health`，存储/broker 故障即自报降级），但服务器**整体**宕机时没有第二方替它喊——看门狗不能与被看者共命运。

**结论：真正的人身安全仍以官方渠道为第一路径**（緊急地震速報、运营商 cell broadcast 等）。AlertHub 的增量价值在于：覆盖收不到 cell broadcast 的桌面场景，以及把组织内的系统告警与物理告警统一到一条已签名、可撤回、可审计的链路上——它是官方渠道的补充，不是替代。

---

## 1. 两个正交维度

| 维度 | 取值（锁定枚举） | 决定 |
|---|---|---|
| `severity` | `notice` / `warning` / `critical` / `emergency` | 呈现强度 + 颜色 |
| `category` | `earthquake` / `fire` / `weather` / `system` / `security` / `custom` | 图标/标签 + 建议处置 `action` |

颜色由 `severity` 决定（红/橙配白字，琥珀配黑字）；图标与默认 `action` 由 `category` 决定。

---

## 2. 消息信封（wire envelope）

线上是 JSON（便于调试），但**签名只覆盖 §3 的规范化字节，不覆盖 JSON 序列化本身**。

```jsonc
{
  "schema_version": 1,
  "id": "01J9Z3M8XK...",          // 服务端生成，全局唯一、时间有序（ULID/UUIDv7）
  "type": "alert",                // "alert" | "cancel"
  "category": "earthquake",
  "severity": "emergency",
  "title": "正在发生地震",
  "body": "震中距你约 42 公里，预计 15 秒后到达。",
  "action": "趴下，掩护，抓牢",     // 建议处置（可由 category 自动填充）
  "source": "panel",              // 逻辑发布者身份（来自鉴权）
  "issued_at": 1765238400,        // 服务端时钟，unix 秒，权威
  "ttl": 60,                      // 有效期，秒。过期 = issued_at + ttl < now
  "nonce": "9f86d081884c7d659a2feaa0c55ad015", // 16 随机字节的小写 hex（32 字符）
  "cancels": "",                  // type=="cancel" 时为被撤回的原 id；否则空串
  "sig": "Base64Url(64-byte Ed25519 签名，无填充)"
}
```

> **`type` 与 `cancels` 必须被签名**（见 §3 字段表）——否则攻击者可伪造一条「撤回」来压掉真实警报，或篡改 alert/cancel 类型。这是评审 gap #1 的「可信」一半。

---

## 3. 规范化与签名（LOCKED）

### 3.1 算法：Ed25519（非对称，不可改为 HMAC）

- 服务端持**私钥**签名；客户端只持 **32 字节公钥**验签。
- 理由（对应威胁模型）：**收警报的设备本身被视为可能被攻陷**。HMAC 是对称的——能验签即能伪造，任一客户端被攻破即可伪造「地震/火灾」。Ed25519 把能力拆开：客户端只能验、永不能签。
- Ed25519 额外优点：确定性签名（无逐次 RNG，跨语言不会因随机源不一致出错）、32B 公钥 / 64B 签名、Go 标准库 + 所有现代浏览器 + JVM 原生支持。

### 3.2 规范化：固定字段拼接（**不签 JSON**）

JSON 不规范（键序、空白、数字格式、unicode 转义都会变）。我们签一个**确定性的字段拼接串**。

**规则（所有语言必须逐字节一致）：**
1. **字段顺序固定（13 个，v1）：**
   ```
   schema_version │ id │ type │ category │ severity │ title │ body │ action │ source │ issued_at │ ttl │ nonce │ cancels
   ```
2. **分隔符**：单个换行字节 `\n`（`0x0A`）。13 字段 = 12 个分隔符，**无结尾换行**。
3. **编码**：UTF-8，对每个**字符串**字段先做 **Unicode NFC** 归一化再拼接（Go `golang.org/x/text/unicode/norm`；JS `String.prototype.normalize("NFC")`；Kotlin `Normalizer`）。保证中日文/emoji 字节一致。
4. **整数字段**（`schema_version`、`issued_at`、`ttl`）：base-10 ASCII，无符号、无前导零、无小数点。`issued_at`/`ttl` 单位**秒**。
5. **`nonce`**：16 随机字节的**小写 hex**（32 字符）。用 hex 而非 base64，避开分隔符/`+`/`/` 歧义。
6. **空字段**仍占位（产生其分隔符）。`body`/`action`/`cancels` 可为空串。
7. **换行安全**：`title`/`body`/`action`/`source` 不得含原始 `\n` 或 `\r`——服务端在签名前必须拒绝（HTTP 400），以免破坏字段分帧。

规范化字节：
```
"1" ‖\n‖ id ‖\n‖ type ‖\n‖ category ‖\n‖ severity ‖\n‖ title ‖\n‖ body ‖\n‖ action ‖\n‖ source ‖\n‖ issued_at ‖\n‖ ttl ‖\n‖ nonce ‖\n‖ cancels
```
Ed25519 内部已做 SHA-512，直接对这些字节签名，**不预哈希**。

### 3.3 签名编码

`sig` = `base64url(签名)`，**无填充**（RFC 4648 §5，`-`/`_`，无 `=`）。
Go `base64.RawURLEncoding`；JS 自带 helper；Kotlin `getUrlEncoder().withoutPadding()`。

### 3.4 库

- **Go**：`crypto/ed25519`（标准库）。
- **浏览器**：优先 WebCrypto `crypto.subtle.verify({name:"Ed25519"}, ...)`（Safari 17 / Firefox 129 / Chrome 137 起原生），feature-detect 失败回退 [`@noble/ed25519`](https://www.npmjs.com/package/@noble/ed25519) v2+ 的 `verifyAsync(sig, msg, pubKey)`。
- **Kotlin（路线图）**：BouncyCastle（兼容旧 API level）。

---

## 4. 客户端接受门（replay / dedup / TTL）

签名过后，**所有以下检查必须通过**，否则丢弃：

```
1. sig 对内嵌/拉取的公钥验签通过        → 否则 DROP(bad-sig)
2. |now - issued_at| <= MAX_SKEW         → 否则 DROP(stale / future-dated)
3. id 未见过 AND nonce 未见过            → 否则 DROP(dup / replay)
4. now - issued_at <= ttl                → 否则 DROP(expired)  [也驱动自动消失]
   然后: 把 (id, nonce) 写入 seen-set
```

- **MAX_SKEW = 120 秒。** 够紧（抓包重放窗口短），够松（容忍 LAN 时钟漂移）。EEW 场景下警报远在 120s 内；超过 2 分钟的警报本就无操作意义。
- **`id`** 处理 QoS1 重复 + retained 重连重发（gap #6/#7）。
- **`nonce`** 防「重放一条被抓的合法包」。
- **seen-set 持久化 + 淘汰**：浏览器用 `Map` 镜像到 `localStorage`，每次接受后清除 `now - issued_at > MAX_SKEW` 的条目——因第 2 步已拒绝更老的，seen-set 天然有界（≈ 警报速率 × 120s）。
- **时钟依赖**：第 2/4 步依赖收发双方时钟在 MAX_SKEW 内一致 → **所有主机必须跑 NTP**。客户端时钟错 >120s 会静默拒收**有效**警报（危险）→ 客户端已据已签名心跳的 `issued_at` 计算偏移，连续 3 拍漂移 >30s 时弹出时钟告警条（SPEC-SAFETY §3.2）。

---

## 5. MQTT 拓扑（LOCKED）

| Topic | retain | QoS | 用途 |
|---|---|---|---|
| `alerts/active` | **是** | 1 | **当前生效的最高优先警报快照**。空 payload = 无生效警报。离线设备重连即补收。 |
| `alerts/events` | 否 | 1 | 每条 alert + 每条 cancel 的实时有序流。在线客户端据此动作；服务端据此存历史。 |
| `alerts/<id>/ack/<deviceId>` | 是 | 1 | 确认记录（每设备每警报一条）。 |
| `status/<deviceId>` | 是 | 1 | 设备在线状态（LWT + birth）。 |

**核心约束**：retained 每个 topic 只留**最后一条**。整套设计由此而来：
- `alerts/active`（retained 单槽）→ 重连补收「现在在发生什么」。
- `alerts/events`（不 retained）→ 历史事件永不在重连时重放，只有在线客户端实时收。

**`alerts/active` 替换策略**（服务端拥有此槽）：新警报覆盖槽**仅当**其 severity ≥ 当前生效警报，或当前已过期/已撤回。低 severity 仍进 `events`（在线可见），但不顶掉更高 severity 的快照。

### 5.1 撤回 / 解除（cancel）——服务端两步原子动作

1. 向 `alerts/events` 发一条**已签名**的 cancel 信封（`retain=false`）：`type:"cancel"`, `cancels:"<原id>"`。在线客户端立即据此动作。
2. 若被撤回的正是 `alerts/active` 中那条：向 `alerts/active` 发**空 payload + retain=true**（这会删除 broker 的 retained 消息）→ 后续重连客户端不再补收已撤回的警报（杀死 gap #6）。若仍有其它生效警报，则改为把那条**重新 retained 发到 active**，而非清空。

**客户端收到合法 cancel 时**：按 `cancels` 的 id 在本地 active 集合里查 → 命中则拆除其全屏、停其警报音、清其强制确认；未命中（自己没见过原警报）则记入 cancelled-set，使原警报若稍后乱序到达被抑制。空 retained payload 到 `alerts/active` 同样是「无生效」信号。

### 5.2 过期自愈

服务端跑 TTL 清扫器：active 警报 TTL 到期未续期 → 清空 `alerts/active`（空 retained）。需更久存活的 `emergency` 由服务端**续期**（用同一 `id` + 新 `issued_at`/`ttl` 重发；客户端按 id dedup，不会重复弹，只延长有效期）。

### 5.3 确认（forced ack）

`critical`/`emergency` 渲染强制确认按钮。点击后客户端向 `alerts/<id>/ack/<deviceId>`（retain=true）发 `{alert_id, device_id, ack_at, by?}`，**然后**才拆除全屏/停音。`deviceId` 放在 **topic** 里，配合 ACL 使设备只能写自己的 ack，无法伪造他人确认。面板订阅 `alerts/+/ack/#` 构建「谁已确认」名册。

### 5.4 在线状态（presence）

每设备一 retained topic `status/<deviceId>`：连接后发 birth（`state:"online"`）；CONNECT 时注册 LWT（`state:"offline"`，retained）；干净断开时客户端自己先发 offline。面板订阅 `status/#` 即得全设备在线态。

---

## 6. Broker 鉴权与 ACL

> MVP 用**内嵌 Go broker（mochi-mqtt）**，协议与 Mosquitto 兼容；上 homelab 改连真 Mosquitto 即可（同一套 ACL 语义）。

- 关闭匿名。每个连接带用户名/密码。
- 角色：
  - **`publisher`**（= Go 服务端，唯一签名者）：可写 `alerts/#`、`status/#`；可读 acks。**原始发布者（EEW/监控脚本/面板）不直接连 broker**，而是调 Go 服务端的 HTTP API，由服务端签名后发布——签名私钥永不出现在 broker 写路径。
  - **`client`**（所有收警报设备）：可读 `alerts/active`、`alerts/events`、`status/#`；只能写 `alerts/+/ack/<自己的deviceId>`、`status/<自己的deviceId>`。**永远不能写 alert 频道** → 被攻陷的接收端在 broker 层就注入不了假警报（在 Ed25519 之上的纵深防御，gap #3）。

Mosquitto ACL 等价写法（pattern `%u`=用户名）：
```conf
user publisher
topic readwrite alerts/#
topic readwrite status/#

pattern read  alerts/active
pattern read  alerts/events
pattern read  status/#
pattern write alerts/+/ack/%u
pattern write status/%u
```

---

## 7. 服务端 HTTP API

> 本节只列**告警面**端点——即直接参与「发布→签名→broker→客户端」这条链路的那些。
> 认证/2FA/passkey/OIDC/SAML/组织/服务账号/健康检查等控制面接口，连同本节这些告警面端点，**全部 38 条路由**见 [docs/API.md](docs/API.md)。

| 方法 路径 | 鉴权 | 作用 |
|---|---|---|
| `POST /api/publish` | 权限 `alert:publish` | 校验输入 → 生成 id/nonce/issued_at → 签名 → 发 `events`(QoS1) + 按 §5 策略 retained 发 `active` → 落库并入投递队列。`source` 固定为 `panel`。返回完整信封。 |
| `POST /api/cancel` | 权限 `alert:cancel` | body `{id}` → 构造已签名 cancel → 发 events + 清空/替换 active（§5.1）。返回 cancel 信封。 |
| `GET /api/history` | 权限 `alert:read` | 当前活动 org 最近 50 条已签名信封（Postgres 下经 RLS 事务读取）。 |
| `GET /api/devices` | 权限 `device:read` | 在线设备名册（服务端订阅 `status/#` 得到的 presence）。**注意：该名册目前是全局的，尚未按 org 隔离**（见 §0）。 |
| `GET /api/delivery/stats` | 权限 `alert:read` | 投递队列状态计数 + 最近 20 条死信。 |
| `POST /api/cap` | 服务账号 scope `alerts:ingest` | CAP 1.2 XML 摄入。`msgType=Cancel` 走撤回路径：按 `<references>` 的每个 `sender,identifier` 重算确定性 id（`cap-` + SHA-256(sender‖NUL‖identifier) 前 12 字节 hex），再走与 `/api/cancel` 相同的已签名撤回。`source` 固定为 `cap`。 |
| `GET /pubkey` | 公开 | 信任锚 + 浏览器引导：`{pubkey, schema_version, max_skew, ws_port, mqtt_user, mqtt_pw}`。**MVP 客户端启动时拉取**（本地 loopback 可信）。**生产**应改为**构建期内嵌**公钥作为信任锚——见 §8。 |
| `GET /`、静态 | 公开 | 提供 `web/`（面板 + 客户端）。 |

**调用者有三类：**
1. **人（会话）**：`Authorization: Bearer <access JWT>`，或 `ah_access` cookie。权限由**调用者在活动 org 中的成员角色**解析（不是用户的全局 role）；不是成员或角色无该权限 → 403。
2. **机器（服务账号）**：`Authorization: Bearer ahk_…`，按 token 的 SHA-256 查库并校验 scope。目前只有 `/api/cap` 走这条路（scope `alerts:ingest`）；非 `ahk_` 前缀的 token 会回退到管理员会话鉴权。
3. **静态管理令牌** `ALERTHUB_ADMIN_TOKEN`：给脚本/自动化用，**绕过 RBAC**（等同 super）。默认值是 `dev-admin-token`，**生产必须改掉**。

**活动 org 解析顺序**：`X-Org-Id` 请求头 > API key 所属 org > 默认 org。带 `X-Org-Id` 时依然要求调用者是该 org 成员（super-admin 与管理令牌除外），因此它不是越权通道。权限名与角色→权限映射见 docs/API.md。

**错误码**：400 输入非法 / CAP XML 不合法；401 未认证；403 权限或 scope 不足；502 broker 发布失败。

输入校验：拒绝 `title/body/action/source` 含 `\n`/`\r`；severity/category 必须在枚举内；ttl 缺省按 severity 给（如 emergency 120s、critical 120s、warning/notice 600s）。

---

## 8. 架构与密钥管理

```
发布来源(面板/脚本/EEW) ──HTTP+token──▶ Go 服务端 ──(内嵌)──▶ mochi broker ──MQTT/WS──▶ 浏览器客户端
                                         │  验权(RBAC)→加签名(Ed25519)→publish→落库(SQLite/PostgreSQL)
                                         └ 单二进制：broker + API + 静态 web + 内嵌管理 SPA，一条命令起
```

- **密钥生成**：首次启动若无私钥则 `ed25519.GenerateKey` 生成，私钥写 `keys/alerthub_ed25519.key`（`0600`，gitignored），公钥写 `keys/alerthub_ed25519.pub`。
- **公钥分发**：
  - **MVP**：客户端启动 `GET /pubkey` 拉取（loopback 可信；威胁模型针对的是「内网设备经 MQTT 注入」，它们既不能签也无法 MITM 本机 HTTP）。
  - **生产（客户端在别的设备/跨网）**：**构建期把公钥内嵌进客户端**（如 Vite `VITE_ALERTHUB_PUBKEY`），这是真正的信任锚——否则 MITM 可同时换公钥并用配对私钥签名。
- **轮换**：客户端接受一个有序公钥列表（任一通过即可），新键重叠期发布，下个客户端版本再去掉旧键。需快速吊销时给信封加 `kid` 并纳入规范化（→ schema v2）。

---

## 9. 客户端行为

- **Arm 手势（一次/会话）**：浏览器的全屏与音频都需用户手势解锁。提供「启用警报」按钮，点击时一并：`audioContext.resume()` 解锁音频、（看板模式可选）`requestFullscreen()` 预存全屏、`Notification.requestPermission()`。未 arm 时**显式**提示「🔔 点击启用警报声」，避免「神秘静音」。
- **渲染**：`position:fixed; inset:0` 全视口覆盖层，背景纯色 = severity。三件事：大标题（出什么事）+ 纯色（多严重）+ 一句 `action`（该干什么）+ 确认按钮。
- **音频**：`emergency` 循环报警音直到确认。
- **deviceId**：随机生成存 `localStorage`。

---

## 10. MVP 能证明什么 / 不能证明什么（诚实边界）

| 能力 | 浏览器客户端（**已实现**）| Tauri 包壳（**未开始**，见 SPEC-SAFETY §10 P6）|
|---|---|---|
| 打通 发布→签名→broker→渲染→确认 全链路 | ✅ | ✅ |
| 全视口纯色覆盖层 + 循环报警音 + 强制确认 | ✅（覆盖当前标签页） | ✅ |
| 真·OS 全屏（无手势自动进入） | ❌（Fullscreen API 需用户手势） | ✅（原生 `setFullscreen` 无需手势） |
| 盖在**其它应用**之上（强出现） | ❌（浏览器做不到） | ✅（always-on-top + NSWindow level/collectionBehavior；fullscreen Space 用 `ActivationPolicy::Accessory` 兜底） |
| 睡眠/锁屏的 Mac 收到 | ❌ | ❌（结构性做不到） |

> ⚠️ 右栏是 Tauri 包壳**若实现**能达到的能力，**目前一行代码都没有**。今天唯一存在的桌面通道是左栏的浏览器客户端。

→ 桌面是「人在机器前时的尽力提醒」通道；**手机（FSI + 高优先级推送唤醒锁屏）才是真正的人身安全通道**（Android 14+ `USE_FULL_SCREEN_INTENT` 对侧载的闹钟类 app 仍可用，运行时查 `canUseFullScreenIntent()`）。

---

## 11. 决策裁决（评审 → 锁定）

| 评审发现的 gap | 裁决 |
|---|---|
| #1 无撤回/解除 | §5.1 已签名 cancel + 清空 retained active；`type`/`cancels` 纳入签名。 |
| #2 签名不防重放 | §4 `issued_at`+`nonce`+`id` 三层门，MAX_SKEW=120s。 |
| #3 HMAC 在威胁模型下错 | §3.1 锁定 Ed25519（非对称），私钥仅服务端。 |
| #4 签名范围未定义 | §3.2 固定 13 字段 `\n` 拼接 + NFC，不签 JSON。 |
| #5 schema 缺字段 | 加 `schema_version`/`id`/`type`/`source`/`issued_at`/`ttl`/`nonce`/`cancels`。 |
| #6 retained 反复触发 | §4 第 2 步按 MAX_SKEW 拒旧 + §5.1 撤回清空 + §5.2 TTL 自愈。 |
| #7 QoS1 重复 | §4 第 3 步按 id+nonce dedup。 |
| 子分歧：拼接 vs JCS | 选**字段拼接**（跨语言更易逐字节一致；JCS 需各语言 canonical-JSON 库，脆弱）。 |
| 子分歧：秒 vs 毫秒 | 选**秒**（贴合原文档 `ttl`、人类可读；Go/JS 都有明确取整语义）。 |
| 命名/目录拼写 `Aleart` | 已知拼写错误（应为 `Alert`）；仓库已按 `github.com/KazuhaHub/AleartHub` 发布，拼写沿用。Go module path 仍是 `github.com/kazuha/alerthub`，与仓库地址不一致——不影响构建运行，但 `go install` 用不了。 |

---
*本规范随实现迭代；改动先改本文件。*
