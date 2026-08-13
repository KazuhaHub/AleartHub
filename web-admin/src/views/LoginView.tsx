import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Box, Button, Card, CardContent, Divider, Stack, TextField, Typography } from "@mui/material";
import FingerprintIcon from "@mui/icons-material/Fingerprint";
import LoginIcon from "@mui/icons-material/Login";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/stores/auth";
import { authApi } from "@/api";

export default function LoginView() {
  const { t } = useTranslation();
  const nav = useNavigate();
  const login = useAuth((s) => s.login);
  const complete2FA = useAuth((s) => s.complete2FA);
  const loginPasskey = useAuth((s) => s.loginPasskey);
  const [upn, setUpn] = useState("admin");
  const [pw, setPw] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [stage, setStage] = useState<"login" | "2fa">("login");
  const [pending, setPending] = useState("");
  const [code, setCode] = useState("");
  const [ssoUrl, setSsoUrl] = useState("");

  useEffect(() => {
    authApi.methods().then((m) => {
      if (m.oidc) setSsoUrl("/api/auth/oidc/login");
      else if (m.saml) setSsoUrl("/api/auth/saml/login");
    }).catch(() => {});
  }, []);

  const passkey = async () => {
    setErr("");
    setBusy(true);
    try {
      await loginPasskey();
      nav("/", { replace: true });
    } catch {
      setErr(t("login.passkeyError"));
    } finally {
      setBusy(false);
    }
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      const r = await login(upn, pw);
      if (r.twoFA) {
        setPending(r.pending!);
        setStage("2fa");
      } else {
        nav("/", { replace: true });
      }
    } catch {
      setErr(t("login.error"));
    } finally {
      setBusy(false);
    }
  };

  const verify = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr("");
    setBusy(true);
    try {
      await complete2FA(pending, code.trim());
      nav("/", { replace: true });
    } catch {
      setErr(t("login.2faError"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Box sx={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", bgcolor: "background.default", p: 2 }}>
      <Card sx={{ width: 380, maxWidth: "100%" }}>
        <CardContent sx={{ p: 4 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 0.5 }}>
            <Box sx={{ width: 28, height: 28, borderRadius: "50%", bgcolor: "error.main" }} />
            <Typography variant="h5" fontWeight={500}>AlertHub</Typography>
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            {stage === "2fa" ? t("login.2faSubtitle") : t("login.subtitle")}
          </Typography>
          {stage === "2fa" ? (
            <form onSubmit={verify}>
              <Stack spacing={2}>
                <TextField label={t("login.2faCode")} value={code} onChange={(e) => setCode(e.target.value)}
                  autoFocus fullWidth inputProps={{ inputMode: "numeric", autoComplete: "one-time-code" }} />
                {err && <Alert severity="error">{err}</Alert>}
                <Button type="submit" variant="contained" size="large" disabled={busy || !code}>{t("login.verify")}</Button>
                <Button variant="text" size="small" onClick={() => { setStage("login"); setErr(""); setCode(""); }}>{t("login.back")}</Button>
              </Stack>
            </form>
          ) : (
            <>
              <form onSubmit={submit}>
                <Stack spacing={2}>
                  <TextField label={t("login.upn")} value={upn} onChange={(e) => setUpn(e.target.value)} autoFocus fullWidth />
                  <TextField label={t("login.password")} type="password" value={pw} onChange={(e) => setPw(e.target.value)} fullWidth />
                  {err && <Alert severity="error">{err}</Alert>}
                  <Button type="submit" variant="contained" size="large" disabled={busy || !pw}>{t("login.submit")}</Button>
                </Stack>
              </form>
              <Divider sx={{ my: 2 }}>{t("login.or")}</Divider>
              <Button fullWidth variant="outlined" size="large" startIcon={<FingerprintIcon />} disabled={busy} onClick={passkey}>
                {t("login.passkey")}
              </Button>
              {ssoUrl && (
                <Button fullWidth variant="outlined" size="large" sx={{ mt: 1 }} startIcon={<LoginIcon />}
                  onClick={() => { window.location.href = ssoUrl; }}>
                  {t("login.sso")}
                </Button>
              )}
            </>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}
