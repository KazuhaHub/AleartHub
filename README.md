# AlertHub

> 自托管的家庭**警报**广播系统：把一条警报（地震、火灾、系统故障…）实时、可靠地推送到家里所有设备，并按紧急程度以不同强度呈现——从安静的 toast 到强制接管的全屏 + 警报音 + 强制确认。

一个 Go 单二进制内嵌 MQTT broker，用 Ed25519 给每条警报签名，提供浏览器客户端 + 发布面板。零外部依赖（无需 Docker / Mosquitto）。

## 快速开始

```bash
make run      # = go run ./server，启动 broker + API + 网页，零外部依赖
```
然后：
1. 打开 http://localhost:8080/ （客户端）→ 点 **启用警报**
2. 打开 http://localhost:8080/publish.html （面板）→ 点 **发送警报**
3. 客户端立刻全屏 + 警报音 + 强制确认

首次启动自动在 `keys/` 生成 Ed25519 密钥（已 gitignore）。

```bash
make test     # 跨语言签名一致性测试（Go 签名 ↔ web/verify.js 验签）
```

## 文档

- [SPEC.md](SPEC.md) — v1 协议（信封、Ed25519 签名规范化、防重放、MQTT 拓扑、撤回/解除、ACL）。**权威契约**。
- [SPEC-SAFETY.md](SPEC-SAFETY.md) — 人身安全级硬化（冗余多通道、fail-loud 心跳、确认送达/升级、日本 EEW、ntfy、CAP API、各平台唤醒）。
- [docs/DESIGN.md](docs/DESIGN.md) — 最初的设计讨论存档。

> ⚠️ **诚实定位**：当前实现**补充**官方渠道（运营商緊急地震速報、认证烟感），不取代。iPhone 无法保证唤醒静音机；睡眠/关机设备收不到。详见 SPEC-SAFETY §0/§11。

## 配置（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `ALERTHUB_HTTP_ADDR` | `:8080` | HTTP（面板 + 客户端 + API）|
| `ALERTHUB_MQTT_TCP` | `:1883` | MQTT/TCP |
| `ALERTHUB_MQTT_WS` | `:1884` | MQTT/WebSocket（浏览器客户端）|
| `ALERTHUB_ADMIN_TOKEN` | `dev-admin-token` | 发布面板 Bearer token（**生产必改**）|
| `ALERTHUB_MQTT_PW` / `ALERTHUB_CLIENT_PW` | dev 默认 | broker publisher / client 密码 |
| `ALERTHUB_NTFY_URL` | （空=禁用）| 自托管 ntfy 基址，独立兜底通道 |
| `ALERTHUB_NTFY_TOKEN` | | 自托管 ntfy bearer token |
| `ALERTHUB_NTFY_SH_TOPIC` | （空）| 公共 ntfy.sh 兜底 topic（仅 critical/emergency，通用文案）|

## 状态

| 阶段 | 内容 | 状态 |
|---|---|---|
| MVP | broker + 签名 + web 客户端/面板 + 全屏 | ✅ |
| P0 | **fail-loud 心跳 + ntfy 独立通道** | ✅ |
| P1+ | CAP API、日本 EEW 双源、确认送达升级、演练、Android/Tauri 原生端 | 路线图见 SPEC-SAFETY §10 |

## 测试

```bash
go test ./...            # 单元测试（含 ntfy 扇出）
make test               # 跨语言签名一致性（含篡改拒绝）
```
