# 家庭警报广播系统 · 设计文档（原始）

> 这是项目最初的设计讨论文档，作为出发点与背景保留。**最终锁定的协议以 [../SPEC.md](../SPEC.md) 为准**——SPEC 在本文基础上修订了定位、补齐了 Schema、锁定了签名/防重放/撤回机制。

---

## 1. 定位

这是一个**警报系统**，不是通知中心。核心原则：
- **没有"一切正常"这一档**。每一条消息都是"有情况、需要你注意"的警报，区别只在紧急程度。
- **"通知 / notification" 只指送达方式**（系统级小弹窗那种），不指消息本身。消息一律称为**警报 / alert**。
- 覆盖范围不限于 homelab 系统告警，也包括**现实物理紧急情况**（地震、火灾、天气等）。

## 2. 两个正交维度：类别 × 紧急度

### 2.1 紧急度 severity
| 级别 | 含义 | 送达方式 |
|---|---|---|
| `notice` 注意 | 有异常但不紧急 | 系统通知 |
| `warning` 警告 | 需要关注 | 系统通知 |
| `critical` 严重 | 需要立刻处理 | 全屏接管 |
| `emergency` 紧急 | 必须马上行动 | 全屏 + 持续警报音 + 强制确认 |

### 2.2 类别 category
`earthquake` / `fire` / `weather` / `system` / `security` / `custom`，决定图标/标签与建议处置 `action`。

> 地震不是一个紧急度，而是一种类别。一场地震可能 `critical` 也可能 `emergency`。

## 3. 消息协议（原始草案）
```json
{
  "id": "uuid",
  "category": "earthquake | fire | weather | system | security | custom",
  "severity": "notice | warning | critical | emergency",
  "title": "正在发生地震",
  "body": "震中距你约 42 公里，预计 15 秒后到达。",
  "action": "趴下，掩护，抓牢",
  "ttl": 60,
  "sig": "HMAC / Ed25519 签名"
}
```

## 4. 系统架构
- **消息总线**：MQTT（Mosquitto）——QoS1 保证送达、retained 补收、心跳在线监控。
- **服务端**：Go（单二进制）+ chi/Gin、paho MQTT、crypto/hmac 或 ed25519、SQLite（modernc）、React+MUI 面板。
- **客户端**：Windows/Mac 用 Tauri 2；Android 原生 Kotlin（前台服务 + FSI + bypassDnd）。

## 5. 客户端 UI 方向
极简：①出什么事（大标题）②多严重（纯色背景）③该干什么（一句处置）+ 确认按钮。原型 `alert-fullscreen-prototype.html`。

## 6. 安全
- 内容可信：消息签名（HMAC 或 Ed25519），纯内网也不能省。
- 传输安全：视部署而定（Cloudflare Tunnel / MQTTS / WireGuard）。
- Broker 认证：关匿名 + 用户名密码 + ACL。

## 7. 可靠性提醒（人身安全场景）
- 手机端需不依赖自建服务的备用通道（ntfy + 系统 critical 通知）。
- 不能依赖单一 broker / 单一推送通道。
- 异地设备需确认隧道保活。

## 8. 发布来源
系统（监控脚本）、火灾（HA 烟感转发）、地震（J-Alert 类源）、任意（面板手动）。Go 预留 webhook 入口。

## 9. 命名
`home-alert` / `alert-hub` / `kazuha/home-alert` / `kazuhacast`。

## 10. 分阶段路线（MVP 优先）
1. Mosquitto + Go 发布端（含签名）+ Tauri 桌面端，跑通 critical/emergency 全屏。
2. Web 面板、历史、设备在线态。
3. Android 客户端（保活 + FSI）。
4. 真实来源（HA 烟感、地震预警）+ 冗余通道（ntfy）。

## 11. 待确认问题
1. "内网"的确切含义（物理 LAN vs 隧道虚拟内网）。
2. 面板与 broker 是否同机。
3. 最终项目名。
4. 是否与 Kazuha 体系设计语言统一。
