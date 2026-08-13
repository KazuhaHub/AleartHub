import { useEffect, useState } from "react";
import {
  Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography,
} from "@mui/material";
import { QRCodeSVG } from "qrcode.react";
import { useTranslation } from "react-i18next";
import { authApi } from "@/api";

type Stage = "loading" | "enroll" | "recovery" | "enabled";

export default function TwoFADialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const [stage, setStage] = useState<Stage>("loading");
  const [otpauth, setOtpauth] = useState("");
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setStage("loading");
    setErr(""); setCode(""); setRecovery([]);
    authApi.twofa.status()
      .then((s) => {
        if (s.enabled) { setStage("enabled"); return; }
        return authApi.twofa.begin().then((b) => {
          setOtpauth(b.otpauth_url); setSecret(b.secret); setStage("enroll");
        });
      })
      .catch(() => setErr(t("twofa.error")));
  }, [open, t]);

  const enable = async () => {
    setBusy(true); setErr("");
    try {
      const r = await authApi.twofa.enable(code.trim());
      setRecovery(r.recovery_codes); setStage("recovery");
    } catch { setErr(t("twofa.badCode")); } finally { setBusy(false); }
  };
  const disable = async () => {
    setBusy(true); setErr("");
    try { await authApi.twofa.disable(code.trim()); onClose(); }
    catch { setErr(t("twofa.badCode")); } finally { setBusy(false); }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{t("twofa.title")}</DialogTitle>
      <DialogContent>
        {stage === "loading" && <Typography color="text.secondary">…</Typography>}

        {stage === "enroll" && (
          <Stack spacing={2} alignItems="center">
            <Typography variant="body2" color="text.secondary">{t("twofa.scan")}</Typography>
            {otpauth && <Box sx={{ bgcolor: "#fff", p: 1.5, borderRadius: 2 }}><QRCodeSVG value={otpauth} size={176} /></Box>}
            <Typography variant="caption" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>{secret}</Typography>
            <TextField label={t("twofa.code")} value={code} onChange={(e) => setCode(e.target.value)} fullWidth
              inputProps={{ inputMode: "numeric", autoComplete: "one-time-code" }} />
            {err && <Alert severity="error" sx={{ width: "100%" }}>{err}</Alert>}
          </Stack>
        )}

        {stage === "recovery" && (
          <Stack spacing={1.5}>
            <Alert severity="success">{t("twofa.enabledOk")}</Alert>
            <Typography variant="body2">{t("twofa.saveRecovery")}</Typography>
            <Box sx={{ fontFamily: "monospace", bgcolor: "action.hover", p: 1.5, borderRadius: 1, columns: 2 }}>
              {recovery.map((c) => <div key={c}>{c}</div>)}
            </Box>
          </Stack>
        )}

        {stage === "enabled" && (
          <Stack spacing={2}>
            <Alert severity="info">{t("twofa.alreadyEnabled")}</Alert>
            <TextField label={t("twofa.code")} value={code} onChange={(e) => setCode(e.target.value)} fullWidth
              inputProps={{ inputMode: "numeric" }} />
            {err && <Alert severity="error">{err}</Alert>}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{stage === "recovery" ? t("twofa.done") : t("twofa.cancel")}</Button>
        {stage === "enroll" && <Button variant="contained" onClick={enable} disabled={busy || !code}>{t("twofa.enable")}</Button>}
        {stage === "enabled" && <Button color="error" variant="contained" onClick={disable} disabled={busy || !code}>{t("twofa.disable")}</Button>}
      </DialogActions>
    </Dialog>
  );
}
