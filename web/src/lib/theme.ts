import { useEffect, useState } from "react";

export type Mode = "system" | "light" | "dark";
export const ACCENTS = ["brand", "cyan", "emerald", "amber", "rose", "violet"] as const;
export type Accent = (typeof ACCENTS)[number];

const readAccent = (): Accent => {
  const v = read("ydiskspanel.accent", "brand");
  return (ACCENTS as readonly string[]).includes(v) ? (v as Accent) : "brand"; // migrates the old "indigo"/"sky" values
};

const read = (k: string, d: string) => {
  try {
    return localStorage.getItem(k) ?? d;
  } catch {
    return d;
  }
};
const write = (k: string, v: string) => {
  try {
    localStorage.setItem(k, v);
  } catch {}
};

export function applyTheme(mode: Mode, accent: Accent) {
  const dark = mode === "dark" || (mode === "system" && matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.dataset.accent = accent;
}

export function initTheme() {
  applyTheme(read("ydiskspanel.mode", "system") as Mode, readAccent());
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () =>
    applyTheme(read("ydiskspanel.mode", "system") as Mode, readAccent()),
  );
}

export function useTheme() {
  const [mode, setMode] = useState<Mode>(() => read("ydiskspanel.mode", "system") as Mode);
  const [accent, setAccent] = useState<Accent>(() => readAccent());
  useEffect(() => {
    write("ydiskspanel.mode", mode);
    write("ydiskspanel.accent", accent);
    applyTheme(mode, accent);
  }, [mode, accent]);
  return { mode, setMode, accent, setAccent };
}
