import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, App, Button, Card, Input, Segmented, Space, Table, Tag, Tooltip, Typography } from "antd";
import { ReloadOutlined, SafetyCertificateOutlined, SearchOutlined } from "@ant-design/icons";
import type { ColumnsType } from "antd/es/table";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import { api, type AuditChain, type AuditEntry } from "@/api";

// Actor kinds are the reason this trail is useful: a human, a machine key, the
// static script token and an internal producer are very different answers to
// "who did this".
const actorColor: Record<string, string> = {
  user: "blue",
  service_account: "purple",
  admin_token: "orange",
  system: "default",
};

// Destructive or security-relevant actions are tinted so they stand out in a
// long list; everything else stays neutral.
const actionColor: Record<string, string> = {
  "alert.publish": "error",
  "alert.cancel": "warning",
  "auth.login_failed": "error",
  "auth.login": "success",
  "service_account.create": "purple",
  "org.create": "processing",
};

export default function AuditView() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const [rows, setRows] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [chain, setChain] = useState<AuditChain | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [kind, setKind] = useState("all");
  const [q, setQ] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setRows(await api.audit());
    } catch {
      message.error(t("audit.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [message, t]);

  useEffect(() => { load(); }, [load]);

  const verify = async () => {
    setVerifying(true);
    try {
      setChain(await api.auditVerify());
    } catch {
      // 403 for a non-super admin is expected, not an error worth shouting about.
      message.info(t("audit.verifyForbidden"));
    } finally {
      setVerifying(false);
    }
  };

  const view = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return rows
      .filter((e) => kind === "all" || e.actor_type === kind)
      .filter((e) => !needle ||
        e.action.toLowerCase().includes(needle) ||
        e.actor_name.toLowerCase().includes(needle) ||
        (e.target_id || "").toLowerCase().includes(needle) ||
        (e.detail || "").toLowerCase().includes(needle));
  }, [rows, kind, q]);

  const columns: ColumnsType<AuditEntry> = [
    {
      title: t("audit.colTime"),
      key: "at",
      width: 170,
      render: (_: unknown, e: AuditEntry) => new Date(e.at * 1000).toLocaleString(),
    },
    {
      title: t("audit.colActor"),
      key: "actor",
      width: 200,
      render: (_: unknown, e: AuditEntry) => (
        <Space size={4} wrap>
          <Tag color={actorColor[e.actor_type] ?? "default"}>{t(`audit.actor.${e.actor_type}`)}</Tag>
          <Typography.Text>{e.actor_name || "—"}</Typography.Text>
        </Space>
      ),
    },
    {
      title: t("audit.colAction"),
      dataIndex: "action",
      key: "action",
      width: 190,
      render: (a: string) => <Tag color={actionColor[a] ?? "default"}>{a}</Tag>,
    },
    {
      title: t("audit.colTarget"),
      key: "target",
      render: (_: unknown, e: AuditEntry) => (
        <div>
          {e.target_id && <Typography.Text code>{e.target_id}</Typography.Text>}
          {e.detail && (
            <div>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>{e.detail}</Typography.Text>
            </div>
          )}
        </div>
      ),
    },
    {
      title: t("audit.colIP"),
      dataIndex: "ip",
      key: "ip",
      width: 130,
      responsive: ["lg"],
      render: (ip: string) => <Typography.Text type="secondary">{ip || "—"}</Typography.Text>,
    },
    {
      title: t("audit.colChain"),
      key: "hash",
      width: 110,
      responsive: ["lg"],
      render: (_: unknown, e: AuditEntry) => (
        <Tooltip title={`hash: ${e.hash}\nprev: ${e.prev_hash || "(genesis)"}`}>
          <Typography.Text code style={{ fontSize: 11 }}>{e.hash.slice(0, 8)}</Typography.Text>
        </Tooltip>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title={t("audit.title")}
        subtitle={t("audit.subtitle")}
        actions={
          <Space>
            <Button icon={<SafetyCertificateOutlined />} onClick={verify} loading={verifying}>
              {t("audit.verify")}
            </Button>
            <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>
              {t("common.refresh")}
            </Button>
          </Space>
        }
      />

      {chain && (
        <Alert
          style={{ marginBottom: 16 }}
          showIcon
          type={chain.ok ? "success" : "error"}
          message={chain.ok
            ? t("audit.chainOK", { n: chain.entries })
            : t("audit.chainBroken", { id: chain.bad_id, reason: chain.reason })}
        />
      )}

      <Card styles={{ body: { padding: 16 } }}>
        <Space wrap style={{ marginBottom: 16 }}>
          <Segmented
            value={kind}
            onChange={(v) => setKind(v as string)}
            options={[
              { label: t("audit.filterAll"), value: "all" },
              { label: t("audit.actor.user"), value: "user" },
              { label: t("audit.actor.service_account"), value: "service_account" },
              { label: t("audit.actor.admin_token"), value: "admin_token" },
              { label: t("audit.actor.system"), value: "system" },
            ]}
          />
          <Input
            allowClear
            prefix={<SearchOutlined />}
            placeholder={t("audit.searchPlaceholder")}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ width: 260 }}
          />
        </Space>
        <Table<AuditEntry>
          rowKey="id"
          size="middle"
          loading={loading}
          columns={columns}
          dataSource={view}
          pagination={view.length > 25 ? { pageSize: 25, showSizeChanger: false } : false}
          locale={{ emptyText: t("audit.empty") }}
          scroll={{ x: "max-content" }}
        />
      </Card>
    </>
  );
}
