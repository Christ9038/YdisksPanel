import type { Service, Zone } from "./types";

export type NetMode = "auto" | "lan" | "wan";
const KEY = "ydiskspanel.netmode";

export function loadNetMode(): NetMode {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "lan" || v === "wan") return v;
  } catch {}
  return "auto";
}
export function saveNetMode(m: NetMode) {
  try {
    localStorage.setItem(KEY, m);
  } catch {}
}

export interface Resolved {
  url: string;
  line: Zone;
  /** true when only one address is configured, so no choice was made */
  only: boolean;
  other?: { url: string; line: Zone };
}

/** Pick the address for the current network. A single configured address is always used. */
export function resolveLink(s: Pick<Service, "lanUrl" | "wanUrl">, zone: Zone): Resolved {
  if (s.lanUrl && !s.wanUrl) return { url: s.lanUrl, line: "lan", only: true };
  if (s.wanUrl && !s.lanUrl) return { url: s.wanUrl, line: "wan", only: true };
  const lan = { url: s.lanUrl, line: "lan" as Zone };
  const wan = { url: s.wanUrl, line: "wan" as Zone };
  return zone === "lan"
    ? { ...lan, only: false, other: wan }
    : { ...wan, only: false, other: lan };
}
