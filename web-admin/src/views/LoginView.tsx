import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Button, Card, Divider, Form, Input, Space, Typography, theme } from "antd";
import { KeyOutlined, LoginOutlined } from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/stores/auth";
import { authApi } from "@/api";

export default function LoginView() {
  const { token } = theme.useToken();
  const { t } = useTranslation();
  const nav = useNavigate();
  const login = useAuth((s) => s.login);
  const complete2FA = useAuth((s) => s.complete2FA);
  const loginPasskey = useAuth((s) => s.loginPasskey);
  const [upn, setUpn] = useState("admin");
  const [pw, setPw] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [stage, setStage] = useState<"login" | "2fa">("login");
  const [pending, setPending] = useState("");
  const [code, setCode] = useState("");
  const [ssoUrl, setSsoUrl] = useState("");

  useEffect(() => {
    authApi.methods().then((m) => {
      if (m.oidc) setSsoUrl("/api/auth/oidc/login");
      else if (m.saml) setSsoUrl("/api/auth/saml/login");
    }).catch(() => {});
  }, []);

  const passkey = async () => {
    setErr("");
    setBusy(true);
    try {
      await loginPasskey();
      nav("/", { replace: true });
    } catch {
      setErr(t("login.passkeyError"));
    } finally {
      setBusy(false);
    }
  };

  // antd <Form> already calls preventDefault() on the native submit event.
  const submit = async () => {
    setErr("");
    setBusy(true);
    try {
      const r = await login(upn, pw);
      if (r.twoFA) {
        setPending(r.pending!);
        setStage("2fa");
      } else {
        nav("/", { replace: true });
      }
    } catch {
      setErr(t("login.error"));
    } finally {
      setBusy(false);
    }
  };

  const verify = async () => {
    setErr("");
    setBusy(true);
    try {
      await complete2FA(pending, code.trim());
      nav("/", { replace: true });
    } catch {
      setErr(t("login.2faError"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: token.colorBgLayout,
        padding: 16,
      }}
    >
      <Card style={{ width: 380, maxWidth: "100%" }} styles={{ body: { padding: 32 } }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 4 }}>
          <div style={{ width: 28, height: 28, borderRadius: "50%", background: token.colorError }} />
          <Typography.Title level={4} style={{ margin: 0, fontWeight: 500 }}>AlertHub</Typography.Title>
        </div>
        <Typography.Text type="secondary" style={{ display: "block", marginBottom: 24 }}>
          {stage === "2fa" ? t("login.2faSubtitle") : t("login.subtitle")}
        </Typography.Text>
        {stage === "2fa" ? (
          <Form layout="vertical" onFinish={verify}>
            <Space direction="vertical" size={16} style={{ display: "flex" }}>
              <Form.Item label={t("login.2faCode")} style={{ marginBottom: 0 }}>
                <Input
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  autoFocus
                  inputMode="numeric"
                  autoComplete="one-time-code"
                />
              </Form.Item>
              {err && <Alert type="error" showIcon message={err} />}
              <Button type="primary" htmlType="submit" size="large" block disabled={busy || !code}>
                {t("login.verify")}
              </Button>
              <Button
                type="text"
                size="small"
                block
                onClick={() => { setStage("login"); setErr(""); setCode(""); }}
              >
                {t("login.back")}
              </Button>
            </Space>
          </Form>
        ) : (
          <>
            <Form layout="vertical" onFinish={submit}>
              <Space direction="vertical" size={16} style={{ display: "flex" }}>
                <Form.Item label={t("login.upn")} style={{ marginBottom: 0 }}>
                  <Input value={upn} onChange={(e) => setUpn(e.target.value)} autoFocus />
                </Form.Item>
                <Form.Item label={t("login.password")} style={{ marginBottom: 0 }}>
                  <Input.Password value={pw} onChange={(e) => setPw(e.target.value)} />
                </Form.Item>
                {err && <Alert type="error" showIcon message={err} />}
                <Button type="primary" htmlType="submit" size="large" block disabled={busy || !pw}>
                  {t("login.submit")}
                </Button>
              </Space>
            </Form>
            <Divider plain style={{ marginTop: 16, marginBottom: 16 }}>{t("login.or")}</Divider>
            <Button block size="large" icon={<KeyOutlined />} disabled={busy} onClick={passkey}>
              {t("login.passkey")}
            </Button>
            {ssoUrl && (
              <Button
                block
                size="large"
                style={{ marginTop: 8 }}
                icon={<LoginOutlined />}
                onClick={() => { window.location.href = ssoUrl; }}
              >
                {t("login.sso")}
              </Button>
            )}
          </>
        )}
      </Card>
    </div>
  );
}
