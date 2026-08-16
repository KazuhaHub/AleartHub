import { useCallback, useEffect, useMemo, useState } from "react";
import { App, Button, Card, Input, Popconfirm, Segmented, Space, Table, Tag, Tooltip, Typography } from "antd";
import { ReloadOutlined, SearchOutlined, StopOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import AckRoster from "@/components/AckRoster";
import { api, type Alert as AlertRow } from "@/api";

// The server returns the 50 most recent envelopes for the ACTIVE org, newest
// first (SPEC §7). There is no pagination endpoint, so this view is deliberately
// a window on the recent past rather than a full archive.
const sevColor: Record<string, string> = {
  notice: "processing", warning: "warning", critical: "error", emergency: "error",
};

export default function HistoryView() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const [rows, setRows] = useState<AlertRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [sev, setSev] = useState<string>("all");
  const [q, setQ] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setRows(await api.history());
    } catch {
      message.error(t("history.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [message, t]);

  useEffect(() => { load(); }, [load]);

  // Every cancel envelope names the alert it recalled, so the history itself
  // tells us which alerts are already dead. Without this an alert that was just
  // recalled would still read "Live" and offer a second, pointless Recall.
  const cancelledIds = useMemo(
    () => new Set(rows.filter((a) => a.type === "cancel" && a.cancels).map((a) => a.cancels)),
    [rows],
  );

  // An alert is still "live" while it is un-recalled and within now < issued_at + ttl.
  // Only those can be usefully recalled; cancelling an expired one is a no-op.
  const isLive = (a: AlertRow) =>
    a.type === "alert" &&
    !cancelledIds.has(a.id) &&
    Math.floor(Date.now() / 1000) < a.issued_at + a.ttl;

  const cancel = async (a: AlertRow) => {
    setBusyId(a.id);
    try {
      await api.cancel(a.id);
      message.success(t("history.cancelled"));
      await load();
    } catch {
      message.error(t("history.cancelFailed"));
    } finally {
      setBusyId(null);
    }
  };

  const view = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return rows
      .filter((a) => sev === "all" || a.severity === sev)
      .filter((a) => !needle ||
        a.title.toLowerCase().includes(needle) ||
        (a.body || "").toLowerCase().includes(needle) ||
        a.id.toLowerCase().includes(needle));
  }, [rows, sev, q]);

  const columns: ColumnsType<AlertRow> = [
    {
      title: t("history.colTime"),
      key: "issued_at",
      width: 170,
      render: (_: unknown, a: AlertRow) => (
        <Tooltip title={`id: ${a.id}`}>
          <span>{new Date(a.issued_at * 1000).toLocaleString()}</span>
        </Tooltip>
      ),
    },
    {
      title: t("history.colSeverity"),
      dataIndex: "severity",
      key: "severity",
      width: 130,
      render: (s: string, a: AlertRow) =>
        a.type === "cancel"
          ? <Tag>{t("history.typeCancel")}</Tag>
          : <Tag color={sevColor[s] ?? "default"}>{s}</Tag>,
    },
    {
      title: t("history.colTitle"),
      key: "title",
      render: (_: unknown, a: AlertRow) => (
        <div>
          <Typography.Text strong>{a.title}</Typography.Text>
          {a.body && (
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>{a.body}</Typography.Text>
            </div>
          )}
          {a.cancels && (
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {t("history.cancelsRef", { id: a.cancels })}
              </Typography.Text>
            </div>
          )}
        </div>
      ),
    },
    {
      title: t("history.colSource"),
      dataIndex: "source",
      key: "source",
      width: 120,
      responsive: ["md"],
      render: (s: string) => <Typography.Text type="secondary">{s}</Typography.Text>,
    },
    {
      title: t("history.colState"),
      key: "state",
      width: 110,
      render: (_: unknown, a: AlertRow) => {
        if (a.type === "cancel") return <Typography.Text type="secondary">—</Typography.Text>;
        if (cancelledIds.has(a.id)) return <Tag>{t("history.recalled")}</Tag>;
        if (isLive(a)) return <Tag color="success">{t("history.live")}</Tag>;
        return <Typography.Text type="secondary">{t("history.expired")}</Typography.Text>;
      },
    },
    {
      title: "",
      key: "actions",
      width: 110,
      render: (_: unknown, a: AlertRow) =>
        isLive(a) ? (
          <Popconfirm
            title={t("history.confirmCancel")}
            okText={t("common.confirm")}
            cancelText={t("common.cancel")}
            onConfirm={() => cancel(a)}
          >
            <Button size="small" danger icon={<StopOutlined />} loading={busyId === a.id}>
              {t("history.recall")}
            </Button>
          </Popconfirm>
        ) : null,
    },
  ];

  return (
    <>
      <PageHeader
        title={t("history.title")}
        subtitle={t("history.subtitle")}
        actions={
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>
            {t("common.refresh")}
          </Button>
        }
      />
      <Card styles={{ body: { padding: 16 } }}>
        <Space wrap style={{ marginBottom: 16 }}>
          <Segmented
            value={sev}
            onChange={(v) => setSev(v as string)}
            options={[
              { label: t("history.filterAll"), value: "all" },
              { label: "emergency", value: "emergency" },
              { label: "critical", value: "critical" },
              { label: "warning", value: "warning" },
              { label: "notice", value: "notice" },
            ]}
          />
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder={t("history.searchPlaceholder")}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ width: 240 }}
          />
        </Space>
        <Table<AlertRow>
          rowKey="id"
          // The roster is per-alert and only meaningful while an alert is live,
          // so it expands from the row rather than taking a page of its own.
          expandable={{
            rowExpandable: (a) =>
              a.type === "alert" && (a.severity === "critical" || a.severity === "emergency"),
            expandedRowRender: (a) => <AckRoster alertId={a.id} severity={a.severity} />,
          }}
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={view}
          pagination={view.length > 20 ? { pageSize: 20, showSizeChanger: false } : false}
          locale={{ emptyText: t("history.empty") }}
          scroll={{ x: "max-content" }}
        />
      </Card>
    </>
  );
}
