import { useEffect, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import {
  AppBar, Avatar, Box, Drawer, IconButton, List, ListItemButton, ListItemIcon,
  ListItemText, ListSubheader, Menu, MenuItem, Toolbar, Tooltip, Typography, useMediaQuery,
} from "@mui/material";
import { useTheme } from "@mui/material/styles";
import MenuIcon from "@mui/icons-material/Menu";
import DashboardIcon from "@mui/icons-material/SpaceDashboard";
import CampaignIcon from "@mui/icons-material/Campaign";
import DevicesIcon from "@mui/icons-material/Devices";
import HistoryIcon from "@mui/icons-material/History";
import SourceIcon from "@mui/icons-material/Sensors";
import SettingsIcon from "@mui/icons-material/Settings";
import LightModeIcon from "@mui/icons-material/LightMode";
import DarkModeIcon from "@mui/icons-material/DarkMode";
import BrightnessAutoIcon from "@mui/icons-material/BrightnessAuto";
import PaletteIcon from "@mui/icons-material/Palette";
import TranslateIcon from "@mui/icons-material/Translate";
import { useTranslation } from "react-i18next";
import LogoutIcon from "@mui/icons-material/Logout";
import FingerprintIcon from "@mui/icons-material/Fingerprint";
import ShieldIcon from "@mui/icons-material/Shield";
import ApartmentIcon from "@mui/icons-material/Apartment";
import ArrowDropDownIcon from "@mui/icons-material/ArrowDropDown";
import { api, activeOrg, type OrgInfo } from "@/api";
import { useAppearance } from "@/stores/appearance";
import { useAuth } from "@/stores/auth";
import { passkeyRegister } from "@/passkey";
import { PRESETS } from "@/theme";
import TwoFADialog from "@/components/TwoFADialog";

const DRAWER_W = 256;

type NavItem = { to: string; icon: React.ReactNode; key: string; soon?: boolean };
type NavSection = { section: string; items: NavItem[] };

// Paths are relative to the router basename ("/admin").
const NAV: NavSection[] = [
  { section: "nav.section.overview", items: [
    { to: "/", icon: <DashboardIcon />, key: "nav.dashboard" },
  ]},
  { section: "nav.section.ops", items: [
    { to: "/publish", icon: <CampaignIcon />, key: "nav.publish" },
    { to: "/devices", icon: <DevicesIcon />, key: "nav.devices", soon: true },
    { to: "/history", icon: <HistoryIcon />, key: "nav.history", soon: true },
  ]},
  { section: "nav.section.config", items: [
    { to: "/sources", icon: <SourceIcon />, key: "nav.sources", soon: true },
    { to: "/settings", icon: <SettingsIcon />, key: "nav.settings" },
  ]},
];

export default function AdminLayout() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const { mode, setMode, setSeed } = useAppearance();
  const { user, logout } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();
  const mobile = useMediaQuery(theme.breakpoints.down("md"));
  const [open, setOpen] = useState(false);
  const [colorEl, setColorEl] = useState<null | HTMLElement>(null);
  const [userEl, setUserEl] = useState<null | HTMLElement>(null);
  const [twofaOpen, setTwofaOpen] = useState(false);
  const [orgs, setOrgs] = useState<OrgInfo[]>([]);
  const [orgEl, setOrgEl] = useState<null | HTMLElement>(null);

  useEffect(() => {
    api.orgs().then((list) => {
      setOrgs(list);
      if (!activeOrg.get() && list.length) {
        const def = list.find((o) => o.slug === "default") ?? list[0];
        activeOrg.set(String(def.id));
      }
    }).catch(() => {});
  }, []);

  const curOrg = orgs.find((o) => String(o.id) === activeOrg.get());
  const switchOrg = (id: number) => {
    activeOrg.set(String(id));
    setOrgEl(null);
    window.location.reload(); // re-fetch every view against the new active org
  };

  const doLogout = async () => {
    setUserEl(null);
    await logout();
    nav("/login", { replace: true });
  };

  const doAddPasskey = async () => {
    setUserEl(null);
    const name = window.prompt(t("passkey.namePrompt"), "My passkey");
    if (name === null) return;
    try {
      await passkeyRegister(name || "Passkey");
      window.alert(t("passkey.added"));
    } catch {
      window.alert(t("passkey.failed"));
    }
  };

  const isActive = (to: string) =>
    to === "/" ? loc.pathname === "/" : loc.pathname.startsWith(to);

  const go = (to: string) => { nav(to); if (mobile) setOpen(false); };
  const cycleMode = () => setMode(mode === "light" ? "dark" : mode === "dark" ? "auto" : "light");
  const modeIcon = mode === "light" ? <LightModeIcon /> : mode === "dark" ? <DarkModeIcon /> : <BrightnessAutoIcon />;

  const drawer = (
    <Box sx={{ width: DRAWER_W, height: "100%", display: "flex", flexDirection: "column" }}>
      <Toolbar sx={{ gap: 1.5 }}>
        <Box sx={{ width: 30, height: 30, borderRadius: "50%", bgcolor: "error.main" }} />
        <Typography variant="h6" fontWeight={500}>AlertHub</Typography>
      </Toolbar>
      <List sx={{ flex: 1, overflowY: "auto", px: 1 }}>
        {NAV.map((sec) => (
          <li key={sec.section}>
            <ListSubheader disableSticky sx={{ bgcolor: "transparent", fontSize: 11, letterSpacing: 1, textTransform: "uppercase" }}>
              {t(sec.section)}
            </ListSubheader>
            {sec.items.map((it) => (
              <ListItemButton
                key={it.to}
                selected={isActive(it.to)}
                onClick={() => go(it.to)}
                sx={{ borderRadius: 9999, mb: 0.5, "&.Mui-selected": { bgcolor: "primary.main", color: "primary.contrastText", "& .MuiListItemIcon-root": { color: "primary.contrastText" }, "&:hover": { bgcolor: "primary.main" } } }}
              >
                <ListItemIcon sx={{ minWidth: 40 }}>{it.icon}</ListItemIcon>
                <ListItemText primary={t(it.key)} />
                {it.soon && <Typography variant="caption" sx={{ opacity: 0.6 }}>{t("common.comingSoon")}</Typography>}
              </ListItemButton>
            ))}
          </li>
        ))}
      </List>
    </Box>
  );

  return (
    <Box sx={{ display: "flex", height: "100vh", bgcolor: "background.default" }}>
      <AppBar position="fixed" sx={{ zIndex: theme.zIndex.drawer + 1 }}>
        <Toolbar sx={{ gap: 1 }}>
          {mobile && (
            <IconButton edge="start" onClick={() => setOpen(true)}><MenuIcon /></IconButton>
          )}
          <Typography variant="h6" fontWeight={500} sx={{ flex: 1 }}>{t("app.title")}</Typography>
          {orgs.length > 1 && (
            <>
              <Box
                onClick={(e) => setOrgEl(e.currentTarget)}
                sx={{ display: "flex", alignItems: "center", gap: 0.5, cursor: "pointer", px: 1, py: 0.5, borderRadius: 2, "&:hover": { bgcolor: "rgba(255,255,255,0.12)" } }}
              >
                <ApartmentIcon fontSize="small" />
                <Typography variant="body2" sx={{ maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {curOrg?.name || t("org.select")}
                </Typography>
                <ArrowDropDownIcon fontSize="small" />
              </Box>
              <Menu anchorEl={orgEl} open={!!orgEl} onClose={() => setOrgEl(null)}>
                {orgs.map((o) => (
                  <MenuItem key={o.id} selected={String(o.id) === activeOrg.get()} onClick={() => switchOrg(o.id)}>
                    <ListItemIcon><ApartmentIcon fontSize="small" /></ListItemIcon>
                    {o.name}
                  </MenuItem>
                ))}
              </Menu>
            </>
          )}
          <Tooltip title="Language">
            <IconButton onClick={() => i18n.changeLanguage(i18n.language.startsWith("zh") ? "en-US" : "zh-CN")}>
              <TranslateIcon />
            </IconButton>
          </Tooltip>
          <Tooltip title={t(`appearance.${mode}`)}>
            <IconButton onClick={cycleMode}>{modeIcon}</IconButton>
          </Tooltip>
          <Tooltip title={t("appearance.color")}>
            <IconButton onClick={(e) => setColorEl(e.currentTarget)}><PaletteIcon /></IconButton>
          </Tooltip>
          <Menu anchorEl={colorEl} open={!!colorEl} onClose={() => setColorEl(null)}>
            {PRESETS.map((p) => (
              <MenuItem key={p.id} onClick={() => { setSeed(p.seed); setColorEl(null); }}>
                <Box sx={{ width: 16, height: 16, borderRadius: "50%", bgcolor: p.seed, mr: 1.5 }} />{p.label}
              </MenuItem>
            ))}
          </Menu>
          <IconButton onClick={(e) => setUserEl(e.currentTarget)} sx={{ ml: 0.5 }}>
            <Avatar sx={{ width: 32, height: 32, bgcolor: "secondary.main" }}>
              {(user?.upn || "A").slice(0, 1).toUpperCase()}
            </Avatar>
          </IconButton>
          <Menu anchorEl={userEl} open={!!userEl} onClose={() => setUserEl(null)}>
            <MenuItem disabled sx={{ opacity: "1 !important" }}>
              <Box>
                <Typography variant="body2" fontWeight={500}>{user?.upn || "—"}</Typography>
                <Typography variant="caption" color="text.secondary">{user?.role}</Typography>
              </Box>
            </MenuItem>
            <MenuItem onClick={doAddPasskey}>
              <ListItemIcon><FingerprintIcon fontSize="small" /></ListItemIcon>
              {t("passkey.add")}
            </MenuItem>
            <MenuItem onClick={() => { setUserEl(null); setTwofaOpen(true); }}>
              <ListItemIcon><ShieldIcon fontSize="small" /></ListItemIcon>
              {t("twofa.menu")}
            </MenuItem>
            <MenuItem onClick={doLogout}>
              <ListItemIcon><LogoutIcon fontSize="small" /></ListItemIcon>
              {t("logout")}
            </MenuItem>
          </Menu>
        </Toolbar>
      </AppBar>

      <Box component="nav" sx={{ width: { md: DRAWER_W }, flexShrink: { md: 0 } }}>
        <Drawer
          variant={mobile ? "temporary" : "permanent"}
          open={mobile ? open : true}
          onClose={() => setOpen(false)}
          ModalProps={{ keepMounted: true }}
          sx={{ "& .MuiDrawer-paper": { width: DRAWER_W, boxSizing: "border-box" } }}
        >
          {drawer}
        </Drawer>
      </Box>

      <Box component="main" sx={{ flexGrow: 1, minWidth: 0, overflowY: "auto", height: "100vh" }}>
        <Toolbar />
        <Box sx={{ p: { xs: 2, md: 3 } }}><Outlet /></Box>
      </Box>

      <TwoFADialog open={twofaOpen} onClose={() => setTwofaOpen(false)} />
    </Box>
  );
}
