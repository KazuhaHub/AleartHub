import { useEffect, useState } from "react";
import {
  Alert, App as AntApp, Button, Card, Input, List, Modal, Space, Tag, Typography, theme,
} from "antd";
import {
  ApiOutlined, DeleteOutlined, KeyOutlined, SafetyCertificateOutlined,
} from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import TwoFADialog from "@/components/TwoFADialog";
import { adminApi, authApi, passkeyApi, type PasskeyInfo, type ServiceAccount } from "@/api";
import { passkeyRegister } from "@/passkey";

export default function SettingsView() {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  const { message } = AntApp.useApp();
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
    // antd has no prompt equivalent -- keep the native one so behaviour is unchanged.
    const name = window.prompt(t("passkey.namePrompt"), "My passkey");
    if (name === null) return;
    try { await passkeyRegister(name || "Passkey"); message.success(t("passkey.added")); refresh(); }
    catch { message.error(t("passkey.failed")); }
  };
  const delPasskey = async (id: number) => { await passkeyApi.delete(id); refresh(); };

  const createKey = async () => {
    try {
      const r = await adminApi.saCreate(newKeyName.trim() || "API key", ["alerts:ingest"]);
      setIssuedToken(r.token);
      setNewKeyName("");
      setNewKeyOpen(false);
      refresh();
    } catch { message.error(t("settings.keyCreateFailed")); }
  };
  const delKey = async (id: number) => { await adminApi.saDelete(id); refresh(); };

  const copyToken = () => {
    navigator.clipboard?.writeText(issuedToken);
    message.success(t("settings.copied"));
  };

  return (
    <>
      <PageHeader title={t("nav.settings")} subtitle={t("settings.subtitle")} />
      <Space direction="vertical" size={16} style={{ display: "flex", maxWidth: 760 }}>
        {/* Security */}
        <Card>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12 }}>
            <SafetyCertificateOutlined style={{ color: token.colorPrimary }} />
            <Typography.Text strong>{t("settings.security")}</Typography.Text>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 16 }}>
            <Typography.Text style={{ flex: 1 }}>{t("twofa.menu")}</Typography.Text>
            <Tag color={twofaOn ? "success" : undefined}>{twofaOn ? t("settings.on") : t("settings.off")}</Tag>
            <Button onClick={() => setTwofaOpen(true)}>{t("settings.manage")}</Button>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 8 }}>
            <KeyOutlined />
            <Typography.Text style={{ flex: 1 }}>Passkeys</Typography.Text>
            <Button size="small" onClick={addPasskey}>{t("passkey.add")}</Button>
          </div>
          <List
            size="small"
            dataSource={passkeys}
            locale={{ emptyText: t("settings.none") }}
            renderItem={(p) => (
              <List.Item
                actions={[
                  <Button
                    key="del"
                    type="text"
                    shape="circle"
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={() => delPasskey(p.id)}
                  />,
                ]}
              >
                <List.Item.Meta
                  title={p.name}
                  description={new Date(p.created_at * 1000).toLocaleString()}
                />
              </List.Item>
            )}
          />
        </Card>

        {/* Integrations / API keys */}
        <Card>
          <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 4 }}>
            <ApiOutlined style={{ color: token.colorPrimary }} />
            <Typography.Text strong>{t("settings.integrations")}</Typography.Text>
            <div style={{ flex: 1 }} />
            <Button type="primary" onClick={() => setNewKeyOpen(true)}>{t("settings.newKey")}</Button>
          </div>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
            {t("settings.integrationsHint")}
          </Typography.Paragraph>
          <List
            size="small"
            dataSource={keys}
            locale={{ emptyText: t("settings.none") }}
            renderItem={(k) => (
              <List.Item
                actions={[
                  <Button
                    key="del"
                    type="text"
                    shape="circle"
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={() => delKey(k.id)}
                  />,
                ]}
              >
                <List.Item.Meta
                  title={<>{k.name} {k.scopes.map((s) => <Tag key={s} style={{ marginInlineStart: 4 }}>{s}</Tag>)}</>}
                  description={k.last_used_at
                    ? `${t("settings.lastUsed")} ${new Date(k.last_used_at * 1000).toLocaleString()}`
                    : t("settings.neverUsed")}
                />
              </List.Item>
            )}
          />
        </Card>
      </Space>

      <TwoFADialog open={twofaOpen} onClose={() => { setTwofaOpen(false); refresh(); }} />

      {/* create-key dialog */}
      <Modal
        open={newKeyOpen}
        onCancel={() => setNewKeyOpen(false)}
        title={t("settings.newKey")}
        width={420}
        destroyOnHidden
        footer={[
          <Button key="cancel" onClick={() => setNewKeyOpen(false)}>{t("twofa.cancel")}</Button>,
          <Button key="create" type="primary" onClick={createKey}>{t("settings.create")}</Button>,
        ]}
      >
        <Typography.Text type="secondary" style={{ display: "block", marginBottom: 4 }}>
          {t("settings.keyName")}
        </Typography.Text>
        <Input autoFocus value={newKeyName} onChange={(e) => setNewKeyName(e.target.value)} />
        <Typography.Text type="secondary" style={{ display: "block", marginTop: 4, fontSize: 12 }}>
          scope: alerts:ingest
        </Typography.Text>
      </Modal>

      {/* show-token-once dialog */}
      <Modal
        open={!!issuedToken}
        onCancel={() => setIssuedToken("")}
        title={t("settings.keyCreated")}
        footer={[
          <Button key="copy" onClick={copyToken}>{t("settings.copy")}</Button>,
          <Button key="done" type="primary" onClick={() => setIssuedToken("")}>{t("twofa.done")}</Button>,
        ]}
      >
        <Alert type="warning" showIcon message={t("settings.tokenOnce")} style={{ marginBottom: 16 }} />
        <Typography.Paragraph
          copyable={{
            text: issuedToken,
            tooltips: [t("settings.copy"), t("settings.copied")],
            onCopy: () => message.success(t("settings.copied")),
          }}
          style={{
            fontFamily: "monospace",
            background: token.colorFillTertiary,
            padding: 12,
            borderRadius: token.borderRadius,
            wordBreak: "break-all",
            marginBottom: 0,
          }}
        >
          {issuedToken}
        </Typography.Paragraph>
      </Modal>
    </>
  );
}
