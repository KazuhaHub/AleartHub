import { useEffect, useState } from "react";
import {
  App as AntApp, Button, Card, Col, Flex, Form, Input, List, Popconfirm, Row, Select,
  Space, Tag, Typography,
} from "antd";
import { SendOutlined } from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import PageHeader from "@/components/PageHeader";
import { api, type Alert, type Category, type PublishReq, type Scenario, type Severity } from "@/api";

const SEVERITIES: Severity[] = ["notice", "warning", "critical", "emergency"];
const CATEGORIES: Category[] = ["earthquake", "fire", "weather", "system", "security", "custom"];

// Scenario templates come from the SERVER (SPEC-SAFETY §6.3). They used to be
// hardcoded here, which meant the panel could drift from any other client — and
// in an emergency people act from muscle memory, so a button that says something
// different on the phone than on the wall tablet is worse than no button.

// antd status colours track the theme tokens (colorInfo / colorWarning / colorError),
// so this keeps the MUI Chip colour semantics under the M3 palette.
const tagColor = (a: Alert) =>
  a.type === "cancel" ? "default" : a.severity === "notice" ? "processing" : a.severity === "warning" ? "warning" : "error";

// MUI's Snackbar autoHideDuration was 4000ms; antd message durations are seconds.
const TOAST_SECONDS = 4;

export default function PublishView() {
  const { t } = useTranslation();
  const { message } = AntApp.useApp();
  const [form, setForm] = useState<PublishReq>({
    severity: "emergency", category: "earthquake",
    title: "正在发生地震", body: "震中距你约 42 公里，预计 15 秒后到达。", action: "",
  });
  const [ttl, setTtl] = useState("");
  const [history, setHistory] = useState<Alert[]>([]);
  const [scenarios, setScenarios] = useState<Scenario[]>([]);
  const [firing, setFiring] = useState<string | null>(null);

  const load = () => api.history().then(setHistory).catch(() => {});

  useEffect(() => { api.scenarios().then(setScenarios).catch(() => {}); }, []);

  // A scenario fires through its own endpoint so the wording stays server-owned;
  // it is confirmed first because these are the loudest messages the system sends.
  const fireScenario = async (sc: Scenario) => {
    setFiring(sc.id);
    try {
      await api.publishScenario(sc.id);
      message.success(t("publish.sent"));
      load();
    } catch {
      message.error(t("publish.failed"));
    } finally {
      setFiring(null);
    }
  };
  useEffect(() => { load(); }, []);

  const set = (k: keyof PublishReq, v: string) => setForm((f) => ({ ...f, [k]: v }));

  const send = async () => {
    try {
      const req: PublishReq = { ...form };
      const n = parseInt(ttl, 10);
      if (!isNaN(n) && n > 0) req.ttl = n;
      const a = await api.publish(req);
      message.success(`已发布 ${a.severity}/${a.category} · ${a.id.slice(0, 8)}…`, TOAST_SECONDS);
      load();
    } catch (e: any) {
      message.error("发布失败：" + (e?.response?.data || e?.message || e), TOAST_SECONDS);
    }
  };

  const cancel = async (id: string) => {
    try { await api.cancel(id); message.success("已解除 " + id.slice(0, 8) + "…", TOAST_SECONDS); load(); }
    catch (e: any) { message.error("解除失败：" + (e?.message || e), TOAST_SECONDS); }
  };

  return (
    <>
      <PageHeader title={t("publish.title")} subtitle={t("publish.subtitle")} />
      <Row gutter={[16, 16]} align="top">
        <Col xs={24} md={14}>
          <Card>
            {/* Two actions per scenario, deliberately distinct: the chip PREFILLS
                the form for review, the send button FIRES the server-owned
                wording immediately. Prefilling and firing must never be the same
                click — one is a draft, the other is an emergency broadcast. */}
            <Flex wrap gap={8} style={{ marginBottom: 16 }}>
              {scenarios.map((sc) => (
                <Space.Compact key={sc.id}>
                  <Tag
                    variant="outlined"
                    style={{ cursor: "pointer", userSelect: "none", marginInlineEnd: 0 }}
                    onClick={() => {
                      setForm({
                        severity: sc.severity, category: sc.category,
                        title: sc.title, body: sc.body, action: sc.action,
                      });
                      setTtl("");
                    }}
                  >
                    {sc.icon} {sc.label}
                  </Tag>
                  <Popconfirm
                    title={t("publish.confirmScenario", { label: sc.label })}
                    okText={t("common.confirm")}
                    cancelText={t("common.cancel")}
                    onConfirm={() => fireScenario(sc)}
                  >
                    <Button
                      size="small"
                      type="text"
                      danger={sc.severity === "emergency" || sc.severity === "critical"}
                      icon={<SendOutlined />}
                      loading={firing === sc.id}
                      aria-label={t("publish.fireScenario", { label: sc.label })}
                    />
                  </Popconfirm>
                </Space.Compact>
              ))}
            </Flex>
            {/* component="div": renders no <form> element, so Enter can't submit/reload (matches the old MUI markup). */}
            <Form layout="vertical" component="div">
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item label="severity" style={{ marginBottom: 16 }}>
                    <Select
                      value={form.severity}
                      onChange={(v) => set("severity", v)}
                      options={SEVERITIES.map((s) => ({ value: s, label: s }))}
                      style={{ width: "100%" }}
                    />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label="category" style={{ marginBottom: 16 }}>
                    <Select
                      value={form.category}
                      onChange={(v) => set("category", v)}
                      options={CATEGORIES.map((c) => ({ value: c, label: c }))}
                      style={{ width: "100%" }}
                    />
                  </Form.Item>
                </Col>
              </Row>
              <Form.Item label="标题 title" style={{ marginBottom: 16 }}>
                <Input value={form.title} onChange={(e) => set("title", e.target.value)} />
              </Form.Item>
              <Form.Item label="正文 body" style={{ marginBottom: 16 }}>
                <Input.TextArea autoSize={{ minRows: 2 }} value={form.body} onChange={(e) => set("body", e.target.value)} />
              </Form.Item>
              <Form.Item label="处置 action（留空按类别自动填）" style={{ marginBottom: 16 }}>
                <Input value={form.action} onChange={(e) => set("action", e.target.value)} />
              </Form.Item>
              <Form.Item label="ttl 秒（留空按默认）" style={{ marginBottom: 16 }}>
                <Input value={ttl} onChange={(e) => setTtl(e.target.value)} style={{ maxWidth: 240 }} />
              </Form.Item>
            </Form>
            <Button type="primary" size="large" icon={<SendOutlined />} onClick={send}>发送警报</Button>
          </Card>
        </Col>

        <Col xs={24} md={10}>
          <Card>
            <Typography.Title level={5} style={{ marginTop: 0, marginBottom: 8, fontWeight: 500 }}>历史 / 当前</Typography.Title>
            {history.length === 0 ? <Typography.Text type="secondary">暂无</Typography.Text> : (
              <List
                size="small"
                dataSource={history.slice(0, 12)}
                renderItem={(a) => (
                  <List.Item
                    key={a.id}
                    actions={a.type !== "cancel" ? [
                      <Button key="cancel" type="text" size="small" style={{ fontSize: 13 }} onClick={() => cancel(a.id)}>解除</Button>,
                    ] : undefined}
                  >
                    <List.Item.Meta
                      avatar={
                        <Tag color={tagColor(a)} style={{ minWidth: 70, textAlign: "center", marginInlineEnd: 0 }}>
                          {a.type === "cancel" ? "cancel" : a.severity}
                        </Tag>
                      }
                      title={a.type === "cancel" ? `解除 ${a.cancels.slice(0, 8)}…` : a.title}
                      description={new Date(a.issued_at * 1000).toLocaleTimeString()}
                    />
                  </List.Item>
                )}
              />
            )}
          </Card>
        </Col>
      </Row>
    </>
  );
}
