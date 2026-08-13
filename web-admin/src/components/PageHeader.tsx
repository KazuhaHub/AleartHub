import { Typography } from "antd";

export default function PageHeader({ title, subtitle, actions }: {
  title: string; subtitle?: string; actions?: React.ReactNode;
}) {
  return (
    <div style={{ display: "flex", alignItems: "flex-end", marginBottom: 24, gap: 16, flexWrap: "wrap" }}>
      <div style={{ flex: 1, minWidth: 0 }}>
        <Typography.Title level={4} style={{ margin: 0, fontWeight: 500 }}>{title}</Typography.Title>
        {subtitle && <Typography.Text type="secondary">{subtitle}</Typography.Text>}
      </div>
      {actions}
    </div>
  );
}
