import axios from "axios";

// --- token storage (session JWTs from the admin login) ---------------------
const ACCESS_KEY = "alerthub-access";
const REFRESH_KEY = "alerthub-refresh";

export const tokens = {
  access: () => localStorage.getItem(ACCESS_KEY) || "",
  refresh: () => localStorage.getItem(REFRESH_KEY) || "",
  set(access: string, refresh: string) {
    localStorage.setItem(ACCESS_KEY, access);
    localStorage.setItem(REFRESH_KEY, refresh);
  },
  clear() {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
  },
};

const ORG_KEY = "alerthub-org";
export const activeOrg = {
  get: () => localStorage.getItem(ORG_KEY) || "",
  set: (id: string) => localStorage.setItem(ORG_KEY, id),
};

export const http = axios.create({ baseURL: "" });

http.interceptors.request.use((cfg) => {
  const t = tokens.access();
  if (t) cfg.headers.Authorization = "Bearer " + t;
  const org = activeOrg.get();
  if (org) cfg.headers["X-Org-Id"] = org;
  return cfg;
});

// Silent refresh on 401 (single-flight), then retry once; else bounce to login.
let refreshing: Promise<boolean> | null = null;
async function doRefresh(): Promise<boolean> {
  const r = tokens.refresh();
  if (!r) return false;
  try {
    const res = await axios.post("/api/auth/refresh", { refresh_token: r });
    tokens.set(res.data.access_token, res.data.refresh_token);
    return true;
  } catch {
    return false;
  }
}
http.interceptors.response.use(
  (res) => res,
  async (err) => {
    const cfg = err.config || {};
    if (err.response?.status === 401 && !cfg._retried) {
      cfg._retried = true;
      refreshing = refreshing || doRefresh();
      const ok = await refreshing;
      refreshing = null;
      if (ok) {
        cfg.headers.Authorization = "Bearer " + tokens.access();
        return http(cfg);
      }
      tokens.clear();
      if (!location.pathname.endsWith("/login")) location.assign("/admin/login");
    }
    return Promise.reject(err);
  },
);

// --- auth API --------------------------------------------------------------
export type AuthUser = { id: number; upn: string; role: string; email?: string };

export type TokenResp = { access_token: string; refresh_token: string; user: AuthUser };
export type LoginResult = TokenResp | { status: "2fa_required"; pending_token: string; methods: string[] };

export const authApi = {
  methods: () => http.get("/api/auth/methods").then((x) => x.data),
  login: (upn: string, password: string) =>
    axios.post("/api/auth/login", { upn, password }).then((x) => x.data as LoginResult),
  verify2FA: (pendingToken: string, code: string) =>
    axios.post("/api/auth/2fa/verify", { pending_token: pendingToken, code }).then((x) => x.data as TokenResp),
  oidcExchange: (code: string) =>
    axios.post("/api/auth/oidc/exchange", { code }).then((x) => x.data as TokenResp),
  me: () => http.get<AuthUser>("/api/auth/me").then((x) => x.data),
  logout: () => http.post("/api/auth/logout").catch(() => {}),
  twofa: {
    status: () => http.get<{ enabled: boolean }>("/api/auth/2fa/status").then((x) => x.data),
    begin: () => http.post<{ otpauth_url: string; secret: string }>("/api/auth/2fa/begin").then((x) => x.data),
    enable: (code: string) => http.post<{ recovery_codes: string[] }>("/api/auth/2fa/enable", { code }).then((x) => x.data),
    disable: (code: string) => http.post("/api/auth/2fa/disable", { code }).then((x) => x.data),
  },
};

// --- admin: service accounts (API keys) ------------------------------------
export type ServiceAccount = {
  id: number; name: string; scopes: string[]; disabled: boolean; created_at: number; last_used_at: number;
};
export const adminApi = {
  saList: () => http.get<ServiceAccount[]>("/api/admin/service-accounts").then((x) => x.data ?? []),
  saCreate: (name: string, scopes: string[]) =>
    http.post<{ id: number; name: string; scopes: string[]; token: string }>(
      "/api/admin/service-accounts", { name, scopes }).then((x) => x.data),
  saDelete: (id: number) => http.post("/api/admin/service-accounts/delete", { id }),
};

// --- passkeys (self-service) ----------------------------------------------
export type PasskeyInfo = { id: number; name: string; created_at: number; last_used_at: number };
export const passkeyApi = {
  list: () => http.get<PasskeyInfo[]>("/api/auth/passkey/list").then((x) => x.data ?? []),
  delete: (id: number) => http.post("/api/auth/passkey/delete", { id }),
};

// --- alert API -------------------------------------------------------------
export type Severity = "notice" | "warning" | "critical" | "emergency";
export type Category = "earthquake" | "fire" | "weather" | "system" | "security" | "custom";

export type Alert = {
  id: string; type: string; category: Category; severity: Severity;
  title: string; body: string; action: string; source: string;
  issued_at: number; ttl: number; cancels: string;
};

export type PublishReq = {
  severity: Severity; category: Category; title: string; body: string; action: string; ttl?: number;
};

export type Device = { device_id: string; state: string; at: number; client: string; last_seen: number };

export type OrgInfo = { id: number; slug: string; name: string };

export type AuditEntry = {
  id: number; org_id: number; at: number;
  actor_type: "user" | "service_account" | "admin_token" | "system";
  actor_id: number; actor_name: string; action: string;
  target_type: string; target_id: string; detail: string; ip: string;
  prev_hash: string; hash: string;
};
export type AuditChain = { ok: boolean; entries: number; bad_id?: number; reason?: string };

export type DeadDelivery = {
  alert_id: string; channel: string; target: string; attempts: number; last_error: string; updated_at: number;
};
export type DeliveryStats = { counts: Record<string, number>; dead: DeadDelivery[] };

export const api = {
  orgs: () => http.get<OrgInfo[]>("/api/orgs").then((x) => x.data ?? []),
  deliveryStats: () => http.get<DeliveryStats>("/api/delivery/stats").then((x) => x.data),
  publish: (r: PublishReq) => http.post<Alert>("/api/publish", r).then((x) => x.data),
  cancel: (id: string) => http.post("/api/cancel", { id }).then((x) => x.data),
  history: () => http.get<Alert[]>("/api/history").then((x) => x.data ?? []),
  devices: () => http.get<Device[]>("/api/devices").then((x) => x.data ?? []),
  audit: (limit = 200) => http.get<AuditEntry[]>(`/api/audit?limit=${limit}`).then((x) => x.data ?? []),
  auditVerify: () => http.get<AuditChain>("/api/audit/verify").then((x) => x.data),
  pubkey: () => http.get<{ pubkey: string; max_skew: number; ws_port: string }>("/pubkey").then((x) => x.data),
};
