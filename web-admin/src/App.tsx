import { useEffect, useMemo } from "react";
import { ThemeProvider, CssBaseline, Box, CircularProgress } from "@mui/material";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import "@fontsource/roboto/400.css";
import "@fontsource/roboto/500.css";
import { useAppearance } from "@/stores/appearance";
import { useAuth } from "@/stores/auth";
import { buildTheme } from "@/theme";
import AdminLayout from "@/layouts/AdminLayout";
import LoginView from "@/views/LoginView";
import SSOCallback from "@/views/SSOCallback";
import DashboardView from "@/views/DashboardView";
import PublishView from "@/views/PublishView";
import SettingsView from "@/views/SettingsView";
import ComingSoon from "@/views/ComingSoon";

function Guard() {
  const ready = useAuth((s) => s.ready);
  const authed = useAuth((s) => s.authed);
  if (!ready) {
    return (
      <Box sx={{ height: "100vh", display: "flex", alignItems: "center", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (!authed) return <Navigate to="/login" replace />;
  return <AdminLayout />;
}

export default function App() {
  const { seed, mode } = useAppearance();
  const theme = useMemo(() => buildTheme(seed, mode), [seed, mode]);
  useEffect(() => { useAuth.getState().init(); }, []);
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <BrowserRouter basename="/admin">
        <Routes>
          <Route path="/login" element={<LoginView />} />
          <Route path="/sso" element={<SSOCallback />} />
          <Route element={<Guard />}>
            <Route path="/" element={<DashboardView />} />
            <Route path="/publish" element={<PublishView />} />
            <Route path="/devices" element={<ComingSoon titleKey="nav.devices" />} />
            <Route path="/history" element={<ComingSoon titleKey="nav.history" />} />
            <Route path="/sources" element={<ComingSoon titleKey="nav.sources" />} />
            <Route path="/settings" element={<SettingsView />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ThemeProvider>
  );
}
