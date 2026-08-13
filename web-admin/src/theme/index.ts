import { createTheme, type Theme } from "@mui/material/styles";
import { tokensFromSource, type M3Tokens } from "./tokens";

// Make theme.m3 (full M3 token set) available to components.
declare module "@mui/material/styles" {
  interface Theme {
    m3: M3Tokens;
  }
  interface ThemeOptions {
    m3?: M3Tokens;
  }
}

// Color presets, mirroring the Passwall panel's preset approach.
export const PRESETS: { id: string; label: string; seed: string }[] = [
  { id: "blue", label: "Blue", seed: "#2563EB" },
  { id: "purple", label: "Purple", seed: "#6750A4" },
  { id: "teal", label: "Teal", seed: "#006A6B" },
  { id: "green", label: "Green", seed: "#386A20" },
  { id: "orange", label: "Orange", seed: "#825500" },
  { id: "red", label: "Red", seed: "#B3261E" },
];

export function resolveDark(mode: "light" | "dark" | "auto"): boolean {
  if (mode === "auto") {
    return typeof window !== "undefined" &&
      window.matchMedia?.("(prefers-color-scheme: dark)").matches;
  }
  return mode === "dark";
}

export function buildTheme(seed: string, mode: "light" | "dark" | "auto"): Theme {
  const dark = resolveDark(mode);
  const m = tokensFromSource(seed, dark);
  return createTheme({
    m3: m,
    palette: {
      mode: dark ? "dark" : "light",
      primary: { main: m.primary, contrastText: m.onPrimary },
      secondary: { main: m.secondary, contrastText: m.onSecondary },
      error: { main: m.error, contrastText: m.onError },
      background: { default: m.background, paper: m.surfaceContainerLow },
      text: { primary: m.onSurface, secondary: m.onSurfaceVariant },
      divider: m.outlineVariant,
    },
    shape: { borderRadius: 12 },
    typography: {
      fontFamily:
        'Roboto, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans", "Microsoft YaHei", sans-serif',
      fontWeightMedium: 500,
      button: { textTransform: "none", fontWeight: 500 },
    },
    components: {
      MuiButton: {
        defaultProps: { disableElevation: true },
        styleOverrides: { root: { borderRadius: 9999, paddingInline: 18 } },
      },
      MuiPaper: { styleOverrides: { root: { backgroundImage: "none" } } },
      MuiCard: {
        styleOverrides: {
          root: { borderRadius: 16, border: `1px solid ${m.outlineVariant}` },
        },
        defaultProps: { elevation: 0 },
      },
      MuiAppBar: {
        defaultProps: { elevation: 0, color: "default" },
        styleOverrides: {
          root: { backgroundColor: m.surfaceContainer, color: m.onSurface, borderBottom: `1px solid ${m.outlineVariant}` },
        },
      },
      MuiDrawer: {
        styleOverrides: { paper: { backgroundColor: m.surfaceContainerLow, borderColor: m.outlineVariant } },
      },
    },
  });
}
