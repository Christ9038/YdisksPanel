export interface User {
  id: number;
  username: string;
  displayName: string;
  role: "admin" | "user";
  disabled: boolean;
  totpEnabled: boolean;
  createdAt: number;
  teams: number[];
}
export interface Me {
  user: User;
  mustEnroll2fa: boolean;
}
export interface Bootstrap {
  needsSetup: boolean;
  siteName: string;
  wallpaper: string;
}
export type Zone = "lan" | "wan";
export interface Network {
  ip: string;
  zone: Zone;
}
export interface Group {
  id: number;
  name: string;
  sort: number;
}
export interface Service {
  id: number;
  groupId: number;
  name: string;
  description: string;
  icon: string;
  color: string;
  iconFit: "contain" | "cover";
  lanUrl: string;
  wanUrl: string;
  newTab: boolean;
  healthCheck: boolean;
  sort: number;
  teams: number[];
  status: "up" | "down" | "unknown";
}
export interface Dashboard {
  groups: Group[];
  services: Service[];
}
export interface Team {
  id: number;
  name: string;
}
export interface SessionInfo {
  id: string;
  createdAt: number;
  lastSeen: number;
  expiresAt: number;
  ip: string;
  userAgent: string;
  current: boolean;
}
export interface AuditEntry {
  id: number;
  ts: number;
  username: string;
  action: string;
  detail: string;
  ip: string;
}
export interface Settings {
  siteName: string;
  wallpaper: string;
  require2fa: boolean;
  lanCidrs: string[];
}
