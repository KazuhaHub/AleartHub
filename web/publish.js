// AlertHub publish panel. POSTs to /api/publish and /api/cancel with the admin
// Bearer token, and lists recent alerts from /api/history. SPEC §7.
const $ = (s) => document.querySelector(s);

const token = () => $("#token").value.trim();
$("#token").value = localStorage.getItem("alerthub_admin_token") || "dev-admin-token";
$("#token").addEventListener("change", () =>
  localStorage.setItem("alerthub_admin_token", token()));

function msg(text, ok) {
  const m = $("#msg");
  m.textContent = text;
  m.className = ok ? "ok" : "err";
}

async function api(path, body) {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: "Bearer " + token() },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${res.status} ${(await res.text()).trim()}`);
  return res.json();
}

const QUICK = {
  eq:    { severity: "emergency", category: "earthquake", title: "正在发生地震", body: "震中距你约 42 公里，预计 15 秒后到达。", action: "" },
  fire:  { severity: "emergency", category: "fire", title: "检测到火警", body: "厨房烟感触发，请立即确认。", action: "" },
  node:  { severity: "critical", category: "system", title: "节点失联：pve-01", body: "心跳超时 90 秒。", action: "" },
  login: { severity: "notice", category: "security", title: "异地登录", body: "来自 Osaka 的新登录。", action: "" },
};

document.querySelectorAll("[data-q]").forEach((b) =>
  b.addEventListener("click", () => {
    const q = QUICK[b.dataset.q];
    $("#severity").value = q.severity;
    $("#category").value = q.category;
    $("#title").value = q.title;
    $("#body").value = q.body;
    $("#action").value = q.action;
    $("#ttl").value = "";
  }));

$("#send").addEventListener("click", async () => {
  const body = {
    severity: $("#severity").value,
    category: $("#category").value,
    title: $("#title").value,
    body: $("#body").value,
    action: $("#action").value,
  };
  const ttl = parseInt($("#ttl").value, 10);
  if (!isNaN(ttl) && ttl > 0) body.ttl = ttl;
  try {
    const a = await api("/api/publish", body);
    msg(`已发布 ${a.severity}/${a.category} · id ${a.id.slice(0, 8)}…`, true);
    loadHistory();
  } catch (e) {
    msg("发布失败：" + e.message, false);
  }
});

async function cancelAlert(id) {
  try {
    await api("/api/cancel", { id });
    msg("已解除 " + id.slice(0, 8) + "…", true);
    loadHistory();
  } catch (e) {
    msg("解除失败：" + e.message, false);
  }
}

async function loadHistory() {
  let items;
  try {
    const res = await fetch("/api/history", { headers: { Authorization: "Bearer " + token() } });
    if (!res.ok) throw new Error(res.status);
    items = await res.json();
  } catch (e) {
    $("#hist").innerHTML = `<div class="hist-item" style="color:var(--muted)">无法加载历史（检查 Token）</div>`;
    return;
  }
  const hist = $("#hist");
  hist.innerHTML = "";
  if (!items.length) {
    hist.innerHTML = `<div class="hist-item" style="color:var(--muted)">暂无</div>`;
    return;
  }
  for (const a of items) {
    const row = document.createElement("div");
    row.className = "hist-item";
    const sev = a.type === "cancel" ? "cancel" : a.severity;
    const badge = document.createElement("span");
    badge.className = "badge b-" + sev;
    badge.textContent = sev;
    const title = document.createElement("span");
    title.className = "hist-title";
    title.textContent = a.type === "cancel" ? `解除 ${(a.cancels || "").slice(0, 8)}…` : a.title;
    const time = document.createElement("span");
    time.className = "hist-time";
    time.textContent = new Date(a.issued_at * 1000).toLocaleTimeString();
    row.append(badge, title, time);
    if (a.type !== "cancel") {
      const btn = document.createElement("button");
      btn.className = "link";
      btn.textContent = "解除";
      btn.onclick = () => cancelAlert(a.id);
      row.append(btn);
    }
    hist.append(row);
  }
}

$("#refresh").addEventListener("click", loadHistory);
loadHistory();
