import { create } from "zustand";
import { authApi, tokens, type AuthUser, type TokenResp } from "@/api";
import { passkeyLogin } from "@/passkey";

type S = {
  user: AuthUser | null;
  ready: boolean; // initial session check finished
  authed: boolean;
  login: (upn: string, password: string) => Promise<{ twoFA: boolean; pending?: string }>;
  complete2FA: (pending: string, code: string) => Promise<void>;
  loginPasskey: () => Promise<void>;
  setSession: (res: TokenResp) => void;
  logout: () => Promise<void>;
  init: () => Promise<void>;
};

export const useAuth = create<S>((set) => ({
  user: null,
  ready: false,
  authed: !!tokens.access(),
  async login(upn, password) {
    const res = await authApi.login(upn, password);
    if ("status" in res && res.status === "2fa_required") {
      return { twoFA: true, pending: res.pending_token };
    }
    tokens.set(res.access_token, res.refresh_token);
    set({ user: res.user, authed: true });
    return { twoFA: false };
  },
  async complete2FA(pending, code) {
    const res = await authApi.verify2FA(pending, code);
    tokens.set(res.access_token, res.refresh_token);
    set({ user: res.user, authed: true });
  },
  async loginPasskey() {
    const u = await passkeyLogin();
    set({ user: u, authed: true });
  },
  setSession(res) {
    tokens.set(res.access_token, res.refresh_token);
    set({ user: res.user, authed: true });
  },
  async logout() {
    await authApi.logout();
    tokens.clear();
    set({ user: null, authed: false });
  },
  async init() {
    if (!tokens.access()) {
      set({ ready: true, authed: false });
      return;
    }
    try {
      const u = await authApi.me();
      set({ user: u, authed: true, ready: true });
    } catch {
      tokens.clear();
      set({ user: null, authed: false, ready: true });
    }
  },
}));
