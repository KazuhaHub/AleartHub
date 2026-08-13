import { useEffect, useState } from "react";
import {
  Box, Card, CardContent, Chip, List, ListItem, ListItemText, Typography,
} from "@mui/material";
import FavoriteIcon from "@mui/icons-material/MonitorHeart";
import DevicesIcon from "@mui/icons-material/Devices";
import OutboxIcon from "@mui/icons-material/Outbox";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import { api, type Alert, type Device, type DeliveryStats } from "@/api";

function Metric({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card sx={{ height: "100%" }}>
      <CardContent>
        <Typography variant="caption" color="text.secondary">{label}</Typography>
        <Typography variant="h4" fontWeight={500} sx={{ my: 0.5 }}>{value}</Typography>
        {hint && <Typography variant="caption" color="text.secondary">{hint}</Typography>}
      </CardContent>
    </Card>
  );
}

const sevColor: Record<string, "default" | "info" | "warning" | "error"> = {
  notice: "info", warning: "warning", critical: "error", emergency: "error",
};

export default function DashboardView() {
  const { t } = useTranslation();
  const [history, setHistory] = useState<Alert[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [maxSkew, setMaxSkew] = useState<number>(120);
  const [delivery, setDelivery] = useState<DeliveryStats | null>(null);

  useEffect(() => {
    const load = () => {
      api.history().then(setHistory).catch(() => {});
      api.devices().then(setDevices).catch(() => {});
      api.deliveryStats().then(setDelivery).catch(() => {});
    };
    load();
    api.pubkey().then((c) => setMaxSkew(c.max_skew)).catch(() => {});
    const id = setInterval(load, 5000); // live-ish roster
    return () => clearInterval(id);
  }, []);

  const now = Math.floor(Date.now() / 1000);
  const active = history.filter((a) => a.type === "alert" && now <= a.issued_at + a.ttl).length;
  const online = devices.filter((d) => d.state === "online").length;
  const dc = delivery?.counts ?? {};

  return (
    <Box>
      <PageHeader title={t("dash.title")} subtitle={t("dash.subtitle")} />
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)" }, gap: 2, mb: 3 }}>
        <Metric label="生效中警报" value={String(active)} />
        <Metric label="在线设备" value={`${online}/${devices.length}`} hint="fail-loud roster" />
        <Metric label="历史记录" value={String(history.length)} />
        <Metric label="MaxSkew" value={`${maxSkew}s`} hint="防重放窗口" />
      </Box>

      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2, alignItems: "start" }}>
        <Card>
          <CardContent>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
              <DevicesIcon fontSize="small" color="primary" />
              <Typography variant="subtitle1" fontWeight={500}>设备名册</Typography>
            </Box>
            {devices.length === 0 ? (
              <Typography variant="body2" color="text.secondary">暂无设备上报（启用客户端后出现）</Typography>
            ) : (
              <List dense>
                {devices.map((d) => (
                  <ListItem key={d.device_id} secondaryAction={
                    <Typography variant="caption" color="text.secondary">
                      {new Date(d.last_seen * 1000).toLocaleTimeString()}
                    </Typography>
                  }>
                    <Box sx={{ width: 9, height: 9, borderRadius: "50%", mr: 1.5,
                      bgcolor: d.state === "online" ? "success.main" : "text.disabled" }} />
                    <ListItemText primary={d.device_id} secondary={d.client || d.state} />
                  </ListItem>
                ))}
              </List>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
              <FavoriteIcon fontSize="small" color="primary" />
              <Typography variant="subtitle1" fontWeight={500}>最近警报</Typography>
            </Box>
            {history.length === 0 ? (
              <Typography variant="body2" color="text.secondary">暂无</Typography>
            ) : (
              <List dense>
                {history.slice(0, 10).map((a) => (
                  <ListItem key={a.id} secondaryAction={
                    <Typography variant="caption" color="text.secondary">
                      {new Date(a.issued_at * 1000).toLocaleTimeString()}
                    </Typography>
                  }>
                    <Chip size="small" label={a.type === "cancel" ? "cancel" : a.severity}
                      color={a.type === "cancel" ? "default" : sevColor[a.severity]} sx={{ mr: 1.5, minWidth: 78 }} />
                    <ListItemText primary={a.type === "cancel" ? `解除 ${a.cancels.slice(0, 8)}…` : a.title} />
                  </ListItem>
                ))}
              </List>
            )}
          </CardContent>
        </Card>
      </Box>

      <Card sx={{ mt: 2 }}>
        <CardContent>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
            <OutboxIcon fontSize="small" color="primary" />
            <Typography variant="subtitle1" fontWeight={500}>投递健康</Typography>
            <Box sx={{ flex: 1 }} />
            <Chip size="small" color="success" variant="outlined" label={`已送达 ${dc.sent ?? 0}`} />
            <Chip size="small" variant="outlined" label={`待投递 ${dc.pending ?? 0}`} />
            <Chip size="small" color={(dc.dead ?? 0) > 0 ? "error" : "default"} variant={(dc.dead ?? 0) > 0 ? "filled" : "outlined"} label={`失败 ${dc.dead ?? 0}`} />
          </Box>
          {!delivery || delivery.dead.length === 0 ? (
            <Typography variant="body2" color="text.secondary">无失败投递（耐久化 outbox：重试 + 退避 + 死信告警）</Typography>
          ) : (
            <List dense>
              {delivery.dead.map((d, i) => (
                <ListItem key={`${d.alert_id}-${d.channel}-${i}`} secondaryAction={
                  <Typography variant="caption" color="text.secondary">
                    {new Date(d.updated_at * 1000).toLocaleTimeString()}
                  </Typography>
                }>
                  <Chip size="small" color="error" label={d.channel} sx={{ mr: 1.5, minWidth: 78 }} />
                  <ListItemText
                    primary={`${d.target} · ${d.attempts} 次尝试`}
                    secondary={d.last_error}
                  />
                </ListItem>
              ))}
            </List>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}
