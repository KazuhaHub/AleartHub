# AlertHub — 人身安全级硬化规范（SPEC-SAFETY）

> 本文件在 [SPEC.md](SPEC.md)（v1 协议）之上，定义把 AlertHub 提升到**人身安全级**所需的冗余、fail-loud、确认送达/升级、真实来源对接与跨平台唤醒。
> 区域：**日本**。研究依据：5 路硬化深挖 + Alertus/CAP（2026-06 联网核实）。
>
> **文档状态约定**：§1–§9 是**目标规范**（要建成什么），不是现状。**当前实现进度以 §10 为准**；未落地的小节在标题下用一行 `> 状态：` 标注。§11 汇总必须告知用户的现实限制。

---

## 0. 定位与诚实的天花板（先读）

**AlertHub 补充官方渠道，永不取代。** 地震/火灾的"原始预警"，最可靠的是运营商 **緊急地震速報（cell broadcast）** 和**认证联动烟感/CO 报警器**——它们不依赖你的网/电/服务器。

AlertHub 的人身安全增量价值：
- 覆盖**收不到 cell broadcast 的设备**（桌面、Wi-Fi 壁挂屏）；
- **聚合**多源（EEW + 火灾 + homelab + 手动）到统一界面与协议；
- **确认送达 + 升级**（谁收到、谁确认、没确认就升级）；
- **fail-loud**——它坏了会大声自曝，而非静默。

**能达到 / 不能达到**（平台能力上限，**不是**当前实现状态——实现进度见 §10）：
| | 状态 |
|---|---|
| Android（侧载/壁挂屏/桌面） | ✅ 可做到锁屏唤醒、绕勿扰、强制确认、本地 fail-loud |
| iPhone | ⚠️ **不能保证唤醒静音手机**（Critical Alerts entitlement 对免费/侧载不可得）；只能 Time-Sensitive。当作次要屏，EEW 靠 cell broadcast |
| 睡眠/关机/无网设备 | ❌ 任何软件都做不到 |

> 铁律（贯穿全文）：**任何路径都不许静默失效——沉默必须在某处被转成大声的信号。**

---

## 1. 架构：冗余扇出 + fail-loud + 确认送达

```
来源                          ┌─ MQTT(签名) ─→ 自定义客户端：Android app / Tauri 桌面 / 壁挂屏 / web
EEW(Wolfx+P2PQuake) ─┐        │                （强呈现：全屏接管 + 报警音 + 强制确认 + ack 回传）
火灾/CO(HA) ─────────┤        ├─ ntfy(自托管) ─→ ntfy app（现成独立接收端，触达手机）
CAP API(他程序CALL) ─┼→ Go ──┤
手动场景模板 ────────┘  服务端 └─ ntfy.sh(公共) ─→ critical/emergency 镜像（自托管挂了仍送；通用文案、无 PII）
                        │
                        ├─【fail-loud①】每 10s 发签名心跳 → 客户端本地看门狗，断了就本地报警"系统离线"
                        ├─【fail-loud②】每 60s ping 外部看门狗(healthchecks.io) → 漏 ping=我家全黑 → 经 ntfy.sh+邮件报警
                        └─【确认送达】emergency 未 ack → 按 per-device 阶梯升级（T1 重发/提级 → T2 最响通道+邮件）
```

**两个独立观察者**，因为它们看的是两种不同故障：
| 故障 | 谁能看见 | 层 |
|---|---|---|
| 服务端活着，但**我这台设备的链路**断了 | 设备自己（收不到心跳） | **A 层（客户端本地）** |
| **整个家**断电/断网/服务器死 | **家以外**的东西（家不再 check-in） | **B 层（外部看门狗）** |

---

## 2. 通道 × 设备矩阵（已核实）

> 状态：本矩阵是**目标**。当前实际可用的接收端只有两个：**web 客户端**（浏览器，最后一行）与现成的 **ntfy app**；Android / 壁挂 / 桌面 / iOS 的自有客户端（P5/P6/P7）均未开工，其“能本地大声”一列是平台能力上限，不是现状。

每条警报**并行**扇出到所有通道（互不等待）。两大传输族：**MQTT**（自有、富、即时、可回 ack）+ **ntfy**（独立、单向、家挂了仍触达手机）。

| 设备 | 主通道 | 备通道 | 能本地大声(绕静音)? | 能回 ack? | 备注 |
|---|---|---|---|---|---|
| **Android 手机**(侧载) | MQTT 自定义 app | ntfy app | **✅** USAGE_ALARM+FSI+bypassDnd+wakelock | ✅ | **人身安全主力** |
| **壁挂平板**(专用) | MQTT 自定义/kiosk | ntfy(可选) | **✅** 音量钉死、常亮 | ✅ | **家庭主报警器**，硬接电源 |
| **桌面 Mac/Win**(Tauri) | MQTT(WS/TCP) | ntfy 桌面 | 部分(OS 勿扰可能压声) | ✅ | **唯一填补"桌面收不到 cell broadcast"的设备** |
| **iPhone**(受限) | ntfy iOS(Time-Sensitive) | —(或自建 app+Critical Alerts) | **❌**(除非走 §9 付费路线) | 仅自建 app | 次要屏；EEW 靠 cell broadcast |
| **web** | MQTT over WS | — | ❌(浏览器) | ✅ | 面板/名册视图 |

**severity → 通道强度：**
| severity | MQTT 客户端 | ntfy priority | 升级阶梯 |
|---|---|---|---|
| notice | toast | 2 | 否 |
| warning | toast | 3 | 否 |
| critical | 全屏+报警音+强制确认 | 5 + tags | **是** |
| emergency | 同上，不确认不消失、更响 | 5 + `siren` tag | **是，更快** |

---

## 3. FAIL-LOUD（最重要的人身安全属性）

### 3.1 A 层：客户端本地看门狗 + 签名心跳

> 状态：✅ 已实现。服务端 10s 签名心跳（`hb2` 域标签）+ web 客户端本地定时器分级（OK / DEGRADED ~30s / OFFLINE ~60s）+ 恢复需连续 ≥2 拍的抖动抑制，均与本节一致。心跳现在还携带**服务端对自己的健康判定**（见下），跨语言一致性由 conformance 守护（含「篡改 `health` 必须验签失败」）。

服务端每 **10s** 向 `system/heartbeat`（retained, QoS1）发**签名**心跳（签名防 LAN 上被攻陷设备伪造"一切健康"来压掉离线报警）。

心跳信封（域分隔标签 `hb2`，与 alert 的 canonical 隔离，防签名跨用）：
```json
{ "schema_version":1, "type":"heartbeat", "seq":184213, "issued_at":1749859200, "interval":10, "active_count":0, "health":"ok", "sig":"base64url(ed25519)" }
```
canonical（Go 与 verify.js 必须逐字节一致）：`"hb2" \n seq \n issued_at \n interval \n active_count \n health`

**`health`：服务端对自身的判定**（`ok` | `degraded`）。在它出现之前心跳是**恒绿**的——存储挂掉的服务端照样告诉每个客户端「一切正常」，这恰恰是 fail-loud 要防的失效模式。它**被签名覆盖**，所以被攻陷的 LAN 设备无法把 `degraded` 改回 `ok` 来消音（有专门的测试守这条）。
- 判定依据：`Store.Ping()` 失败 → `degraded`；上一次 broker 发布失败 → `degraded`。
- **降级不停拍**：继续发「我降级了」比沉默信息量更大，而沉默本来就由客户端看门狗兜底。
- 客户端把它渲染成**独立于连接状态**的琥珀横幅（链路正常但服务端自称有病），连接真正断掉时仍由看门狗的 OFFLINE 接管。
- 域标签由 `hb1` 升到 `hb2`：签名字段集变了，旧 5 字段与新 6 字段**必须不能交叉验签**。

> ⚠️ **自检的诚实边界**：`Ping()` 探的是「连接是否可用」。Postgres 档它是真往返（已用停库演练验证：心跳转 `degraded`、`/readyz` 转 503、恢复后自动转回）。**SQLite 档它探不到「数据库文件被删」**——POSIX 语义下已打开的文件句柄依然有效，`Ping()` 照常成功。

**客户端本地定时器**（不靠收消息驱动——这是关键，半开 socket 不会触发 onMessage）按距上次有效心跳的时长分级：
| 距上次心跳 | 状态 | 行为 |
|---|---|---|
| < 3×interval (~30s) | OK | 绿点，无声 |
| 3–6×interval (30–60s) | DEGRADED | 琥珀点 + 小横幅"连接中断，重连中…"，无声 |
| > 6×interval (~60s) | **OFFLINE — 大声** | `⚠ 告警系统离线 — 收不到警报了`，持续横幅 + 渐强提示音；壁挂/桌面接管 |
- **抖动抑制**：恢复需连续 ≥2 个心跳健康才清除 OFFLINE。
- OFFLINE 报警**本地生成**，不依赖任何收到的消息（定义如此）。
- 平台现实：Android（前台服务）/壁挂/桌面 A 层稳；**iOS 后台跑不了看门狗 → iPhone 的 fail-loud 靠 B 层**。

### 3.2 时钟漂移自检（静默杀手）

> 状态：✅ 已实现。`|drift|>30s` 连续 ≥3 拍触发持久横幅。
客户端拒收 `|now−issued_at|>120s` 的警报 → 时钟错 >120s 的设备**静默拒收所有真实警报**。用每条签名心跳的 `issued_at` 当可信时间基准：`drift=local_now−hb.issued_at`，连续 ≥3 个心跳 `|drift|>30s` → **大声持久报警**"时钟偏差过大，可能收不到真实警报，请开启自动对时"。（心跳签名正是为防此处被武器化。）

### 3.3 B 层：外部看门狗（检测"整个家黑了"）

> 状态：✅ **服务端侧已实现**（`internal/watchdog`，`ALERTHUB_WATCHDOG_URL` 开启）。机制是**反向**的：不要求外部服务能入站访问家里，而是本机**健康时才打点**，沉默即告警。健康 → GET `<url>`；自检降级 → GET `<url>/fail`（healthchecks.io 约定，让对端立即告警而非等宽限期）；进程/主机/上行死亡 → 完全无打点 → 对端超时告警。已用本地假采集端做过三段演练（健康打点 → 停 Postgres 转 `/fail` → 杀进程后彻底静默）。
>
> ⚠️ **剩下的是部署动作,不是代码**：必须由使用者在 healthchecks.io / Cloudflare Worker 上建一个 check 并把 URL 配进来。**未配置时服务启动会 WARN**——因为「以为有外部看门狗其实没有」比「知道自己没有」更危险。看门狗不能与被看者共命运，所以这个对端必须在别处。
**反相逻辑**：家死了发不出消息 → 看门狗**在"该来的 ping 没来"时**触发（dead-man's switch）。
- **主**：[healthchecks.io](https://healthchecks.io)（免费层 20 checks）。家服务端每 **60s** ping `hc-ping.com/<uuid>`；period=1min, grace=5min；Down 时 fan-out ntfy(自托管+ntfy.sh) + 邮件。服务端 ping **条件于内部健康**（broker/DB 不健康则主动打 `/fail`）。
- **备**（不同厂商，去单点）：Cloudflare Worker cron 每 1min 拉取 `/healthz`，失败 POST ntfy.sh。
- **防狼来了**：grace 5min；Down(>5min)=prio5+邮件，Recovered(Up)=prio3 给闭环。

### 3.4 演练（在出事前证明它还活着）

> 状态：❌ 未实现。没有演练 cron、收集窗与名册对照。（CAP 侧的 `status=Exercise/Test/System → 强制降为演练` 已实现，见 §7，可手动灌一条演练警报。）
服务端 cron goroutine，**每周日 10:00 JST**：发一条 `category=system, severity=warning, drill=true` 的**真**警报（走完整签名+渲染+accept-gate+ack 路径），等 10min 收集窗，对照 `status/#` 设备名册：
- 全 ack → 面板/壁挂记"上次演练 PASS N/N"。
- 有设备漏 ack → **只通知管理员**（非全家）：ntfy prio4 + 邮件"设备 X 未确认月度测试，可能在真实警报时不会响"。SQLite 存历史看劣化。演练时离线的设备，下次 birth 时补测。
- 每季度一次 `critical` 演练，验证全屏接管 + 强制确认路径。

---

## 4. ntfy 独立通道

> 状态：✅ 已实现。自托管发所有等级，ntfy.sh 仅镜像 critical/emergency，Go 侧走 JSON body。

发布 = HTTP POST `https://<host>/<topic>`，正文 = message，其余走 header（或 JSON body，**Go 侧用 JSON body**，避开日文+逗号在 header 里的转义坑）。priority `5`=max。

| severity | ntfy priority | tags |
|---|---|---|
| notice/warning | 2 / 3 | `information_source` |
| critical | 5 | `warning,<category>` |
| emergency | 5 | `rotating_light,siren,<category>` |

**自托管主 + ntfy.sh 兜底**：自托管发**所有**级别（私密、富、LAN 快）；ntfy.sh 仅镜像 **critical/emergency**，**文案通用、零 PII**（不放住址/姓名/AlertHub URL/token/HA 实体名——ntfy.sh topic 公开可读可伪发）。每台设备在 ntfy app 里同时订阅自托管 topic + ntfy.sh topic，家挂了 ntfy.sh 仍送（接受偶发重复响）。

**唤醒能力（诚实）：**
- **Android**：prio5 + 用户一次性设置（长按 ntfy → Max/Urgent 频道 → 开声音/震动 + **覆盖勿扰** + 开 Instant delivery 前台服务 + 免电池优化）= 真能绕勿扰、唤醒。但只是 heads-up，**非全屏接管**（接管在自定义 app）。
- **iOS**：**无唤醒保证**。ntfy iOS 无 Critical Alerts entitlement；静音/Focus 会压掉；iOS 26.2+ 还有无声 bug。且**自托管 ntfy 在 iOS 上必须经 ntfy.sh 当 APNs 代理**（`upstream-base-url: https://ntfy.sh`）→ iOS"自托管"**并不独立**。iPhone 的 ntfy.sh 直发反而比自托管更可靠（都过 APNs，少一跳）。
- **桌面**：仅在 app/PWA 运行时；睡眠/锁屏收不到。

**信任边界**：ntfy.sh 消息**无签名、可伪发、第三方可见** → 只做"去看 AlertHub / 听官方"的人类可读提示，**绝不触发自动动作**。可信锚永远是 MQTT 上的 Ed25519 签名路径。

**UnifiedPush**：自定义 Android app 已有前台服务 MQTT（即 Google-free 常驻推送），无需再上 UnifiedPush；直接装 ntfy app 当第二独立接收端即可"白嫖"该能力。

---

## 5. 确认送达 + 升级阶梯

> 状态：🟡 **服务端半环已实现**。ack 现由服务端订阅并持久化（§5.3），`critical`/`emergency` 发布后进入 **T1/T2/T3 per-alert 状态机**：仍有「在线但未确认」设备时逐级升级——经 ntfy 独立通道以递增优先级重发「仍未确认」，T3 标记 `UNREACHABLE` 并在日志与 `GET /api/alerts/escalations` 中给出待查设备名单。确认、撤回、TTL 到期均会终止阶梯；离线设备不阻塞阶梯（那是投递问题，已由备份通道覆盖）。
>
> **仍缺（需 schema v2）**：`reissued_at`/`escalation_phase`/`requires_ack` 三个**签名字段**与 accept-gate 的「再次拉响」改动——它们改变规范化形式，属破坏性协议变更；因此目前**客户端不会因升级而重新拉响**，升级只走服务端与 ntfy。`alerts/<id>/escalation/<deviceId>` topic 亦未落地（状态改由 HTTP 暴露）。

对 `critical`/`emergency` 跑 **per-device 升级状态机**（"目标设备"=配置的必须 ack 名单，如每人手机 + 壁挂屏）。一台 ack 不停止对另一台未 ack 的升级。

| 阶段 | 触发 | 动作 | critical | emergency |
|---|---|---|---|---|
| t0 | 发布 | 全通道扇出，启动 per-device 计时 | 0s | 0s |
| T1 | 仍未 ack | 该设备 MQTT 重发 + ntfy prio5 重发"仍未确认" + 壁挂/Android 重响 | 30s | 15s |
| T2 | 仍未 ack | 最响通道：ntfy(siren) + **外部看门狗→邮件**（你选的兜底，无 Pushover/Twilio）| 90s | 45s |
| T3 | 仍未 ack | 标 `UNREACHABLE`，ntfy 退避重发直到 ack/TTL/cancel；面板高亮 | 3min | 2min |

**新增 topic / 字段：**
| topic | retained | 用途 |
|---|---|---|
| `system/heartbeat` | 是 | §3.1 fail-loud 心跳（签名）|
| `alerts/<id>/escalation/<deviceId>` | 是 | 升级状态 `{phase,lastSentAt,channels[]}`（签名，驱动名册、供面板重连恢复）|

| 新字段(纳入签名) | 用途 |
|---|---|
| `reissued_at` | T1/T2/T3 重发时服务端重签；客户端 freshness 用 `max(issued_at, reissued_at)`，让合法重发过 120s 窗，而 `id`+`nonce` 仍保身份/去重 |
| `escalation_phase` | 让客户端知道这是未 ack 警报的重响，应**再次拉响**而非静默去重 |
| `requires_ack` | 显式标记强制确认（critical/emergency），客户端据此把"消除"门控在"已发 ack" |

**accept-gate 一处改动**：原 `验签→freshness→去重(id,nonce)→丢过期`。对强制确认警报，去重步改为：*若 `id` 已见 AND 本地尚未 ack AND `escalation_phase` 前进 → **再次拉响**，而非丢弃*；纯重复（同 phase、已 ack）仍丢。

**名册（面板可操作视图）**：交叉 `status/#`(在线) × `alerts/<id>/ack/#`(已确认) × 配置(目标)：①在线+已确认=绿 ②**在线但未确认=红闪（行动项：打电话/去人那）** ③离线=琥珀（已升级到 ntfy+邮件，去现场看）。

> **确认的是"设备 ack"，非"人知晓"**——emergency 要求明确的屏上"我安全/已知晓"手势，且"在线未确认"始终需人工核实。

---

## 6. 真实来源

### 6.1 日本 EEW（地震预警）

> 状态：🟡 部分实现——**仅 Wolfx**（`ALERTHUB_EEW=wolfx`，默认关闭），按 `EventID` 去重、同一事件只发一次。**P2PQuake 第二源未接 → 本节的双源冗余要求未满足**；`Serial` renew 与严重度升级（予報→警報 / 震度提级）也未实现。
**主 Wolfx WS `wss://ws-api.wolfx.jp/jma_eew`**（含 予報+警報、稳定 `EventID`+`Serial`、~60s 心跳、无强制断连）；**备 P2PQuake WS `wss://api.p2pquake.net/v2/ws`**（code 556，仅 警報；注意：**每 IP 2 连接上限**、**~10min 强制断连属正常勿报警**）。两路同跑，按 `EventID` 互相去重（先到先建警报）。

**EEW → severity 映射**（取 警报/预报标志 与 预测最大震度 中更严者）：
| EEW 条件 | severity |
|---|---|
| 予報，震度≤3 | warning(toast) |
| 予報，震度4 | critical |
| 警報，震度5弱〜5強 | critical |
| 警報/特別，震度6弱+ | **emergency** |
| cancel 报 | 发 cancel |

`category=earthquake`，`source=eew:wolfx|eew:p2pquake`，`id="eew-"+EventID`（后续 Serial 报 **renew 同一 id**，不重新拉响，除非 severity 升级如 予報→警報/震度4→6弱）。S 波到达**绝对 unix 时间**塞进 action，客户端本地 tick"あと N 秒"。**延迟预算**：收帧→映射→去重→签名→**先 publish(MQTT+ntfy)→后异步落库**（落库绝不挡 publish）。
> 诚实：比 cell broadcast 慢、是免费"無保証"第三方中继、大震时最易被打爆——正因如此才要双源 + fail-loud + ntfy 独立路径。手机务必保持**緊急速報メール 开启**。授权/合规升级路径：**DMDATA.JP（Project DM-D.S.S）** 付费一手数据（需 予報業務許可）。

### 6.2 火灾/烟雾/CO

> 状态：🟡 AlertHub 侧的 CAP 入口已就绪（§7）；Home Assistant 侧为用户自行配置。
经 Home Assistant 桥接烟感/CO → 调 AlertHub CAP API（§7）。诚实：**认证联动烟感本身才是火灾人身安全主力**，AlertHub 作放大。

### 6.3 手动场景模板（Alertus 借鉴）

> 状态：❌ 未实现。
预置场景一键发（撤离 / 就地避险 / 锁闭 / 趴下掩护抓牢 / 警报解除），每个映射到 CAP `responseType` + 目标组 + 通道集。面板/app/桌面一键。

---

## 7. 对外 API = CAP 1.2（最关键的互通决策）

> 状态：🟡 部分实现。入向 `POST /api/cap`（API key，scope `alerts:ingest`）已支持 `msgType=Alert` 与 **`Cancel`**——ingest 用确定性 id `cap.AlertID(sender,identifier)`，Cancel 的 `<references>` 重算出同一 id 后走 `CancelByID`，因此不需要映射表；三元折叠与 `status` 演练门也已实现。**未实现**：`msgType=Update` 的 supersede 语义、下表最后一行的 `<area>`/geocode 定向（当前所有 CAP 警报都是全员）、以及出向 emit CAP。

"其他程序可以 CALL"的 API **接收 CAP 1.2**（对齐 Alertus 的 Inbound CAP），一个解析器即打通 HA + JMA 衍生源 + 任意应急工具。CAP 是 OASIS 标准，结构 `alert → info → area`。

**CAP → AlertHub 映射：**
| AlertHub | CAP 来源 | 规则 |
|---|---|---|
| category | `info.category` | Fire→fire, Met→weather, Geo→earthquake, Security/Safety→security, Health/Infra→system, Env→weather；未知→custom |
| severity | **由 `severity`+`urgency`+`certainty` 三元导出**（见下）| 不要直接 copy |
| title | `info.headline`(退 `event`) | |
| body | `info.description`(+`instruction`) | |
| action | `info.responseType` | Shelter→就地避险, Evacuate→撤离, AllClear→解除——**最干净的映射** |
| issued_at | ~~`alert.sent`(或 `onset`)~~ | ⚠️ **未按规范实现**：当前取服务端 `time.Now()`；`sent`/`onset` 已解析但未使用 |
| ttl | `info.expires − now` | 无则按 **severity** 默认（emergency/critical 120s，其余 600s）——不是按类别 |
| id/dedupe | `alert.identifier`+`sender` | 幂等键；`msgType=Update/Cancel` 据 `references` |
| **drill 门** | `alert.status` | **`Exercise/Test/System` → 演练模式，绝不当真实报警**（安全关键）|
| targeting | `info.area.geocode`(JIS 市町村码)/`areaDesc` | 映射到 房间/人，无匹配=全员 |

**三元 → 单 severity 折叠**（保留原始三元存 `parameters` 以便再发标准 CAP）：
```
Immediate + (Extreme|Severe) + (Observed|Likely) → emergency/critical(fail-loud,持续至确认,全通道)
Immediate + Moderate                             → critical
Expected/Future + any                            → notice
certainty=Unlikely 或 status≠Actual              → 降级/仅演练
```
> EEW 是三元的典型：`urgency=Immediate` 但 `certainty` 早期可能只是 `Likely/Possible`。**urgency 驱动是否 fail-loud/持续至确认，certainty 驱动是否升级**。
> 日本现实：**JMA 不发原生 CAP**（其防災情報 XML 是自有 schema，经 Atom feed）。所以 EEW 用一个 **JMA-XML→CAP 适配器**，对齐 OASIS CAP 1.2 国际标准（而非 TC-only 的实验性日本 profile）。出向也可 emit CAP（title→headline, action→responseType…）。

---

## 8. 各平台客户端（要点；完整配方见研究存档）

> 状态：❌ 未实现（P5/P6）。本节是配方，不是现状；当前唯一的自有接收端是浏览器 web 客户端。

- **Android（主力，侧载，无 FCM）**：前台服务（type `connectedDevice|specialUse`，**无超时**；勿用 `dataSync`=6h 上限）持 MQTT/TLS（keepalive<NAT 空闲窗），`START_STICKY`+`onTaskRemoved` 重起+BootReceiver。唤醒配方：`IMPORTANCE_HIGH` + `CATEGORY_ALARM` + `setBypassDnd`（需通知策略访问授权）+ `USAGE_ALARM` 满音量循环 + `setFullScreenIntent` → 锁屏 Activity `setShowWhenLocked+setTurnScreenOn+ACQUIRE_CAUSES_WAKEUP`，强制 ack 才停。一次性授权：POST_NOTIFICATIONS、免电池优化、勿扰策略访问、全屏 intent 特殊访问、**各 ROM autostart/不限制**（MIUI/ColorOS 最凶，见 dontkillmyapp.com）。首选 Pixel/GrapheneOS 当壁挂主力。
- **iOS（受限）**：见 §9。现实路径=官方 ntfy app（Time-Sensitive），**不保证唤醒静音机**。
- **桌面/壁挂（Tauri）**：A 层 fail-loud 最稳；壁挂应作**家庭主报警器**（常亮、音量钉死、硬接电源）。

---

## 9. iOS 现实（坦白）

> 状态：未启动（P7）。本节陈述的是 Apple 的约束，不随实现进度变化。
- **Critical Alerts**（绕静音/Focus 出声）需 entitlement `com.apple.developer.usernotifications.critical-alerts` → 申请表**要求 Account Holder（付费 Apple Developer Program）登录** → **免费/侧载拿不到，硬墙，无 workaround。**
- 想要 iOS 保证唤醒的唯一现实路径：**付费 $99/yr + 建最小自建 iOS app（验签同其它客户端）+ 向 Apple 申请 entitlement**（理由要具体："住宅 EEW/火灾/CO 人身安全，夜间睡眠中的老人/小孩漏警有直接人身后果，Time-Sensitive 无法唤醒静音机"），批了走 Ad-hoc/TestFlight（不必上架）。**数周、可能反复、不保证**。
- **不要让系统依赖 iOS 批准落地**——把 iOS 设计成 bonus，不是承重腿。

---

## 10. 分阶段建设计划

图例：✅ 已完成 · 🟡 部分完成 · ❌ 未开始

| 阶段 | 内容 | 状态 | 实际缺口 |
|---|---|---|---|
| MVP | 内嵌 broker + Ed25519 签名 + web 客户端/面板 + 全屏 | ✅ 已完成 | 协议核心已过跨语言一致性测试（Go 签名 ↔ `web/verify.js` 验签，篡改必拒）|
| **P0** | fail-loud 心跳 + ntfy 扇出（自托管 + ntfy.sh）| ✅ **代码完成**（需一步部署配置）| **A 层**（§3.1/§3.2）：10s 签名心跳、客户端 30s/60s 分级看门狗、抖动抑制、时钟漂移横幅；ntfy 双通道（§4）已跑通。**服务端自检**：心跳携带签名 `health`，`Store.Ping()` 或上次 broker 发布失败即自报 `degraded`，客户端独立横幅（停 Postgres 演练验证过；SQLite 档探不到「文件被删」，见 §3.1）。**B 层**（§3.3）：`internal/watchdog` 反向 dead-man switch —— 健康打点 / 降级打 `/fail` / 死亡静默，本地假采集端三段演练通过。⚠️ **仍需使用者配置 `ALERTHUB_WATCHDOG_URL`**（在 healthchecks.io 等第三方建 check）；未配置时启动会 WARN。看门狗必须在别处，不能与被看者共命运。 |
| P1 | CAP 1.2 ingest API + 出向 CAP；场景模板 | 🟡 部分完成 | 已做：CAP 1.2 解析、三元（severity/urgency/certainty）折叠、`Exercise/Test/System` 强制降为演练、`responseType→action`、**Cancel**（确定性 id + `<references>` → `CancelByID`，无映射表）。**`msgType=Update` 的 supersede 已实现**（Update 沿用 `<references>` 所指警报的 id 重发 → 客户端就地更新；未升级则静默，升级则重弹，见 SPEC §5.2）。缺：`<area>`/geocode（JIS 市町村码）定向——**当前所有 CAP 警报都是全员**；出向 emit CAP；§6.3 场景模板。|
| P2 | 日本 EEW 双源 WS（Wolfx + P2PQuake）+ renew 去重 | 🟡 **双源已达成，renew 未做** | 已做：**Wolfx + P2PQuake 两个独立中继**（`ALERTHUB_EEW=wolfx,p2pquake`，逗号分隔），**共享一个去重器** → 任一源掉线另一源仍可送达，而两源都报的同一次地震只发一条；两源的震度→severity 映射一致（有测试守）；只配一个源会在启动 WARN。**震度提级已打通**：`Deduper` 不再是纯首报胜出——同一 `EventID` 的**上调**震度会以同一 alert id 经 `RenewAlert` 重发，客户端据 §5.2 重新强呈现；重复与下调仍被抑制（免得训练用户忽略警报）。缺：`Serial` 字段本身尚未用于判定（当前以 severity 提级为准）。|
| P3 | 确认送达升级状态机 + 名册视图 + 每周演练 cron | ❌ 未开始 | §5 与 §3.4 整节仍是设计。客户端发得出 ack、broker ACL 也放行，但服务端不订阅、不统计；T1/T2/T3、`alerts/<id>/escalation/<deviceId>`、`reissued_at`/`escalation_phase`/`requires_ack`、三色名册、演练 cron 均无。|
| P4 | 外部看门狗（healthchecks.io + CF Worker）配置 + 文档 | ❌ 未开始 | 与 P0-B 是同一个洞。`/healthz`、`/readyz` 已就绪（接入点备好），但没有任何外部实体在轮询。|
| P5 | Android 原生 app（前台服务 + FSI + 绕勿扰） | ❌ 未开始 | 需 Android 工具链。|
| P6 | Tauri 桌面 always-on-top 包壳 | ❌ 未开始 | 需 Rust 工具链。|
| P7（可选） | iOS 自建 app + Critical Alerts 申请 | ❌ 未开始 | 需付费 Apple Developer 账号（§9）。|
| **PE** | **企业 / 平台层——不在本路线图上，但已经建成** | ✅ 已完成 | 明细见 §10.1；**边界见 §10.2，很重要**。|

### 10.1 PE 明细（补记：路线图之外实际建成的东西）

| 能力 | 现状 |
|---|---|
| 多租户 + RBAC | 组织 / 成员 / 角色；9 项权限（`alert:publish` / `alert:cancel` / `alert:read`、`device:read` / `device:provision`、`sa:manage`、`member:manage`、`org:manage`、`settings:manage`）；活动 org 解析顺序 = `X-Org-Id` 头 > API key 所属 org > 默认 org |
| 管理台认证 | JWT access(2h)/refresh(7d) + per-user `TokenVersion` 吊销；bcrypt；passkey/WebAuthn（usernameless）；TOTP 2FA + 一次性恢复码（AES-256-GCM 静态加密）；OIDC（PKCE + nonce + JIT）；SAML 2.0（IdP-initiated 默认关闭）；5 个凭据端点 10 次/分钟/IP 限流。**SSO 为单租户**：全局一份 IdP 配置，未实现每租户 SSO |
| 持久化投递 | 事务性 outbox（`delivery_jobs`）、至少一次、按 (alert, channel, target) 幂等、指数退避、租约式崩溃恢复、死信队列；webhook + SMTP；Postgres 用 `FOR UPDATE SKIP LOCKED` |
| 存储 | 同一套代码跑 SQLite（modernc）与 PostgreSQL（pgx）|
| PostgreSQL 行级安全 | **已上线**：`Store.BeginOrg` 开事务并设置事务级 GUC `app.current_org`，`api.Server.inOrg` 包住 alerts 的每次读写；读墙（USING）与写墙（WITH CHECK）已在真 PG + 最小权限角色上验证。`service_accounts` **故意不入 RLS**——API key 认证必须在 org 已知之前按 `token_hash` 查 |
| 可观测性 | Prometheus 计数器 + `/healthz` + `/readyz` |
| HTTP 传输 | gzip（实测 ~3.1x）+ 非对称缓存策略（`/admin/assets/*` immutable；告警客户端钉死 `no-cache`——它不带内容哈希，浏览器的启发式新鲜度可能送出过期的人身安全客户端；JSON 一律 `no-store`）|
| 管理台 | React 19 + Ant Design 6 + Vite 8，M3 动态配色，zh/en，`go:embed` 内嵌 |
| CI | `.github/workflows/ci.yml` 四个 job：SQLite 门禁（gofmt + vet + build + test）、Postgres service + RLS 实跑、Node 22 上的跨语言签名一致性、SPA typecheck + build |

### 10.2 PE 的边界（务必先读再当多租户产品用）

**控制面多租户，设备面单租户。** `/api/devices` 返回的是**全局**在线名册；MQTT `alerts/active`、`alerts/events` 是**全局** topic；设备没有 `org_id`（设备开通尚未建）。因此在多 org 部署下，**任一 org 发布的警报会广播给所有已连接设备**。租户隔离目前只覆盖控制面（alerts 历史、service accounts）。

**同样仍缺**（未列入上表）：SCIM 2.0、审计日志、EMQX 集群总线 / leader 选举；管理台 `/devices`、`/history`、`/sources` 仍是“Coming soon”占位页。

---

## 11. 诚实限制（汇总，必须告知用户）
1. **iPhone 非人身安全级**（除非走 §9 付费 + 批准）；即便如此，iPhone 的保证 EEW 唤醒仍然是 cell broadcast。
2. **ntfy iOS 不真独立**（过 ntfy.sh + APNs），且受 iOS 26.2 无声 bug 影响——依赖前查 release notes。
3. **Time-Sensitive 唤不醒静音 iPhone**，确定无疑。
4. **外部看门狗尚未部署**（§3.3 / P4 未做）——所以“整个家黑了 / 服务器死了”这件事，**当前的外部检测能力为零**。设备自己仍会因收不到心跳而本地报 OFFLINE（A 层还在），但家以外没有任何东西会通知你。即便日后建成，分辨率也只有 ~5min：够“系统下线、改听官方”，不够当主地震警报。
5. **服务端自检有边界**：心跳现在携带签名的 `health`，`Store.Ping()` 失败即自报 `degraded`（已用停 Postgres 演练验证）。但 `Ping()` 只探「连接可用性」：**SQLite 档探不到数据库文件被删**（POSIX 下已打开的句柄仍有效），也探不到磁盘写满、数据损坏这类故障。它能覆盖的是「连接断了」这一类。
6. **EEW 目前单源**（仅 Wolfx，且默认关闭）——§6.1 的双源冗余未生效；Wolfx 是免费“無保証”第三方中继，大震时最易被打爆。手机务必保持**緊急速報メール 开启**。
7. **确认送达 / 升级未实现**：确认的语义本就是“设备 ack”而非“人知晓”；而**当前服务端根本不收集 ack**（客户端只用它门控本地消除）——“谁没确认”无人统计，也不会有任何升级。
8. **没有定时演练**（§3.4 未实现）——“在出事前证明它还活着”目前只能手动灌一条 CAP `status=Exercise` 来做。
9. **没有原生客户端**：唯一的自有接收端是浏览器 web 客户端，外加现成的 ntfy app。浏览器不能锁屏唤醒、不能绕勿扰、后台标签页会被节流——**§0 与 §2 里给 Android / 壁挂的那几行是平台能力上限，不是现在拿得到的东西**。
10. **OEM 杀进程**：等 P5 的 Android app 落地后才成为现实问题；到时即便全清单照做，某些 MIUI/ColorOS 仍会杀——首选 Pixel/GrapheneOS 当主力。
11. **设备面单租户**：`/api/devices` 与 `alerts/*` topic 是全局的，设备无 org 归属，多 org 部署下警报会跨 org 广播（见 §10.2）。
12. **无审计日志**：谁在何时发了哪条警报、改了谁的权限，除 alerts 历史本身外没有独立可查的审计轨迹。
13. **手机关机 / 无电 / 无网**：任何软件都无解。

---
*本规范随实现迭代。§10/§11 是**实现状态的唯一权威**，改代码时同步改它——本文件曾长期落后于代码，别再让它发生。研究全文存档于会话 transcript。*
