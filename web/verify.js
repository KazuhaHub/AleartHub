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
// SPEC §8 rotation: the client accepts an ORDERED LIST of public keys and takes
// a signature that any of them validates. That is what lets a key be replaced
// without downtime — publish under the new key while clients still accept the
// old one, then drop the old one in a later client release. Verifying against a
// single key means every rotation is an outage.
let _webcryptoBroken = false;
const _keyCache = new Map(); // raw-key string -> imported CryptoKey

function keyId(raw) {
  let s = "";
  for (const b of raw) s += String.fromCharCode(b);
  return s;
}

async function webcryptoVerify(msg, sig, pubRaw) {
  const id = keyId(pubRaw);
  let k = _keyCache.get(id);
  if (!k) {
    k = await crypto.subtle.importKey("raw", pubRaw, { name: "Ed25519" }, false, ["verify"]);
    _keyCache.set(id, k);
  }
  return crypto.subtle.verify({ name: "Ed25519" }, k, sig, msg);
}

let _noble = null;
async function nobleVerify(msg, sig, pubRaw) {
  if (!_noble) {
    _noble = await import("https://cdn.jsdelivr.net/npm/@noble/ed25519@2/+esm");
  }
  return _noble.verifyAsync(sig, msg, pubRaw);
}

// asKeyList accepts either one key or a list, so every caller can pass whatever
// it has without each of them re-implementing the normalisation.
function asKeyList(pub) {
  if (!pub) return [];
  return Array.isArray(pub) ? pub.filter(Boolean) : [pub];
}

// verifyBytes tries each accepted key in order and returns true on the first
// that validates. Order matters only for speed: the current key should be first.
async function verifyBytes(msg, sig, pub) {
  for (const raw of asKeyList(pub)) {
    if (!_webcryptoBroken && globalThis.crypto?.subtle) {
      try {
        if (await webcryptoVerify(msg, sig, raw)) return true;
        continue;
      } catch (_) {
        _webcryptoBroken = true; // engine lacks Ed25519 — use noble from here on
      }
    }
    try {
      if (await nobleVerify(msg, sig, raw)) return true;
    } catch (_) { /* try the next key */ }
  }
  return false;
}

export async function verifyAlert(a, pub) {
  if (!a || typeof a.sig !== "string") return false;
  const sig = b64urlToBytes(a.sig);
  if (sig.length !== 64) return false;
  return verifyBytes(canonicalBytes(a), sig, pub);
}

// --- Heartbeat verify (SPEC-SAFETY §3.1) -----------------------------------
// MUST match server CanonicalHeartbeat(): domain tag "hb1" + 4 fields, '\n'-joined.
export function canonicalHeartbeatBytes(h) {
  // Domain tag "hb2": bumped from hb1 when `health` joined the signed set, so an
  // old 5-field beat can never cross-verify against a new 6-field one.
  // MUST stay byte-identical with Go's alert.CanonicalHeartbeat.
  const parts = [
    "hb2",
    String(h.seq),
    String(h.issued_at),
    String(h.interval),
    String(h.active_count),
    String(h.health ?? ""),
  ];
  return enc.encode(parts.join("\n"));
}

export async function verifyHeartbeat(h, pub) {
  if (!h || typeof h.sig !== "string") return false;
  const sig = b64urlToBytes(h.sig);
  if (sig.length !== 64) return false;
  // Same rotation rule as alerts: a beat signed with the previous key must still
  // verify during the overlap, or a rotation would make every client believe the
  // server had died.
  return verifyBytes(canonicalHeartbeatBytes(h), sig, pub);
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

// Per-ALERT state, kept separately from the nonce set: a renewal re-uses the id
// but carries a fresh nonce, so the two need different bookkeeping. We remember
// the last issued_at and severity per id in order to tell a stale re-delivery,
// a renewal and an escalation apart (SPEC §5.2).
const SEEN_ALERTS_KEY = "alerthub_seen_alerts_v1";

function loadSeenAlerts() {
  try {
    return new Map(JSON.parse(localStorage.getItem(SEEN_ALERTS_KEY) || "[]"));
  } catch {
    return new Map();
  }
}
function saveSeenAlerts(m) {
  try {
    localStorage.setItem(SEEN_ALERTS_KEY, JSON.stringify([...m]));
  } catch {}
}

const seen = loadSeen();
const seenAlerts = loadSeenAlerts();

const SEV_RANK = { notice: 0, warning: 1, critical: 2, emergency: 3 };

// acceptAlert implements the SPEC §4 accept gate plus the §5.2 renewal rules.
//
// A re-issue under a known id is NOT the same thing as a replay. A replay is a
// captured packet resent verbatim — it carries the same nonce, and that is what
// the nonce check catches. A renewal is a fresh message the server signed to
// extend or update a live alert, and dropping it (as this used to) meant §5.2
// simply did not work.
//
// Returns { ok, reason?, renewal?, escalation? }:
//   ok:false            → drop
//   ok:true             → new alert, present it
//   ok:true, renewal    → known id, newer, severity NOT higher: extend only,
//                         do not re-alarm (re-alarming trains people to ignore alerts)
//   ok:true, escalation → known id, newer, severity HIGHER: present again. This is
//                         a safety requirement — an EEW going 震度4 → 6弱, or a CAP
//                         Update raising Moderate → Extreme, must not pass quietly.
export async function acceptAlert(a, pubRaw, maxSkew = 120) {
  if (!(await verifyAlert(a, pubRaw))) return { ok: false, reason: "bad-sig" };
  const now = Math.floor(Date.now() / 1000);
  if (Math.abs(now - a.issued_at) > maxSkew) return { ok: false, reason: "stale" };
  // The nonce is per-message, so a repeat of it is a genuine replay.
  if (seen.has("n:" + a.nonce)) return { ok: false, reason: "dup" };
  if (now - a.issued_at > a.ttl) return { ok: false, reason: "expired" };

  const prev = seenAlerts.get(a.id);
  let renewal = false, escalation = false;
  if (prev) {
    if (a.issued_at <= prev.issued_at) return { ok: false, reason: "dup" }; // stale re-delivery
    if ((SEV_RANK[a.severity] ?? 0) > (SEV_RANK[prev.severity] ?? 0)) escalation = true;
    else renewal = true;
  }

  seen.set("n:" + a.nonce, a.issued_at);
  seenAlerts.set(a.id, { issued_at: a.issued_at, severity: a.severity });
  for (const [k, t] of seen) if (now - t > maxSkew) seen.delete(k); // bounded sweep
  for (const [k, v] of seenAlerts) if (now - v.issued_at > maxSkew) seenAlerts.delete(k);
  saveSeen(seen);
  saveSeenAlerts(seenAlerts);
  return { ok: true, renewal, escalation };
}
