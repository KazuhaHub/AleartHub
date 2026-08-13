import { Box, Typography } from "@mui/material";

export default function PageHeader({ title, subtitle, actions }: {
  title: string; subtitle?: string; actions?: React.ReactNode;
}) {
  return (
    <Box sx={{ display: "flex", alignItems: "flex-end", mb: 3, gap: 2, flexWrap: "wrap" }}>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="h5" fontWeight={500}>{title}</Typography>
        {subtitle && <Typography variant="body2" color="text.secondary">{subtitle}</Typography>}
      </Box>
      {actions}
    </Box>
  );
}
