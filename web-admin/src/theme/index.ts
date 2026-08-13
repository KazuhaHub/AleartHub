// Material Design 3 -> Ant Design 6 theme adapter.
//
// ./tokens.ts stays framework-agnostic (plain hex strings generated from a seed
// colour by @material/material-color-utilities). THIS file is the only place
// that knows about antd. Components should read colours from antd's own tokens
// (`theme.useToken()`), not from M3 directly -- see buildTokens() for the escape
// hatch when an M3 surface colour has no antd equivalent.
import { theme as antdTheme } from "antd";
import type { ThemeConfig } from "antd";
import { tokensFromSource, type M3Tokens } from "./tokens";

export type { M3Tokens };

export type AppearanceMode = "light" | "dark" | "auto";

// @fontsource/roboto is no longer installed, so the system stack leads and
// Roboto is only used when the OS happens to have it.
export const FONT_FAMILY =
  '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans", "Microsoft YaHei", sans-serif';

// Color presets, mirroring the Passwall panel's preset approach.
export const PRESETS: { id: string; label: string; seed: string }[] = [
  { id: "blue", label: "Blue", seed: "#2563EB" },
  { id: "purple", label: "Purple", seed: "#6750A4" },
  { id: "teal", label: "Teal", seed: "#006A6B" },
  { id: "green", label: "Green", seed: "#386A20" },
  { id: "orange", label: "Orange", seed: "#825500" },
  { id: "red", label: "Red", seed: "#B3261E" },
];

export function resolveDark(mode: AppearanceMode): boolean {
  if (mode === "auto") {
    return !!(
      typeof window !== "undefined" &&
      window.matchMedia?.("(prefers-color-scheme: dark)").matches
    );
  }
  return mode === "dark";
}

/**
 * Raw M3 tokens for a seed + mode. Use ONLY when you need an M3 surface tone
 * that antd has no token for (surfaceContainer / primaryContainer / tertiary...).
 * For ordinary colours prefer `const { token } = theme.useToken()`.
 */
export function buildTokens(seed: string, mode: AppearanceMode): M3Tokens {
  return tokensFromSource(seed, resolveDark(mode));
}

/** Build the ConfigProvider theme config for a seed + mode. */
export function buildTheme(seed: string, mode: AppearanceMode): ThemeConfig {
  const dark = resolveDark(mode);
  const m = tokensFromSource(seed, dark);
  return {
    algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      // --- seed tokens: the algorithm derives the rest of the scale from these
      colorPrimary: m.primary,
      colorError: m.error,
      colorInfo: m.primary,
      colorBgBase: m.background,
      colorTextBase: m.onSurface,
      borderRadius: 12,
      fontFamily: FONT_FAMILY,
      // --- map/alias overrides: applied on top of the algorithm's output so the
      //     M3 surface/outline tones win over antd's generated greys.
      colorBgLayout: m.background,
      colorBgContainer: m.surfaceContainerLow,
      colorBgElevated: m.surfaceContainerHigh,
      colorText: m.onSurface,
      colorTextSecondary: m.onSurfaceVariant,
      colorTextTertiary: m.onSurfaceVariant,
      colorTextDescription: m.onSurfaceVariant,
      colorTextHeading: m.onSurface,
      colorBorder: m.outlineVariant,
      colorBorderSecondary: m.outlineVariant,
      colorSplit: m.outlineVariant,
    },
    components: {
      // M3 buttons are pills, flat (the old MUI theme used disableElevation).
      Button: {
        borderRadius: 9999,
        borderRadiusLG: 9999,
        borderRadiusSM: 9999,
        paddingInline: 18,
        paddingInlineLG: 22,
        fontWeight: 500,
        primaryShadow: "none",
        defaultShadow: "none",
        dangerShadow: "none",
      },
      Card: {
        borderRadiusLG: 16,
        colorBorderSecondary: m.outlineVariant,
      },
      Layout: {
        headerBg: m.surfaceContainer,
        headerColor: m.onSurface,
        headerHeight: 64,
        headerPadding: "0 16px",
        bodyBg: m.background,
        siderBg: m.surfaceContainerLow,
        triggerBg: m.surfaceContainerHigh,
        triggerColor: m.onSurface,
      },
      // Nav rail: pill-shaped rows, selected row filled with the primary colour
      // (matches the previous MUI ListItemButton "Mui-selected" override).
      Menu: {
        itemBg: "transparent",
        subMenuItemBg: "transparent",
        popupBg: m.surfaceContainerHigh,
        itemBorderRadius: 9999,
        itemHeight: 44,
        itemMarginInline: 8,
        itemColor: m.onSurfaceVariant,
        itemHoverColor: m.onSurface,
        itemSelectedBg: m.primary,
        itemSelectedColor: m.onPrimary,
        groupTitleColor: m.onSurfaceVariant,
        groupTitleFontSize: 11,
        activeBarWidth: 0,
        activeBarBorderWidth: 0,
      },
      Modal: {
        contentBg: m.surfaceContainerHigh,
        headerBg: m.surfaceContainerHigh,
        titleColor: m.onSurface,
      },
    },
  };
}
