// AlertHub browser client. Connects MQTT-over-WS, runs the SPEC §4 accept gate,
// renders by severity (toast for notice/warning, fullscreen for critical/emergency),
// forced-ack, cancel/解除 handling, and a WebAudio alarm. SPEC §5/§9.
import { b64urlToBytes, acceptAlert, verifyHeartbeat } from "./verify.js";

const $ = (s) => document.querySelector(s);

const CAT = {
  earthquake: "🌐 地震", fire: "🔥 火灾", weather: "🌧 天气",
  system: "🖥 系统", security: "🔒 安全", custom: "❗ 提醒",
};
const catLabel = (c) => CAT[c] || c;

let cfg, pubRaw, maxSkew = 120, deviceId, client;
const activeFs = [];            // critical/emergency alerts currently shown
const cancelledSet = new Set(); // ids recalled by a cancel (suppress late arrivals)

// --- FAIL-LOUD heartbeat watchdog state (SPEC-SAFETY §3.1/§3.2) ---
let hbInterval = 10;            // seconds, learned from the heartbeat
let lastBeatMs = 0;            // local wall-clock of last GOOD heartbeat
let serverDegraded = false;    // server's own signed health verdict (SPEC-SAFETY §3.1)
let lastSeq = -1;
let healthyStreak = 0;         // consecutive good beats (flap damping)
let driftStreak = 0;          // consecutive beats with large clock drift
let failState = "init";       // init | ok | degraded | offline
let offlineChimeTimer = null;

// ---------- device id ----------
function getDeviceId() {
  let id = localStorage.getItem("alerthub_device_id");
  if (!id) {
    id = "dev-" + Math.random().toString(36).slice(2, 8) + Date.now().toString(36).slice(-4);
    localStorage.setItem("alerthub_device_id", id);
  }
  return id;
}

// ---------- audio (alarm) ----------
let audioCtx = null, alarmTimer = null;
function unlockAudio() {
  if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)();
  if (audioCtx.state === "suspended") audioCtx.resume();
}
function beep(freq, dur) {
  const o = audioCtx.createOscillator();
  const g = audioCtx.createGain();
  o.type = "square";
  o.frequency.value = freq;
  o.connect(g); g.connect(audioCtx.destination);
  const t = audioCtx.currentTime;
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(0.25, t + 0.02);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  o.start(t); o.stop(t + dur + 0.02);
}
function updateAlarm() {
  const wantAlarm = audioCtx && activeFs.some((a) => a.severity === "emergency");
  if (wantAlarm && !alarmTimer) {
    const pattern = () => { beep(880, 0.18); setTimeout(() => beep(660, 0.18), 220); };
    pattern();
    alarmTimer = setInterval(pattern, 900);
  } else if (!wantAlarm && alarmTimer) {
    clearInterval(alarmTimer); alarmTimer = null;
  }
}

// ---------- connection status ----------
function setConn(ok) {
  $("#conn-dot").className = "dot " + (ok ? "on" : "off");
  $("#conn-text").textContent = ok ? "已连接" : "断开 · 重连中…";
}
function maybeIdle() {
  const busy = activeFs.length > 0 || $("#toasts").children.length > 0;
  $("#idle").hidden = busy;
}

// ---------- fullscreen overlay ----------
function showTop() {
  const ov = $("#overlay");
  ov.classList.remove("critical", "emergency");
  if (activeFs.length === 0) {
    ov.classList.remove("show");
    $("#o-title").classList.remove("pulse");
    updateAlarm();
    maybeIdle();
    return;
  }
  const a = activeFs[activeFs.length - 1];
  ov.classList.add("show", a.severity);
  $("#o-cat").textContent = catLabel(a.category);
  $("#o-title").textContent = a.title;
  $("#o-body").textContent = a.body || "";
  $("#o-action").textContent = a.action || "";
  const t = new Date(a.issued_at * 1000).toLocaleTimeString();
  $("#o-meta").textContent = `来源 ${a.source} · ${t}` +
    (activeFs.length > 1 ? ` · 还有 ${activeFs.length - 1} 条待确认` : "");
  $("#o-title").classList.toggle("pulse", a.severity === "emergency");
  updateAlarm();
}
function pushFullscreen(a) {
  if (activeFs.some((x) => x.id === a.id)) return;
  activeFs.push(a);
  $("#idle").hidden = true;
  showTop();
}
function removeFs(id) {
  const i = activeFs.findIndex((x) => x.id === id);
  if (i >= 0) activeFs.splice(i, 1);
}

// ---------- toasts (notice / warning) ----------
function showToast(a) {
  const el = document.createElement("div");
  el.className = "toast " + a.severity;
  el.dataset.id = a.id;
  el.innerHTML = `<div class="t-cat"></div><div class="t-title"></div>
    <div class="t-body"></div><div class="t-action"></div>`;
  el.querySelector(".t-cat").textContent = `${catLabel(a.category)} · ${a.severity}`;
  el.querySelector(".t-title").textContent = a.title;
  el.querySelector(".t-body").textContent = a.body || "";
  const act = el.querySelector(".t-action");
  if (a.action) act.textContent = "→ " + a.action; else act.remove();
  $("#toasts").appendChild(el);
  $("#idle").hidden = true;
}
function removeToast(id) {
  $("#toasts").querySelectorAll(`.toast[data-id="${id}"]`).forEach((e) => e.remove());
  maybeIdle();
}

// ---------- TTL auto-dismiss (SPEC §4 step 4) ----------
function scheduleExpiry(a) {
  const ms = (a.issued_at + a.ttl) * 1000 - Date.now();
  if (ms <= 0) return;
  setTimeout(() => { removeFs(a.id); removeToast(a.id); showTop(); },
    Math.min(ms, 2 ** 31 - 1));
}

// ---------- render + cancel ----------
function renderAlert(a) {
  scheduleExpiry(a);
  if (a.severity === "critical" || a.severity === "emergency") pushFullscreen(a);
  else showToast(a);
}
function handleCancel(id) {
  if (!id) return;
  cancelledSet.add(id);
  removeFs(id);
  removeToast(id);
  showTop();
}
function clearSnapshot() {
  // empty retained alerts/active = "nothing active" (SPEC §5.1)
  activeFs.length = 0;
  showTop();
}

// ---------- FAIL-LOUD: heartbeat + local watchdog (SPEC-SAFETY §3) ----------
async function handleHeartbeat(buf) {
  let hb;
  try { hb = JSON.parse(buf.toString()); } catch { return; }
  if (!(await verifyHeartbeat(hb, pubRaw))) {
    console.warn("[alerthub] heartbeat sig invalid — ignoring");
    return; // a forged/invalid heartbeat must NOT count as liveness
  }
  lastBeatMs = Date.now();
  if (hb.interval > 0) hbInterval = hb.interval;
  lastSeq = hb.seq;

  // The beat arriving proves the link is alive; `health` is the server's verdict
  // on ITSELF. A degraded server still beats (going silent would tell us less),
  // so this is a separate signal from the watchdog's connection states.
  serverDegraded = hb.health && hb.health !== "ok";
  applyServerHealth();

  // clock-drift check: the signed issued_at is the trusted time reference.
  const drift = Math.floor(Date.now() / 1000) - hb.issued_at;
  if (Math.abs(drift) > 30) {
    driftStreak++;
    if (driftStreak >= 3) showClockWarn(drift);
  } else {
    driftStreak = 0;
    hideClockWarn();
  }
}

// Local timer — fires on its OWN schedule, NOT driven by incoming messages, so it
// still alarms when the socket is silently dead. This is the whole point.
function startWatchdog() {
  if (startWatchdog._t) return;
  lastBeatMs = Date.now(); // grace the initial connect window
  startWatchdog._t = setInterval(evalLiveness, 3000);
}
function evalLiveness() {
  const elapsed = (Date.now() - lastBeatMs) / 1000;
  const degradedAt = 3 * hbInterval; // ~30s
  const offlineAt = 6 * hbInterval;  // ~60s
  if (elapsed > offlineAt) {
    setFailState("offline");
    healthyStreak = 0;
  } else if (elapsed > degradedAt) {
    if (failState !== "offline") setFailState("degraded");
    healthyStreak = 0;
  } else {
    // healthy beat window; require a couple before clearing OFFLINE (flap damping)
    healthyStreak++;
    if (failState === "offline" && healthyStreak < 2) return;
    setFailState("ok");
  }
}
// applyServerHealth shows the server's self-reported degradation. It only paints
// while the connection is otherwise healthy — a dead link is the louder problem
// and setFailState owns the banner in that case.
function applyServerHealth() {
  if (failState !== "ok") return;
  const el = $("#failloud");
  if (serverDegraded) {
    el.hidden = false;
    el.className = "degraded";
    el.textContent = "⚠ 服务端自报降级 — 告警仍在发送，但部分功能（历史/投递）可能不可用";
  } else {
    el.hidden = true;
    el.className = "";
  }
}

function setFailState(s) {
  if (s === failState) return;
  failState = s;
  const el = $("#failloud");
  const dot = $("#conn-dot");
  if (s === "ok") {
    el.hidden = true;
    el.className = "";
    dot.className = "dot on";
    $("#conn-text").textContent = "已连接 · 心跳正常";
    stopOfflineChime();
    applyServerHealth(); // the link is back; the server may still self-report degraded
  } else if (s === "degraded") {
    el.hidden = false;
    el.className = "degraded";
    el.textContent = "⚠ 与告警系统的连接中断 — 正在重连…";
    dot.className = "dot";
    stopOfflineChime();
  } else if (s === "offline") {
    el.hidden = false;
    el.className = "offline";
    el.textContent = "⚠ 告警系统离线 — 现在收不到警报了。请检查服务器/网络，并改听官方渠道。";
    dot.className = "dot off";
    $("#conn-text").textContent = "离线";
    startOfflineChime();
  }
}
function startOfflineChime() {
  if (offlineChimeTimer || !audioCtx) return;
  const tone = () => { beep(440, 0.25); setTimeout(() => beep(330, 0.25), 300); };
  tone();
  offlineChimeTimer = setInterval(tone, 2500); // gentler/slower than the alarm
}
function stopOfflineChime() {
  if (offlineChimeTimer) { clearInterval(offlineChimeTimer); offlineChimeTimer = null; }
}
function showClockWarn(drift) {
  const el = $("#clockwarn");
  el.hidden = false;
  el.textContent = `⚠ 本机时钟偏差约 ${drift}s — 可能会静默丢弃真实警报。请开启「自动设置日期与时间」。`;
}
function hideClockWarn() { $("#clockwarn").hidden = true; }

// ---------- MQTT ----------
async function onMessage(topic, buf) {
  if (topic === "system/heartbeat") { handleHeartbeat(buf); return; }
  if (!buf || buf.length === 0) {
    if (topic === "alerts/active") clearSnapshot();
    return;
  }
  let a;
  try { a = JSON.parse(buf.toString()); } catch { return; }
  const res = await acceptAlert(a, pubRaw, maxSkew);
  if (!res.ok) { console.debug("[alerthub] drop", res.reason, a && a.id); return; }
  if (a.type === "cancel") { handleCancel(a.cancels); return; }
  if (cancelledSet.has(a.id)) return; // recalled before we saw it

  // SPEC §5.2: a re-issue under a known id either extends the alert or escalates
  // it. Extending must be silent — re-alarming for an unchanged warning is how
  // you train people to ignore alerts. Escalating must NOT be silent, because a
  // 震度4 that turns out to be 6弱 is a different instruction to the person
  // reading it.
  if (res.renewal) {
    console.debug("[alerthub] renewal", a.id, "ttl ->", a.ttl);
    extendAlert(a);
    return;
  }
  if (res.escalation) {
    console.debug("[alerthub] ESCALATION", a.id, "->", a.severity);
    renderAlert(a); // full presentation again: new severity, new instruction
    return;
  }
  renderAlert(a);
}

// extendAlert refreshes a live alert's validity window (and its wording, which a
// CAP Update may have changed) WITHOUT re-triggering the fullscreen takeover, the
// siren, or the acknowledgement requirement.
function extendAlert(a) {
  const live = activeFs.find((x) => x.id === a.id);
  if (!live) {
    // We never presented it — either it was a toast, or we joined late. Present
    // it now rather than silently extending something the user cannot see.
    renderAlert(a);
    return;
  }
  // Mutate in place: showTop() re-renders from this object, so the wording and
  // the new expiry take effect without tearing the overlay down and rebuilding
  // it (which would look like a fresh alarm).
  live.issued_at = a.issued_at;
  live.ttl = a.ttl;
  live.title = a.title;
  live.body = a.body;
  live.action = a.action;
  scheduleExpiry(live); // the extended TTL needs its own dismissal timer
  showTop();
}

function publishAck(id) {
  if (!client || !client.connected) return;
  const payload = JSON.stringify({
    alert_id: id, device_id: deviceId, ack_at: Math.floor(Date.now() / 1000),
  });
  client.publish(`alerts/${id}/ack/${deviceId}`, payload, { qos: 1, retain: true });
}

function connect() {
  const scheme = location.protocol === "https:" ? "wss" : "ws";
  const url = `${scheme}://${location.hostname}:${cfg.ws_port}`;
  client = window.mqtt.connect(url, {
    username: cfg.mqtt_user,
    password: cfg.mqtt_pw,
    clientId: "alerthub-" + deviceId,
    clean: true,
    reconnectPeriod: 2000,
    will: {
      topic: `status/${deviceId}`,
      payload: JSON.stringify({ device_id: deviceId, state: "offline", at: 0 }),
      qos: 1, retain: true,
    },
  });
  client.on("connect", () => {
    setConn(true);
    client.subscribe("alerts/active", { qos: 1 });
    client.subscribe("alerts/events", { qos: 1 });
    client.subscribe("system/heartbeat", { qos: 1 });
    startWatchdog();
    client.publish(`status/${deviceId}`, JSON.stringify({
      device_id: deviceId, state: "online", at: Math.floor(Date.now() / 1000), client: "web/1.0",
    }), { qos: 1, retain: true });
  });
  client.on("reconnect", () => setConn(false));
  client.on("close", () => setConn(false));
  client.on("offline", () => setConn(false));
  client.on("error", (e) => console.warn("[alerthub] mqtt", e.message));
  client.on("message", onMessage);
}

// ---------- boot ----------
async function boot() {
  deviceId = getDeviceId();
  $("#dev-id").textContent = deviceId;
  try {
    cfg = await (await fetch("/pubkey")).json();
  } catch (e) {
    $("#conn-text").textContent = "无法获取配置";
    return;
  }
  pubRaw = b64urlToBytes(cfg.pubkey);
  maxSkew = cfg.max_skew || 120;
  connect();
}

$("#ackbtn").onclick = () => {
  const a = activeFs[activeFs.length - 1];
  if (!a) return;
  publishAck(a.id);
  removeFs(a.id);
  showTop();
};

$("#armbtn").onclick = () => {
  unlockAudio();
  $("#arm").remove();
  $("#statusbar").hidden = false;
  $("#idle").hidden = false;
  $("#audio-state").textContent = "🔔 声音已启用";
  boot();
};
