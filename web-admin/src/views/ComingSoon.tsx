import { Box, Typography } from "@mui/material";
import ConstructionIcon from "@mui/icons-material/Construction";
import { useTranslation } from "react-i18next";

export default function ComingSoon({ titleKey }: { titleKey: string }) {
  const { t } = useTranslation();
  return (
    <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", py: 12, color: "text.secondary", gap: 1.5 }}>
      <ConstructionIcon sx={{ fontSize: 48, opacity: 0.5 }} />
      <Typography variant="h6">{t(titleKey)}</Typography>
      <Typography variant="body2">{t("common.comingSoon")}</Typography>
    </Box>
  );
}
