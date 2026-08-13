// Cross-language signing conformance: publish via the Go server, then verify the
// Go-produced signature using the SAME web/verify.js the browser client uses.
// Proves SPEC §3 canonicalization is byte-identical across Go and JS.
import { canonicalBytes, b64urlToBytes, verifyAlert } from "../web/verify.js";

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

console.log("envelope id :", env.id);
console.log("canonical   :", JSON.stringify(new TextDecoder().decode(canonicalBytes(env))));
console.log("VERIFY (Go-signed -> web/verify.js):", ok ? "✅ PASS" : "❌ FAIL");
console.log("TAMPER REJECTED                    :", tamperRejected ? "✅ PASS" : "❌ FAIL");

process.exit(ok && tamperRejected ? 0 : 1);
