import { useEffect, useState } from "react";
import {
  Alert, Box, Button, Card, CardContent, Chip, Dialog, DialogActions, DialogContent, DialogTitle,
  IconButton, List, ListItem, ListItemText, Snackbar, Stack, TextField, Typography,
} from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import ShieldIcon from "@mui/icons-material/Shield";
import FingerprintIcon from "@mui/icons-material/Fingerprint";
import KeyIcon from "@mui/icons-material/VpnKey";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import TwoFADialog from "@/components/TwoFADialog";
import { adminApi, authApi, passkeyApi, type PasskeyInfo, type ServiceAccount } from "@/api";
import { passkeyRegister } from "@/passkey";

export default function SettingsView() {
  const { t } = useTranslation();
  const [snack, setSnack] = useState("");
  const [twofaOpen, setTwofaOpen] = useState(false);
  const [twofaOn, setTwofaOn] = useState(false);
  const [passkeys, setPasskeys] = useState<PasskeyInfo[]>([]);
  const [keys, setKeys] = useState<ServiceAccount[]>([]);
  const [newKeyOpen, setNewKeyOpen] = useState(false);
  const [newKeyName, setNewKeyName] = useState("");
  const [issuedToken, setIssuedToken] = useState("");

  const refresh = () => {
    authApi.twofa.status().then((s) => setTwofaOn(s.enabled)).catch(() => {});
    passkeyApi.list().then(setPasskeys).catch(() => {});
    adminApi.saList().then(setKeys).catch(() => {});
  };
  useEffect(refresh, []);

  const addPasskey = async () => {
    const name = window.prompt(t("passkey.namePrompt"), "My passkey");
    if (name === null) return;
    try { await passkeyRegister(name || "Passkey"); setSnack(t("passkey.added")); refresh(); }
    catch { setSnack(t("passkey.failed")); }
  };
  const delPasskey = async (id: number) => { await passkeyApi.delete(id); refresh(); };

  const createKey = async () => {
    try {
      const r = await adminApi.saCreate(newKeyName.trim() || "API key", ["alerts:ingest"]);
      setIssuedToken(r.token);
      setNewKeyName("");
      setNewKeyOpen(false);
      refresh();
    } catch { setSnack(t("settings.keyCreateFailed")); }
  };
  const delKey = async (id: number) => { await adminApi.saDelete(id); refresh(); };

  return (
    <Box>
      <PageHeader title={t("nav.settings")} subtitle={t("settings.subtitle")} />
      <Stack spacing={2} sx={{ maxWidth: 760 }}>
        {/* Security */}
        <Card>
          <CardContent>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
              <ShieldIcon color="primary" /><Typography variant="subtitle1" fontWeight={500}>{t("settings.security")}</Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 2, mb: 2 }}>
              <Typography sx={{ flex: 1 }}>{t("twofa.menu")}</Typography>
              <Chip size="small" label={twofaOn ? t("settings.on") : t("settings.off")} color={twofaOn ? "success" : "default"} />
              <Button variant="outlined" onClick={() => setTwofaOpen(true)}>{t("settings.manage")}</Button>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 1 }}>
              <FingerprintIcon fontSize="small" /><Typography sx={{ flex: 1 }}>Passkeys</Typography>
              <Button size="small" variant="outlined" onClick={addPasskey}>{t("passkey.add")}</Button>
            </Box>
            <List dense>
              {passkeys.length === 0 && <ListItem><ListItemText secondary={t("settings.none")} /></ListItem>}
              {passkeys.map((p) => (
                <ListItem key={p.id} secondaryAction={<IconButton edge="end" size="small" onClick={() => delPasskey(p.id)}><DeleteIcon fontSize="small" /></IconButton>}>
                  <ListItemText primary={p.name} secondary={new Date(p.created_at * 1000).toLocaleString()} />
                </ListItem>
              ))}
            </List>
          </CardContent>
        </Card>

        {/* Integrations / API keys */}
        <Card>
          <CardContent>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
              <KeyIcon color="primary" /><Typography variant="subtitle1" fontWeight={500}>{t("settings.integrations")}</Typography>
              <Box sx={{ flex: 1 }} />
              <Button variant="contained" onClick={() => setNewKeyOpen(true)}>{t("settings.newKey")}</Button>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>{t("settings.integrationsHint")}</Typography>
            <List dense>
              {keys.length === 0 && <ListItem><ListItemText secondary={t("settings.none")} /></ListItem>}
              {keys.map((k) => (
                <ListItem key={k.id} secondaryAction={<IconButton edge="end" size="small" onClick={() => delKey(k.id)}><DeleteIcon fontSize="small" /></IconButton>}>
                  <ListItemText
                    primary={<>{k.name} {k.scopes.map((s) => <Chip key={s} size="small" label={s} sx={{ ml: 0.5 }} />)}</>}
                    secondary={k.last_used_at ? `${t("settings.lastUsed")} ${new Date(k.last_used_at * 1000).toLocaleString()}` : t("settings.neverUsed")}
                  />
                </ListItem>
              ))}
            </List>
          </CardContent>
        </Card>
      </Stack>

      <TwoFADialog open={twofaOpen} onClose={() => { setTwofaOpen(false); refresh(); }} />

      {/* create-key dialog */}
      <Dialog open={newKeyOpen} onClose={() => setNewKeyOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>{t("settings.newKey")}</DialogTitle>
        <DialogContent>
          <TextField autoFocus fullWidth label={t("settings.keyName")} value={newKeyName} onChange={(e) => setNewKeyName(e.target.value)} sx={{ mt: 1 }} />
          <Typography variant="caption" color="text.secondary">scope: alerts:ingest</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewKeyOpen(false)}>{t("twofa.cancel")}</Button>
          <Button variant="contained" onClick={createKey}>{t("settings.create")}</Button>
        </DialogActions>
      </Dialog>

      {/* show-token-once dialog */}
      <Dialog open={!!issuedToken} onClose={() => setIssuedToken("")} maxWidth="sm" fullWidth>
        <DialogTitle>{t("settings.keyCreated")}</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>{t("settings.tokenOnce")}</Alert>
          <Box sx={{ fontFamily: "monospace", bgcolor: "action.hover", p: 1.5, borderRadius: 1, wordBreak: "break-all" }}>{issuedToken}</Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { navigator.clipboard?.writeText(issuedToken); setSnack(t("settings.copied")); }}>{t("settings.copy")}</Button>
          <Button variant="contained" onClick={() => setIssuedToken("")}>{t("twofa.done")}</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={!!snack} autoHideDuration={3000} onClose={() => setSnack("")} message={snack} />
    </Box>
  );
}
