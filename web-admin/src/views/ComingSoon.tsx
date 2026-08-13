import { Typography, theme } from "antd";
import { ToolOutlined } from "@ant-design/icons";
import { useTranslation } from "react-i18next";

export default function ComingSoon({ titleKey }: { titleKey: string }) {
  const { t } = useTranslation();
  const { token } = theme.useToken();
  return (
    <div style={{
      display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center",
      paddingTop: 96, paddingBottom: 96, gap: 12, color: token.colorTextSecondary,
    }}>
      <ToolOutlined style={{ fontSize: 48, opacity: 0.5 }} />
      <Typography.Title level={5} style={{ margin: 0, color: token.colorTextSecondary }}>{t(titleKey)}</Typography.Title>
      <Typography.Text type="secondary">{t("common.comingSoon")}</Typography.Text>
    </div>
  );
}
