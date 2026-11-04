import type { Service } from "./types";

/** Preset background colors for service icons (empty string = theme gradient). */
export const COLOR_PRESETS = [
  "#0a8cf8", "#2f7cff", "#06b6d4", "#14b8a6", "#10b981", "#84cc16",
  "#f59e0b", "#f97316", "#ef4444", "#ec4899", "#64748b", "#1e293b", "#111827", "#ffffff",
];

/** Readable foreground (white/near-black) for a #rrggbb background, by WCAG luminance. */
export function contrastFg(hex: string): string {
  const m = /^#([0-9a-f]{6})$/i.exec(hex);
  if (!m) return "#fff";
  const n = parseInt(m[1], 16);
  const lin = (c: number) => {
    c /= 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  const L = 0.2126 * lin((n >> 16) & 255) + 0.7152 * lin((n >> 8) & 255) + 0.0722 * lin(n & 255);
  return L > 0.5 ? "#111827" : "#fff";
}

/** Cache-busting token so changing a service's URL refetches its favicon. */
function hash(s: string) {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return (h >>> 0).toString(36);
}

export function faviconUrl(s: Pick<Service, "id" | "lanUrl" | "wanUrl">) {
  return `/api/services/${s.id}/favicon?v=${hash(s.lanUrl || s.wanUrl)}`;
}
