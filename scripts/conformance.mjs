// Cross-language signing conformance: publish via the Go server, then verify the
// Go-produced signature using the SAME web/verify.js the browser client uses.
// Proves SPEC §3 canonicalization is byte-identical across Go and JS.
import { canonicalBytes, canonicalHeartbeatBytes, b64urlToBytes, verifyAlert, verifyHeartbeat } from "../web/verify.js";

const base = process.env.BASE || "http://localhost:8080";
const token = process.env.ALERTHUB_ADMIN_TOKEN || "dev-admin-token";

const cfg = await (await fetch(base + "/pubkey")).json();
const pubRaw = b64urlToBytes(cfg.pubkey);

const env = await (await fetch(base + "/api/publish", {
  method: "POST",
  headers: { "Content-Type": "application/json", Authorization: "Bearer " + token },
  body: JSON.stringify({
    severity: "emergency",
    category: "earthquake",
    title: "正在发生地震 🌐",
    body: "震中距你约 42 公里，预计 15 秒后到达。",
    action: "趴下，掩护，抓牢",
  }),
})).json();

const ok = await verifyAlert(env, pubRaw);
const tampered = { ...env, title: env.title + "X" };
const tamperRejected = (await verifyAlert(tampered, pubRaw)) === false;

// --- heartbeat leg -----------------------------------------------------------
// The heartbeat has its OWN canonical form (domain tag "hb2") and its own pair of
// implementations, so it needs its own conformance check. The signed sample comes
// from `go run ./server/cmd/hbgen`, which uses the same alert.SignHeartbeat as the
// server (the live beat travels over MQTT, which this hermetic script has no
// client for).
const hbJSON = process.env.ALERTHUB_HB_JSON;
let hbOK = null, hbTamperRejected = null;
if (hbJSON) {
  const hb = JSON.parse(hbJSON);
  hbOK = await verifyHeartbeat(hb, pubRaw);
  // Flipping the health verdict must invalidate the signature — otherwise a
  // compromised device could downgrade "degraded" to "ok" and mute the warning.
  const hbTampered = { ...hb, health: hb.health === "ok" ? "degraded" : "ok" };
  hbTamperRejected = (await verifyHeartbeat(hbTampered, pubRaw)) === false;
}

console.log("envelope id :", env.id);
console.log("canonical   :", JSON.stringify(new TextDecoder().decode(canonicalBytes(env))));
console.log("VERIFY (Go-signed -> web/verify.js):", ok ? "✅ PASS" : "❌ FAIL");
console.log("TAMPER REJECTED                    :", tamperRejected ? "✅ PASS" : "❌ FAIL");
if (hbJSON) {
  console.log("hb canonical:", JSON.stringify(new TextDecoder().decode(canonicalHeartbeatBytes(JSON.parse(hbJSON)))));
  console.log("HEARTBEAT VERIFY (Go -> verify.js) :", hbOK ? "✅ PASS" : "❌ FAIL");
  console.log("HEARTBEAT HEALTH TAMPER REJECTED   :", hbTamperRejected ? "✅ PASS" : "❌ FAIL");
} else {
  console.log("HEARTBEAT                          : ⚠️  SKIPPED (no ALERTHUB_HB_JSON)");
}

const allPass = ok && tamperRejected && (!hbJSON || (hbOK && hbTamperRejected));
process.exit(allPass ? 0 : 1);
