import { create } from "zustand";

type Mode = "light" | "dark" | "auto";

const COLOR_KEY = "alerthub-admin-color";
const MODE_KEY = "alerthub-admin-mode";

type AppearanceState = {
  seed: string;
  mode: Mode;
  setSeed: (s: string) => void;
  setMode: (m: Mode) => void;
};

export const useAppearance = create<AppearanceState>((set) => ({
  seed: localStorage.getItem(COLOR_KEY) || "#2563EB",
  mode: (localStorage.getItem(MODE_KEY) as Mode) || "auto",
  setSeed: (s) => {
    localStorage.setItem(COLOR_KEY, s);
    set({ seed: s });
  },
  setMode: (m) => {
    localStorage.setItem(MODE_KEY, m);
    set({ mode: m });
  },
}));
