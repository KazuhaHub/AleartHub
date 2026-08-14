import { useCallback, useEffect, useState } from "react";
import { Alert, Card, Col, Row, Space, Tag, Typography } from "antd";
import { ApiOutlined, CheckCircleOutlined, MinusCircleOutlined, SendOutlined } from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import { api, type SourceInfo } from "@/api";

// Read-only on purpose: making feeds editable here would turn the console into
// another way to inject alerts, and the console is the surface most likely to be
// phished. Configuration stays in the environment.
export default function SourcesView() {
  const { t } = useTranslation();
  const [sources, setSources] = useState<SourceInfo[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      setSources(await api.sources());
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => { load(); }, [load]);

  const group = (kind: "ingress" | "egress") => sources.filter((s) => s.kind === kind);

  const card = (s: SourceInfo) => (
    <Col xs={24} md={12} key={s.key}>
      <Card
        loading={loading}
        style={{ height: "100%" }}
        styles={{ body: { padding: 16 } }}
        title={
          <Space>
            <Typography.Text strong>{s.key}</Typography.Text>
            {s.enabled
              ? <Tag color="success" icon={<CheckCircleOutlined />}>{t("sources.enabled")}</Tag>
              : <Tag icon={<MinusCircleOutlined />}>{t("sources.disabled")}</Tag>}
          </Space>
        }
      >
        <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
          {s.detail}
        </Typography.Paragraph>
        <Typography.Text code style={{ fontSize: 12 }}>{s.config_var}</Typography.Text>
      </Card>
    </Col>
  );

  return (
    <>
      <PageHeader title={t("sources.title")} subtitle={t("sources.subtitle")} />
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        message={t("sources.readOnlyNotice")}
      />

      <Typography.Title level={5} style={{ margin: "0 0 12px" }}>
        <Space><ApiOutlined />{t("sources.ingress")}</Space>
      </Typography.Title>
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>{group("ingress").map(card)}</Row>

      <Typography.Title level={5} style={{ margin: "0 0 12px" }}>
        <Space><SendOutlined />{t("sources.egress")}</Space>
      </Typography.Title>
      <Row gutter={[16, 16]}>{group("egress").map(card)}</Row>
    </>
  );
}
