import { useEffect, useMemo, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import {
  App as AntApp, Avatar, Button, Drawer, Dropdown, Layout, Menu, Tooltip, Typography, theme,
} from "antd";
import type { MenuProps } from "antd";
import {
  ApartmentOutlined, BgColorsOutlined, BulbOutlined, DashboardOutlined, DownOutlined,
  HistoryOutlined, KeyOutlined, LogoutOutlined, MenuOutlined, MobileOutlined, MoonOutlined,
  NotificationOutlined, RadarChartOutlined, SafetyCertificateOutlined, SettingOutlined,
  SunOutlined, TranslationOutlined,
} from "@ant-design/icons";
import { useTranslation } from "react-i18next";
import { api, activeOrg, type OrgInfo } from "@/api";
import { useAppearance } from "@/stores/appearance";
import { useAuth } from "@/stores/auth";
import { passkeyRegister } from "@/passkey";
import { PRESETS, buildTokens } from "@/theme";
import TwoFADialog from "@/components/TwoFADialog";

const { Header, Sider, Content } = Layout;

const DRAWER_W = 256;
// Mirrors the previous MUI `theme.breakpoints.down("md")` (md = 900px).
const MOBILE_QUERY = "(max-width: 899.95px)";

type NavItem = { to: string; icon: React.ReactNode; key: string; soon?: boolean };
type NavSection = { section: string; items: NavItem[] };

// Paths are relative to the router basename ("/admin").
const NAV: NavSection[] = [
  { section: "nav.section.overview", items: [
    { to: "/", icon: <DashboardOutlined />, key: "nav.dashboard" },
  ]},
  { section: "nav.section.ops", items: [
    { to: "/publish", icon: <NotificationOutlined />, key: "nav.publish" },
    { to: "/devices", icon: <MobileOutlined />, key: "nav.devices", soon: true },
    { to: "/history", icon: <HistoryOutlined />, key: "nav.history", soon: true },
  ]},
  { section: "nav.section.config", items: [
    { to: "/sources", icon: <RadarChartOutlined />, key: "nav.sources", soon: true },
    { to: "/settings", icon: <SettingOutlined />, key: "nav.settings" },
  ]},
];

function useIsMobile(): boolean {
  const [mobile, setMobile] = useState(
    () => typeof window !== "undefined" && window.matchMedia(MOBILE_QUERY).matches,
  );
  useEffect(() => {
    const mq = window.matchMedia(MOBILE_QUERY);
    const onChange = () => setMobile(mq.matches);
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);
  return mobile;
}

export default function AdminLayout() {
  const { token } = theme.useToken();
  const { message } = AntApp.useApp();
  const { t, i18n } = useTranslation();
  const { seed, mode, setMode, setSeed } = useAppearance();
  const { user, logout } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();
  const mobile = useIsMobile();
  const [open, setOpen] = useState(false);
  const [twofaOpen, setTwofaOpen] = useState(false);
  const [orgs, setOrgs] = useState<OrgInfo[]>([]);

  // M3 surface tones that antd has no token for (see theme/index.ts#buildTokens).
  const m3 = useMemo(() => buildTokens(seed, mode), [seed, mode]);

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
    window.location.reload(); // re-fetch every view against the new active org
  };

  const doLogout = async () => {
    await logout();
    nav("/login", { replace: true });
  };

  const doAddPasskey = async () => {
    const name = window.prompt(t("passkey.namePrompt"), "My passkey");
    if (name === null) return;
    try {
      await passkeyRegister(name || "Passkey");
      message.success(t("passkey.added"));
    } catch {
      message.error(t("passkey.failed"));
    }
  };

  const isActive = (to: string) =>
    to === "/" ? loc.pathname === "/" : loc.pathname.startsWith(to);

  const go = (to: string) => { nav(to); if (mobile) setOpen(false); };
  const cycleMode = () => setMode(mode === "light" ? "dark" : mode === "dark" ? "auto" : "light");
  const modeIcon = mode === "light" ? <SunOutlined /> : mode === "dark" ? <MoonOutlined /> : <BulbOutlined />;

  const navItems: MenuProps["items"] = NAV.map((sec) => ({
    key: sec.section,
    type: "group" as const,
    label: (
      <span style={{ letterSpacing: 1, textTransform: "uppercase" }}>{t(sec.section)}</span>
    ),
    children: sec.items.map((it) => ({
      key: it.to,
      icon: it.icon,
      label: (
        <span style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
          <span style={{ overflow: "hidden", textOverflow: "ellipsis" }}>{t(it.key)}</span>
          {it.soon && (
            <span style={{ fontSize: 12, opacity: 0.6, flex: "0 0 auto" }}>{t("common.comingSoon")}</span>
          )}
        </span>
      ),
    })),
  }));

  const selectedKeys = NAV.flatMap((s) => s.items).filter((it) => isActive(it.to)).map((it) => it.to);

  const orgItems: MenuProps["items"] = orgs.map((o) => ({
    key: String(o.id),
    icon: <ApartmentOutlined />,
    label: o.name,
  }));

  const colorItems: MenuProps["items"] = PRESETS.map((p) => ({
    key: p.id,
    label: (
      <span style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span style={{ width: 16, height: 16, borderRadius: "50%", background: p.seed, flex: "0 0 auto" }} />
        {p.label}
      </span>
    ),
  }));

  const userItems: MenuProps["items"] = [
    {
      key: "identity",
      type: "group",
      label: (
        <span style={{ display: "block", paddingBlock: 4 }}>
          {/* explicit font sizes: the Menu.groupTitleFontSize token (11) is tuned
              for the nav rail's section headers and would otherwise apply here. */}
          <span style={{ display: "block", fontSize: 14, fontWeight: 500, color: token.colorText }}>
            {user?.upn || "—"}
          </span>
          <span style={{ display: "block", fontSize: 12, color: token.colorTextSecondary }}>
            {user?.role}
          </span>
        </span>
      ),
    },
    { type: "divider" },
    { key: "passkey", icon: <KeyOutlined />, label: t("passkey.add") },
    { key: "twofa", icon: <SafetyCertificateOutlined />, label: t("twofa.menu") },
    { key: "logout", icon: <LogoutOutlined />, label: t("logout") },
  ];

  const onUserMenu: MenuProps["onClick"] = ({ key }) => {
    if (key === "passkey") void doAddPasskey();
    else if (key === "twofa") setTwofaOpen(true);
    else if (key === "logout") void doLogout();
  };

  const sideNav = (
    <div style={{ height: "100%", display: "flex", flexDirection: "column" }}>
      <div style={{ height: 64, flex: "0 0 auto", display: "flex", alignItems: "center", gap: 12, padding: "0 20px" }}>
        <div style={{ width: 30, height: 30, borderRadius: "50%", background: token.colorError, flex: "0 0 auto" }} />
        <Typography.Title level={5} style={{ margin: 0, fontWeight: 500 }}>AlertHub</Typography.Title>
      </div>
      <div style={{ flex: 1, minHeight: 0, overflowY: "auto", paddingBottom: 8 }}>
        <Menu
          mode="inline"
          items={navItems}
          selectedKeys={selectedKeys}
          onClick={({ key }) => go(key)}
          style={{ background: "transparent", borderInlineEnd: "none" }}
        />
      </div>
    </div>
  );

  return (
    <Layout style={{ height: "100vh" }}>
      <Header style={{ display: "flex", alignItems: "center", gap: 8, borderBottom: `1px solid ${token.colorSplit}`, flex: "0 0 auto" }}>
        {mobile && (
          <Button type="text" shape="circle" icon={<MenuOutlined />} onClick={() => setOpen(true)} />
        )}
        <Typography.Title
          level={4}
          style={{ margin: 0, flex: 1, minWidth: 0, fontWeight: 500, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
        >
          {t("app.title")}
        </Typography.Title>

        {orgs.length > 1 && (
          <Dropdown
            trigger={["click"]}
            menu={{ items: orgItems, selectable: true, selectedKeys: [activeOrg.get()], onClick: ({ key }) => switchOrg(Number(key)) }}
          >
            <Button type="text" icon={<ApartmentOutlined />}>
              <span style={{ display: "inline-block", maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", verticalAlign: "bottom" }}>
                {curOrg?.name || t("org.select")}
              </span>
              <DownOutlined style={{ fontSize: 12 }} />
            </Button>
          </Dropdown>
        )}

        <Tooltip title="Language">
          <Button
            type="text"
            shape="circle"
            icon={<TranslationOutlined />}
            onClick={() => i18n.changeLanguage(i18n.language.startsWith("zh") ? "en-US" : "zh-CN")}
          />
        </Tooltip>
        <Tooltip title={t(`appearance.${mode}`)}>
          <Button type="text" shape="circle" icon={modeIcon} onClick={cycleMode} />
        </Tooltip>
        <Dropdown
          trigger={["click"]}
          menu={{
            items: colorItems,
            onClick: ({ key }) => {
              const p = PRESETS.find((x) => x.id === key);
              if (p) setSeed(p.seed);
            },
          }}
        >
          {/* span wrapper: Dropdown injects onClick/ref onto its direct child,
              which a Tooltip would not forward down to the Button. */}
          <span style={{ display: "inline-flex" }}>
            <Tooltip title={t("appearance.color")}>
              <Button type="text" shape="circle" icon={<BgColorsOutlined />} />
            </Tooltip>
          </span>
        </Dropdown>

        <Dropdown trigger={["click"]} menu={{ items: userItems, onClick: onUserMenu }}>
          <span style={{ cursor: "pointer", display: "inline-flex", marginInlineStart: 4 }}>
            <Avatar size={32} style={{ backgroundColor: m3.secondary, color: m3.onSecondary }}>
              {(user?.upn || "A").slice(0, 1).toUpperCase()}
            </Avatar>
          </span>
        </Dropdown>
      </Header>

      <Layout style={{ minHeight: 0 }}>
        {mobile ? (
          <Drawer
            placement="left"
            open={open}
            onClose={() => setOpen(false)}
            width={DRAWER_W}
            closable={false}
            styles={{ body: { padding: 0, background: m3.surfaceContainerLow } }}
          >
            {sideNav}
          </Drawer>
        ) : (
          <Sider width={DRAWER_W} style={{ borderInlineEnd: `1px solid ${token.colorSplit}` }}>
            {sideNav}
          </Sider>
        )}

        <Content style={{ minWidth: 0, overflowY: "auto", padding: mobile ? 16 : 24 }}>
          <Outlet />
        </Content>
      </Layout>

      <TwoFADialog open={twofaOpen} onClose={() => setTwofaOpen(false)} />
    </Layout>
  );
}
