import { startAuthentication, startRegistration } from "@simplewebauthn/browser";
import axios from "axios";
import { http, tokens, type AuthUser } from "@/api";

// Usernameless (discoverable) passkey login → stores session tokens, returns user.
export async function passkeyLogin(): Promise<AuthUser> {
  const begin = (await axios.post("/api/auth/passkey/login/begin")).data;
  const assertion = await startAuthentication({ optionsJSON: begin.options.publicKey });
  const res = await axios.post(
    `/api/auth/passkey/login/finish?session=${encodeURIComponent(begin.session)}`,
    assertion,
  );
  tokens.set(res.data.access_token, res.data.refresh_token);
  return res.data.user as AuthUser;
}

// Register a new passkey for the currently logged-in user.
export async function passkeyRegister(name: string): Promise<void> {
  const begin = (await http.post("/api/auth/passkey/register/begin")).data;
  const attestation = await startRegistration({ optionsJSON: begin.options.publicKey });
  await http.post(
    `/api/auth/passkey/register/finish?session=${encodeURIComponent(begin.session)}&name=${encodeURIComponent(name)}`,
    attestation,
  );
}
