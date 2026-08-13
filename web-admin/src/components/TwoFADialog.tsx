import { useEffect, useState } from "react";
import { Alert, Button, Flex, Input, Modal, Typography, theme } from "antd";
import { QRCodeSVG } from "qrcode.react";
import { useTranslation } from "react-i18next";
import { authApi } from "@/api";

type Stage = "loading" | "enroll" | "recovery" | "enabled";

export default function TwoFADialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { token } = theme.useToken();
  const { t } = useTranslation();
  const [stage, setStage] = useState<Stage>("loading");
  const [otpauth, setOtpauth] = useState("");
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    setStage("loading");
    setErr(""); setCode(""); setRecovery([]);
    authApi.twofa.status()
      .then((s) => {
        if (s.enabled) { setStage("enabled"); return; }
        return authApi.twofa.begin().then((b) => {
          setOtpauth(b.otpauth_url); setSecret(b.secret); setStage("enroll");
        });
      })
      .catch(() => setErr(t("twofa.error")));
  }, [open, t]);

  const enable = async () => {
    setBusy(true); setErr("");
    try {
      const r = await authApi.twofa.enable(code.trim());
      setRecovery(r.recovery_codes); setStage("recovery");
    } catch { setErr(t("twofa.badCode")); } finally { setBusy(false); }
  };
  const disable = async () => {
    setBusy(true); setErr("");
    try { await authApi.twofa.disable(code.trim()); onClose(); }
    catch { setErr(t("twofa.badCode")); } finally { setBusy(false); }
  };

  // MUI `label` had no antd equivalent on Input, so the label is rendered by hand.
  const codeLabel = (
    <span style={{ display: "block", marginBottom: 4, fontSize: 12, color: token.colorTextSecondary }}>
      {t("twofa.code")}
    </span>
  );

  const footer = [
    <Button key="close" onClick={onClose}>
      {stage === "recovery" ? t("twofa.done") : t("twofa.cancel")}
    </Button>,
    stage === "enroll" ? (
      <Button key="enable" type="primary" onClick={enable} loading={busy} disabled={!code}>
        {t("twofa.enable")}
      </Button>
    ) : null,
    stage === "enabled" ? (
      <Button key="disable" type="primary" danger onClick={disable} loading={busy} disabled={!code}>
        {t("twofa.disable")}
      </Button>
    ) : null,
  ];

  return (
    <Modal open={open} onCancel={onClose} title={t("twofa.title")} width={444} footer={footer}>
      {stage === "loading" && <Typography.Text type="secondary">…</Typography.Text>}

      {stage === "enroll" && (
        <Flex vertical align="center" gap={16}>
          <Typography.Text type="secondary">{t("twofa.scan")}</Typography.Text>
          {otpauth && (
            <div style={{ background: "#fff", padding: 12, borderRadius: token.borderRadius }}>
              <QRCodeSVG value={otpauth} size={176} />
            </div>
          )}
          <Typography.Text style={{ fontSize: 12, fontFamily: "monospace", wordBreak: "break-all" }}>
            {secret}
          </Typography.Text>
          <div style={{ alignSelf: "stretch" }}>
            {codeLabel}
            <Input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              inputMode="numeric"
              autoComplete="one-time-code"
            />
          </div>
          {err && <Alert type="error" showIcon title={err} style={{ alignSelf: "stretch" }} />}
        </Flex>
      )}

      {stage === "recovery" && (
        <Flex vertical gap={12}>
          <Alert type="success" showIcon title={t("twofa.enabledOk")} />
          <Typography.Text>{t("twofa.saveRecovery")}</Typography.Text>
          <div
            style={{
              fontFamily: "monospace",
              background: token.colorFillTertiary,
              padding: 12,
              borderRadius: token.borderRadius,
              columns: 2,
            }}
          >
            {recovery.map((c) => <div key={c}>{c}</div>)}
          </div>
        </Flex>
      )}

      {stage === "enabled" && (
        <Flex vertical gap={16}>
          <Alert type="info" showIcon title={t("twofa.alreadyEnabled")} />
          <div>
            {codeLabel}
            <Input value={code} onChange={(e) => setCode(e.target.value)} inputMode="numeric" />
          </div>
          {err && <Alert type="error" showIcon title={err} />}
        </Flex>
      )}
    </Modal>
  );
}
