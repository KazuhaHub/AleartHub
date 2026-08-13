import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Alert, Box, CircularProgress } from "@mui/material";
import { useTranslation } from "react-i18next";
import { authApi } from "@/api";
import { useAuth } from "@/stores/auth";

// /admin/sso?code=… — the OIDC callback redirected here; swap the one-time code
// for tokens and enter the app.
export default function SSOCallback() {
  const nav = useNavigate();
  const [params] = useSearchParams();
  const setSession = useAuth((s) => s.setSession);
  const { t } = useTranslation();
  const [err, setErr] = useState("");

  useEffect(() => {
    const code = params.get("code");
    if (!code) { nav("/login", { replace: true }); return; }
    authApi.oidcExchange(code)
      .then((res) => { setSession(res); nav("/", { replace: true }); })
      .catch(() => { setErr(t("login.ssoError")); setTimeout(() => nav("/login", { replace: true }), 1500); });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <Box sx={{ height: "100vh", display: "flex", alignItems: "center", justifyContent: "center" }}>
      {err ? <Alert severity="error">{err}</Alert> : <CircularProgress />}
    </Box>
  );
}
