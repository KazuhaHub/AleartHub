// Material Design 3 token generation from a single seed color, like the
// Passwall panel. Uses @material/material-color-utilities (no color-picker libs).
import {
  argbFromHex,
  hexFromArgb,
  themeFromSourceColor,
} from "@material/material-color-utilities";

export type M3Tokens = {
  primary: string;
  onPrimary: string;
  primaryContainer: string;
  onPrimaryContainer: string;
  secondary: string;
  onSecondary: string;
  tertiary: string;
  error: string;
  onError: string;
  background: string;
  onBackground: string;
  surface: string;
  onSurface: string;
  surfaceVariant: string;
  onSurfaceVariant: string;
  surfaceContainerLow: string;
  surfaceContainer: string;
  surfaceContainerHigh: string;
  outline: string;
  outlineVariant: string;
};

export function tokensFromSource(seedHex: string, dark: boolean): M3Tokens {
  const t = themeFromSourceColor(argbFromHex(seedHex));
  const s = dark ? t.schemes.dark : t.schemes.light;
  const h = (argb: number) => hexFromArgb(argb);
  const neutral = t.palettes.neutral;
  const tone = (x: number) => hexFromArgb(neutral.tone(x));
  return {
    primary: h(s.primary),
    onPrimary: h(s.onPrimary),
    primaryContainer: h(s.primaryContainer),
    onPrimaryContainer: h(s.onPrimaryContainer),
    secondary: h(s.secondary),
    onSecondary: h(s.onSecondary),
    tertiary: h(s.tertiary),
    error: h(s.error),
    onError: h(s.onError),
    background: h(s.background),
    onBackground: h(s.onBackground),
    surface: h(s.surface),
    onSurface: h(s.onSurface),
    surfaceVariant: h(s.surfaceVariant),
    onSurfaceVariant: h(s.onSurfaceVariant),
    // M3 surface-container tones derived from the neutral tonal palette
    surfaceContainerLow: dark ? tone(10) : tone(96),
    surfaceContainer: dark ? tone(12) : tone(94),
    surfaceContainerHigh: dark ? tone(17) : tone(92),
    outline: h(s.outline),
    outlineVariant: h(s.outlineVariant),
  };
}
