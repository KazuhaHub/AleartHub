import { useEffect, useState } from "react";
import {
  Box, Button, Card, CardContent, Chip, IconButton, List, ListItem, ListItemText,
  MenuItem, Snackbar, Stack, TextField, Typography,
} from "@mui/material";
import SendIcon from "@mui/icons-material/Send";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import { api, type Alert, type Category, type PublishReq, type Severity } from "@/api";

const SEVERITIES: Severity[] = ["notice", "warning", "critical", "emergency"];
const CATEGORIES: Category[] = ["earthquake", "fire", "weather", "system", "security", "custom"];

// Scenario templates (Alertus borrow): one tap fills the form.
const SCENARIOS: { label: string; v: PublishReq }[] = [
  { label: "🌐 地震 emergency", v: { severity: "emergency", category: "earthquake", title: "正在发生地震", body: "震中距你约 42 公里，预计 15 秒后到达。", action: "趴下，掩护，抓牢" } },
  { label: "🔥 火灾 emergency", v: { severity: "emergency", category: "fire", title: "检测到火警", body: "厨房烟感触发，请立即确认。", action: "立即撤离，不要乘电梯" } },
  { label: "🚪 全家撤离", v: { severity: "critical", category: "custom", title: "全家立即撤离", body: "请按预案到集合点。", action: "保持冷静，迅速撤离" } },
  { label: "🖥 节点失联 critical", v: { severity: "critical", category: "system", title: "节点失联：pve-01", body: "心跳超时 90 秒。", action: "确认并检查节点" } },
  { label: "🔒 异地登录 notice", v: { severity: "notice", category: "security", title: "异地登录", body: "来自 Osaka 的新登录。", action: "核实并确认" } },
];

const chipColor = (a: Alert) =>
  a.type === "cancel" ? "default" : a.severity === "notice" ? "info" : a.severity === "warning" ? "warning" : "error";

export default function PublishView() {
  const { t } = useTranslation();
  const [form, setForm] = useState<PublishReq>({
    severity: "emergency", category: "earthquake",
    title: "正在发生地震", body: "震中距你约 42 公里，预计 15 秒后到达。", action: "",
  });
  const [ttl, setTtl] = useState("");
  const [history, setHistory] = useState<Alert[]>([]);
  const [snack, setSnack] = useState("");

  const load = () => api.history().then(setHistory).catch(() => {});
  useEffect(() => { load(); }, []);

  const set = (k: keyof PublishReq, v: string) => setForm((f) => ({ ...f, [k]: v }));

  const send = async () => {
    try {
      const req: PublishReq = { ...form };
      const n = parseInt(ttl, 10);
      if (!isNaN(n) && n > 0) req.ttl = n;
      const a = await api.publish(req);
      setSnack(`已发布 ${a.severity}/${a.category} · ${a.id.slice(0, 8)}…`);
      load();
    } catch (e: any) {
      setSnack("发布失败：" + (e?.response?.data || e?.message || e));
    }
  };

  const cancel = async (id: string) => {
    try { await api.cancel(id); setSnack("已解除 " + id.slice(0, 8) + "…"); load(); }
    catch (e: any) { setSnack("解除失败：" + (e?.message || e)); }
  };

  return (
    <Box>
      <PageHeader title={t("publish.title")} subtitle={t("publish.subtitle")} />
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "7fr 5fr" }, gap: 2, alignItems: "start" }}>
        <Card>
          <CardContent>
            <Stack direction="row" sx={{ flexWrap: "wrap", gap: 1, mb: 2 }}>
              {SCENARIOS.map((s) => (
                <Chip key={s.label} label={s.label} onClick={() => { setForm(s.v); setTtl(""); }} variant="outlined" />
              ))}
            </Stack>
            <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
              <TextField select fullWidth label="severity" value={form.severity} onChange={(e) => set("severity", e.target.value)}>
                {SEVERITIES.map((s) => <MenuItem key={s} value={s}>{s}</MenuItem>)}
              </TextField>
              <TextField select fullWidth label="category" value={form.category} onChange={(e) => set("category", e.target.value)}>
                {CATEGORIES.map((c) => <MenuItem key={c} value={c}>{c}</MenuItem>)}
              </TextField>
            </Box>
            <Stack spacing={2} sx={{ mt: 2 }}>
              <TextField fullWidth label="标题 title" value={form.title} onChange={(e) => set("title", e.target.value)} />
              <TextField fullWidth multiline minRows={2} label="正文 body" value={form.body} onChange={(e) => set("body", e.target.value)} />
              <TextField fullWidth label="处置 action（留空按类别自动填）" value={form.action} onChange={(e) => set("action", e.target.value)} />
              <TextField label="ttl 秒（留空按默认）" value={ttl} onChange={(e) => setTtl(e.target.value)} sx={{ maxWidth: 240 }} />
            </Stack>
            <Box sx={{ mt: 2 }}>
              <Button variant="contained" size="large" startIcon={<SendIcon />} onClick={send}>发送警报</Button>
            </Box>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <Typography variant="subtitle1" fontWeight={500} sx={{ mb: 1 }}>历史 / 当前</Typography>
            {history.length === 0 ? <Typography variant="body2" color="text.secondary">暂无</Typography> : (
              <List dense>
                {history.slice(0, 12).map((a) => (
                  <ListItem key={a.id} secondaryAction={
                    a.type !== "cancel" ? <IconButton size="small" onClick={() => cancel(a.id)}><span style={{ fontSize: 13 }}>解除</span></IconButton> : null
                  }>
                    <Chip size="small" sx={{ mr: 1, minWidth: 70 }} label={a.type === "cancel" ? "cancel" : a.severity} color={chipColor(a)} />
                    <ListItemText primary={a.type === "cancel" ? `解除 ${a.cancels.slice(0, 8)}…` : a.title}
                      secondary={new Date(a.issued_at * 1000).toLocaleTimeString()} />
                  </ListItem>
                ))}
              </List>
            )}
          </CardContent>
        </Card>
      </Box>
      <Snackbar open={!!snack} autoHideDuration={4000} onClose={() => setSnack("")} message={snack} />
    </Box>
  );
}
