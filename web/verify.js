// AlertHub — client-side signature verification + replay/dedup/TTL accept gate.
//
// canonicalBytes() MUST stay byte-identical with server Canonical()
// (server/internal/alert/alert.go). SPEC.md §3–§4. A mismatch = every alert
// silently rejected, so there is a conformance test in scripts/ that checks a
// Go-signed vector verifies here.

const enc = new TextEncoder();

// base64url (no padding) -> Uint8Array
export function b64urlToBytes(s) {
  s = s.replace(/-/g, "+").replace(/_/g, "/");
  while (s.length % 4) s += "=";
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

// 13 fields, NFC-normalized strings, base-10 integers, '\n'-joined, no trailing
// newline — the exact contract from SPEC §3.2.
export function canonicalBytes(a) {
  const nfc = (x) => (x ?? "").toString().normalize("NFC");
  const parts = [
    String(a.schema_version),
    nfc(a.id),
    nfc(a.type),
    nfc(a.category),
    nfc(a.severity),
    nfc(a.title),
    nfc(a.body),
    nfc(a.action),
    nfc(a.source),
    String(a.issued_at),
    String(a.ttl),
    nfc(a.nonce),
    nfc(a.cancels),
  ];
  return enc.encode(parts.join("\n"));
}

// --- Ed25519 verify: WebCrypto first (Safari 17 / FF 129 / Chrome 137+),
//     @noble/ed25519 fallback for older engines. SPEC §3.4. ----------------
let _key = null;
let _webcryptoBroken = false;

async function webcryptoVerify(msg, sig, pubRaw) {
  if (!_key) {
    _key = await crypto.subtle.importKey("raw", pubRaw, { name: "Ed25519" }, false, ["verify"]);
  }
  return crypto.subtle.verify({ name: "Ed25519" }, _key, sig, msg);
}

let _noble = null;
async function nobleVerify(msg, sig, pubRaw) {
  if (!_noble) {
    _noble = await import("https://cdn.jsdelivr.net/npm/@noble/ed25519@2/+esm");
  }
  return _noble.verifyAsync(sig, msg, pubRaw);
}

export async function verifyAlert(a, pubRaw) {
  if (!a || typeof a.sig !== "string") return false;
  const msg = canonicalBytes(a);
  const sig = b64urlToBytes(a.sig);
  if (sig.length !== 64) return false;
  if (!_webcryptoBroken && globalThis.crypto?.subtle) {
    try {
      return await webcryptoVerify(msg, sig, pubRaw);
    } catch (_) {
      _webcryptoBroken = true; // engine lacks Ed25519 — use noble from now on
    }
  }
  return nobleVerify(msg, sig, pubRaw);
}

// --- Heartbeat verify (SPEC-SAFETY §3.1) -----------------------------------
// MUST match server CanonicalHeartbeat(): domain tag "hb1" + 4 fields, '\n'-joined.
export function canonicalHeartbeatBytes(h) {
  const parts = ["hb1", String(h.seq), String(h.issued_at), String(h.interval), String(h.active_count)];
  return enc.encode(parts.join("\n"));
}

export async function verifyHeartbeat(h, pubRaw) {
  if (!h || typeof h.sig !== "string") return false;
  const msg = canonicalHeartbeatBytes(h);
  const sig = b64urlToBytes(h.sig);
  if (sig.length !== 64) return false;
  if (!_webcryptoBroken && globalThis.crypto?.subtle) {
    try {
      return await webcryptoVerify(msg, sig, pubRaw);
    } catch (_) {
      _webcryptoBroken = true;
    }
  }
  return nobleVerify(msg, sig, pubRaw);
}

// --- Accept gate: signature -> skew -> dedup -> TTL (SPEC §4) ---------------
const SEEN_KEY = "alerthub_seen_v1";

function loadSeen() {
  try {
    return new Map(JSON.parse(localStorage.getItem(SEEN_KEY) || "[]"));
  } catch {
    return new Map();
  }
}
function saveSeen(seen) {
  try {
    localStorage.setItem(SEEN_KEY, JSON.stringify([...seen]));
  } catch {}
}

const seen = loadSeen();

// Returns { ok: true } if the message should be acted on, else { ok:false, reason }.
export async function acceptAlert(a, pubRaw, maxSkew = 120) {
  if (!(await verifyAlert(a, pubRaw))) return { ok: false, reason: "bad-sig" };
  const now = Math.floor(Date.now() / 1000);
  if (Math.abs(now - a.issued_at) > maxSkew) return { ok: false, reason: "stale" };
  if (seen.has(a.id) || seen.has("n:" + a.nonce)) return { ok: false, reason: "dup" };
  if (now - a.issued_at > a.ttl) return { ok: false, reason: "expired" };
  seen.set(a.id, a.issued_at);
  seen.set("n:" + a.nonce, a.issued_at);
  for (const [k, t] of seen) if (now - t > maxSkew) seen.delete(k); // bounded sweep
  saveSeen(seen);
  return { ok: true };
}
