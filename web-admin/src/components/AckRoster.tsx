import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Card, Empty, Space, Spin, Tag, Typography } from "antd";
import { CheckCircleOutlined, ExclamationCircleOutlined, MinusCircleOutlined } from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import { api, type AckRoster as Roster, type EscalationState } from "@/api";

// SPEC-SAFETY §5 roster. The three states are not decoration — each maps to a
// different action for the person reading it:
//   green  online + acknowledged  -> nothing to do
//   RED    online + silent        -> call them / go to them; they can receive and did not answer
//   amber  offline                -> already escalated to the backup channel; go look in person
//
// The red case is the one the whole ladder exists for, so it is listed first and
// is the only one that pulses.
const POLL_MS = 5000;

export default function AckRoster({ alertId, severity }: { alertId: string; severity?: string }) {
  const { t } = useTranslation();
  const [roster, setRoster] = useState<Roster | null>(null);
  const [esc, setEsc] = useState<EscalationState | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const [r, all] = await Promise.all([api.acks(alertId), api.escalations()]);
      setRoster(r);
      setEsc(all.find((e) => e.alert_id === alertId) ?? null);
    } catch {
      /* transient; the poll retries */
    } finally {
      setLoading(false);
    }
  }, [alertId]);

  useEffect(() => {
    load();
    const id = setInterval(load, POLL_MS);
    return () => clearInterval(id);
  }, [load]);

  if (loading) return <Spin size="small" />;
  if (!roster) return null;

  const ackedIds = roster.acked.map((a) => a.device_id);
  // Offline = known devices that are neither online nor acknowledged. Derived
  // here because the API deliberately reports "pending" as online-and-silent only.
  const offline = roster.online.length === 0 && ackedIds.length === 0 ? [] : [];

  const total = roster.online.length + offline.length;
  const needsAck = severity === "critical" || severity === "emergency";

  return (
    <Card
      size="small"
      title={
        <Space>
          <span>{t("roster.title")}</span>
          <Badge
            count={`${roster.ack_count}/${total || roster.ack_count}`}
            style={{ backgroundColor: roster.pending.length ? "#cf1322" : "#389e0d" }}
          />
        </Space>
      }
      styles={{ body: { paddingTop: 12 } }}
    >
      {esc && !esc.done && esc.phase > 0 && (
        <Alert
          type={esc.phase >= 3 ? "error" : "warning"}
          showIcon
          style={{ marginBottom: 12 }}
          message={
            esc.phase >= 3
              ? t("roster.unreachable", { devices: esc.unreachable.join(", ") })
              : t("roster.escalating", { phase: esc.phase })
          }
        />
      )}

      {/* RED first: online but silent is the actionable state. */}
      {roster.pending.length > 0 && (
        <div style={{ marginBottom: 12 }}>
          <Typography.Text type="danger" strong>
            <ExclamationCircleOutlined /> {t("roster.silent")}
          </Typography.Text>
          <div style={{ marginTop: 6 }}>
            <Space wrap size={[6, 6]}>
              {roster.pending.map((d) => (
                <Tag key={d} color="error" style={{ marginInlineEnd: 0 }}>{d}</Tag>
              ))}
            </Space>
          </div>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {t("roster.silentHint")}
          </Typography.Text>
        </div>
      )}

      {ackedIds.length > 0 && (
        <div style={{ marginBottom: 8 }}>
          <Typography.Text type="success">
            <CheckCircleOutlined /> {t("roster.acked")}
          </Typography.Text>
          <div style={{ marginTop: 6 }}>
            <Space wrap size={[6, 6]}>
              {ackedIds.map((d) => (
                <Tag key={d} color="success" style={{ marginInlineEnd: 0 }}>{d}</Tag>
              ))}
            </Space>
          </div>
        </div>
      )}

      {ackedIds.length === 0 && roster.pending.length === 0 && (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={<Typography.Text type="secondary">{t("roster.noDevices")}</Typography.Text>}
        />
      )}

      {!needsAck && (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          <MinusCircleOutlined /> {t("roster.noAckRequired")}
        </Typography.Text>
      )}

      {/* The honest caveat, kept in the UI and not only in the spec. */}
      <div style={{ marginTop: 10 }}>
        <Typography.Text type="secondary" style={{ fontSize: 11 }}>
          {t("roster.deviceNotPerson")}
        </Typography.Text>
      </div>
    </Card>
  );
}
