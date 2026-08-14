import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Input, Segmented, Space, Table, Tag, Tooltip, Typography } from "antd";
import { ReloadOutlined, SearchOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import { api, type Device } from "@/api";

// Presence comes from the retained status/<deviceId> topics (SPEC §5.4): each
// device publishes a birth message and registers an LWT, so "offline" rows are
// devices the broker saw disconnect — not devices that were never here.
const POLL_MS = 5000;

function ago(unixSec: number, t: (k: string, o?: any) => string): string {
  if (!unixSec) return "—";
  const s = Math.max(0, Math.floor(Date.now() / 1000) - unixSec);
  if (s < 60) return t("devices.agoSec", { n: s });
  if (s < 3600) return t("devices.agoMin", { n: Math.floor(s / 60) });
  if (s < 86400) return t("devices.agoHour", { n: Math.floor(s / 3600) });
  return t("devices.agoDay", { n: Math.floor(s / 86400) });
}

export default function DevicesView() {
  const { t } = useTranslation();
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<"all" | "online" | "offline">("all");
  const [q, setQ] = useState("");

  const load = useCallback(async () => {
    try {
      setDevices(await api.devices());
    } catch {
      /* transient; the poll will retry */
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, POLL_MS);
    return () => clearInterval(id);
  }, [load]);

  const online = devices.filter((d) => d.state === "online").length;

  const rows = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return devices
      .filter((d) => (filter === "all" ? true : d.state === filter))
      .filter((d) => !needle || d.device_id.toLowerCase().includes(needle))
      .sort((a, b) => {
        // Online first, then most recently seen — the roster reads as "who can
        // actually receive an alert right now".
        if (a.state !== b.state) return a.state === "online" ? -1 : 1;
        return (b.last_seen || b.at) - (a.last_seen || a.at);
      });
  }, [devices, filter, q]);

  const columns: ColumnsType<Device> = [
    {
      title: t("devices.colDevice"),
      dataIndex: "device_id",
      key: "device_id",
      render: (id: string) => <Typography.Text code copyable={{ text: id }}>{id}</Typography.Text>,
    },
    {
      title: t("devices.colState"),
      dataIndex: "state",
      key: "state",
      width: 120,
      render: (s: string) =>
        s === "online"
          ? <Tag color="success">{t("devices.online")}</Tag>
          : <Tag>{t("devices.offline")}</Tag>,
    },
    {
      title: t("devices.colLastSeen"),
      key: "last_seen",
      width: 160,
      render: (_: unknown, d: Device) => (
        <Tooltip title={new Date((d.last_seen || d.at) * 1000).toLocaleString()}>
          <span>{ago(d.last_seen || d.at, t)}</span>
        </Tooltip>
      ),
    },
    {
      title: t("devices.colClient"),
      dataIndex: "client",
      key: "client",
      responsive: ["md"],
      render: (c: string) => <Typography.Text type="secondary">{c || "—"}</Typography.Text>,
    },
  ];

  return (
    <>
      <PageHeader
        title={t("devices.title")}
        subtitle={t("devices.subtitle", { online, total: devices.length })}
        actions={
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>
            {t("common.refresh")}
          </Button>
        }
      />

      {/* The device plane is still single-tenant — say so here rather than let an
          operator assume this roster is scoped to the active org. */}
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message={t("devices.globalNotice")}
      />

      <Card styles={{ body: { padding: 16 } }}>
        <Space wrap style={{ marginBottom: 16 }}>
          <Segmented
            value={filter}
            onChange={(v) => setFilter(v as typeof filter)}
            options={[
              { label: t("devices.filterAll"), value: "all" },
              { label: t("devices.online"), value: "online" },
              { label: t("devices.offline"), value: "offline" },
            ]}
          />
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder={t("devices.searchPlaceholder")}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ width: 240 }}
          />
        </Space>
        <Table<Device>
          rowKey="device_id"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={rows}
          pagination={rows.length > 20 ? { pageSize: 20, showSizeChanger: false } : false}
          locale={{ emptyText: t("devices.empty") }}
          scroll={{ x: "max-content" }}
        />
      </Card>
    </>
  );
}
