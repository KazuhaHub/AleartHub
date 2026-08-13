import { useEffect, useState } from "react";
import { Card, Col, List, Row, Statistic, Tag, Typography, theme } from "antd";
import { AlertOutlined, MobileOutlined, SendOutlined } from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import { api, type Alert, type Device, type DeliveryStats } from "@/api";

const { Text } = Typography;

function Metric({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card style={{ height: "100%" }} styles={{ body: { padding: 16 } }}>
      <Statistic
        title={label}
        value={value}
        // antd groups thousands with "," by default; the old MUI markup printed
        // the raw String(value), so keep it ungrouped.
        groupSeparator=""
        styles={{
          title: { fontSize: 12 },
          content: { fontSize: 28, fontWeight: 500 },
        }}
      />
      {hint && <Text type="secondary" style={{ fontSize: 12 }}>{hint}</Text>}
    </Card>
  );
}

function SectionTitle({ icon, text, extra }: {
  icon: React.ReactNode; text: string; extra?: React.ReactNode;
}) {
  const { token } = theme.useToken();
  return (
    <div style={{
      display: "flex", alignItems: "center", gap: 8, marginBottom: 12, flexWrap: "wrap",
    }}>
      <span style={{ color: token.colorPrimary, display: "inline-flex", fontSize: 16 }}>{icon}</span>
      <Typography.Title level={5} style={{ margin: 0, fontWeight: 500 }}>{text}</Typography.Title>
      {extra && <div style={{ marginInlineStart: "auto", display: "flex", gap: 8, flexWrap: "wrap" }}>{extra}</div>}
    </div>
  );
}

// Was a MUI Chip `color`; antd Tag status colours are theme-token driven.
const sevColor: Record<string, string> = {
  notice: "processing", warning: "warning", critical: "error", emergency: "error",
};

const TS: React.CSSProperties = { fontSize: 12, flex: "0 0 auto", marginInlineStart: 12 };
const ROW: React.CSSProperties = { paddingInline: 0 };

export default function DashboardView() {
  const { t } = useTranslation();
  const { token } = theme.useToken();
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
    <>
      <PageHeader title={t("dash.title")} subtitle={t("dash.subtitle")} />

      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col xs={12} md={6}><Metric label="生效中警报" value={String(active)} /></Col>
        <Col xs={12} md={6}><Metric label="在线设备" value={`${online}/${devices.length}`} hint="fail-loud roster" /></Col>
        <Col xs={12} md={6}><Metric label="历史记录" value={String(history.length)} /></Col>
        <Col xs={12} md={6}><Metric label="MaxSkew" value={`${maxSkew}s`} hint="防重放窗口" /></Col>
      </Row>

      <Row gutter={[16, 16]} align="top">
        <Col xs={24} md={12}>
          <Card>
            <SectionTitle icon={<MobileOutlined />} text="设备名册" />
            {devices.length === 0 ? (
              <Text type="secondary">暂无设备上报（启用客户端后出现）</Text>
            ) : (
              <List
                size="small"
                split={false}
                dataSource={devices}
                rowKey={(d) => d.device_id}
                renderItem={(d) => (
                  <List.Item style={ROW}>
                    <List.Item.Meta
                      avatar={
                        <span style={{
                          display: "block", width: 9, height: 9, borderRadius: "50%", marginTop: 7,
                          background: d.state === "online" ? token.colorSuccess : token.colorTextDisabled,
                        }} />
                      }
                      title={<span style={{ fontWeight: 400 }}>{d.device_id}</span>}
                      description={<Text type="secondary" style={{ fontSize: 12 }}>{d.client || d.state}</Text>}
                    />
                    <Text type="secondary" style={TS}>
                      {new Date(d.last_seen * 1000).toLocaleTimeString()}
                    </Text>
                  </List.Item>
                )}
              />
            )}
          </Card>
        </Col>

        <Col xs={24} md={12}>
          <Card>
            <SectionTitle icon={<AlertOutlined />} text="最近警报" />
            {history.length === 0 ? (
              <Text type="secondary">暂无</Text>
            ) : (
              <List
                size="small"
                split={false}
                dataSource={history.slice(0, 10)}
                rowKey={(a) => a.id}
                renderItem={(a) => (
                  <List.Item style={ROW}>
                    <div style={{ display: "flex", alignItems: "center", gap: 12, minWidth: 0, flex: 1 }}>
                      <Tag
                        color={a.type === "cancel" ? undefined : sevColor[a.severity]}
                        style={{ minWidth: 78, textAlign: "center", marginInlineEnd: 0, flex: "0 0 auto" }}
                      >
                        {a.type === "cancel" ? "cancel" : a.severity}
                      </Tag>
                      <span style={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                        {a.type === "cancel" ? `解除 ${a.cancels.slice(0, 8)}…` : a.title}
                      </span>
                    </div>
                    <Text type="secondary" style={TS}>
                      {new Date(a.issued_at * 1000).toLocaleTimeString()}
                    </Text>
                  </List.Item>
                )}
              />
            )}
          </Card>
        </Col>
      </Row>

      <Card style={{ marginTop: 16 }}>
        <SectionTitle
          icon={<SendOutlined />}
          text="投递健康"
          extra={
            <>
              <Tag color="success" style={{ marginInlineEnd: 0 }}>{`已送达 ${dc.sent ?? 0}`}</Tag>
              <Tag style={{ marginInlineEnd: 0 }}>{`待投递 ${dc.pending ?? 0}`}</Tag>
              <Tag
                color={(dc.dead ?? 0) > 0 ? "error" : undefined}
                style={{ marginInlineEnd: 0 }}
              >{`失败 ${dc.dead ?? 0}`}</Tag>
            </>
          }
        />
        {!delivery || delivery.dead.length === 0 ? (
          <Text type="secondary">无失败投递（耐久化 outbox：重试 + 退避 + 死信告警）</Text>
        ) : (
          <List
            size="small"
            split={false}
            dataSource={delivery.dead}
            renderItem={(d) => (
              <List.Item style={ROW}>
                <div style={{ display: "flex", alignItems: "flex-start", gap: 12, minWidth: 0, flex: 1 }}>
                  <Tag color="error" style={{ minWidth: 78, textAlign: "center", marginInlineEnd: 0, flex: "0 0 auto" }}>
                    {d.channel}
                  </Tag>
                  <div style={{ minWidth: 0 }}>
                    <div style={{ overflow: "hidden", textOverflow: "ellipsis" }}>
                      {`${d.target} · ${d.attempts} 次尝试`}
                    </div>
                    <Text type="secondary" style={{ fontSize: 12 }}>{d.last_error}</Text>
                  </div>
                </div>
                <Text type="secondary" style={TS}>
                  {new Date(d.updated_at * 1000).toLocaleTimeString()}
                </Text>
              </List.Item>
            )}
          />
        )}
      </Card>
    </>
  );
}
