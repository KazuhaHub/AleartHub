// Cross-language signing conformance: publish via the Go server, then verify the
// Go-produced signature using the SAME web/verify.js the browser client uses.
// Proves SPEC §3 canonicalization is byte-identical across Go and JS.
import { canonicalBytes, canonicalHeartbeatBytes, b64urlToBytes, verifyAlert, verifyHeartbeat, acceptAlert } from "../web/verify.js";

const base_ = process.env.BASE || "http://localhost:8080";
const token = process.env.ALERTHUB_ADMIN_TOKEN || "dev-admin-token";

const cfg = await (await fetch(base_ + "/pubkey")).json();
const pubRaw = b64urlToBytes(cfg.pubkey);

const env = await (await fetch(base_ + "/api/publish", {
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

// --- §5.2 accept-gate: replay vs renewal vs escalation -----------------------
// This classification lives ONLY in web/verify.js — no Go test can reach it — and
// it decides whether a re-issued alert stays silent or alarms again, which is a
// safety behaviour. The fixtures are signed by the same Go code the server uses
// (server/cmd/gatefixtures), because the client cannot forge them itself.
const gate = { first: null, replay: null, stale: null, renewal: null, escalation: null };
const fixJSON = process.env.ALERTHUB_GATE_JSON;
if (fixJSON) {
  const f = JSON.parse(fixJSON);
  gate.first      = (await acceptAlert(f.base, pubRaw)).ok === true;
  gate.replay     = (await acceptAlert(f.base, pubRaw)).ok === false;      // same nonce
  gate.stale      = (await acceptAlert(f.stale, pubRaw)).ok === false;     // older issued_at
  const r         = await acceptAlert(f.renewal, pubRaw);
  gate.renewal    = r.ok === true && r.renewal === true && r.escalation !== true;
  const e         = await acceptAlert(f.escalation, pubRaw);
  gate.escalation = e.ok === true && e.escalation === true;
}

// --- §8 key rotation ---------------------------------------------------------
// A rotation is only downtime-free if a client accepts BOTH keys during the
// overlap. Verify that a signature made under the real key still passes when the
// key list also contains an unrelated key, and that a list WITHOUT the real key
// rejects it (otherwise "accept a list" would just mean "accept anything").
const rot = { overlapAccepts: null, wrongListRejects: null };
{
  const decoy = new Uint8Array(32); // not a real key
  rot.overlapAccepts   = (await verifyAlert(env, [decoy, pubRaw])) === true;
  rot.wrongListRejects = (await verifyAlert(env, [decoy])) === false;
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
if (fixJSON) {
  console.log("GATE accept / replay / stale       :", gate.first && gate.replay && gate.stale ? "✅ PASS" : "❌ FAIL");
  console.log("GATE renewal is SILENT (§5.2)      :", gate.renewal ? "✅ PASS" : "❌ FAIL");
  console.log("GATE escalation RE-ALARMS (§5.2)   :", gate.escalation ? "✅ PASS" : "❌ FAIL");
} else {
  console.log("GATE §5.2                          : ⚠️  SKIPPED (no ALERTHUB_GATE_JSON)");
}

console.log("ROTATION overlap accepts both      :", rot.overlapAccepts ? "✅ PASS" : "❌ FAIL");
console.log("ROTATION list without key rejects  :", rot.wrongListRejects ? "✅ PASS" : "❌ FAIL");

const gateOK = !fixJSON || (gate.first && gate.replay && gate.stale && gate.renewal && gate.escalation);
const allPass = ok && tamperRejected && (!hbJSON || (hbOK && hbTamperRejected)) && gateOK &&
  rot.overlapAccepts && rot.wrongListRejects;
process.exit(allPass ? 0 : 1);
