import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronDown, Globe, Home as HomeIcon, LogOut, Monitor, Moon, Pencil, Search, Settings, Sun, Wifi } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { post } from "@/lib/api";
import { Logo } from "@/components/ui";
import { useBootstrap, useMe } from "@/lib/hooks";
import type { NetMode } from "@/lib/links";
import { useTheme } from "@/lib/theme";
import type { Network, Zone } from "@/lib/types";

const modeText: Record<NetMode, string> = { auto: "自动", lan: "强制内网", wan: "强制公网" };

function Popover({ trigger, children }: { trigger: (o: boolean) => React.ReactNode; children: (close: () => void) => React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const click = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    const key = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", click);
    document.addEventListener("keydown", key);
    return () => { document.removeEventListener("mousedown", click); document.removeEventListener("keydown", key); };
  }, [open]);
  return (
    <div className="relative" ref={ref}>
      <div onClick={() => setOpen(!open)}>{trigger(open)}</div>
      {open && <div className="glass-strong absolute right-0 mt-2 min-w-48 rounded-2xl p-1.5 z-30">{children(() => setOpen(false))}</div>}
    </div>
  );
}

const Item = ({ active, onClick, icon, children }: { active?: boolean; onClick: () => void; icon?: React.ReactNode; children: React.ReactNode }) => (
  <button onClick={onClick} className="w-full flex items-center gap-2.5 px-3 py-2 rounded-xl text-sm text-left cursor-pointer hover:bg-[color-mix(in_srgb,var(--accent)_14%,transparent)]">
    {icon}<span className="flex-1">{children}</span>{active && <Check size={14} className="text-[var(--accent)]" />}
  </button>
);

export function TopBar({ net, zone, mode, setMode, edit, setEdit, onSearch }: {
  net?: Network; zone: Zone; mode: NetMode; setMode: (m: NetMode) => void; edit: boolean; setEdit: (e: boolean) => void; onSearch: () => void;
}) {
  const { data: boot } = useBootstrap();
  const { data: me } = useMe();
  const qc = useQueryClient();
  const nav = useNavigate();
  const { mode: tm, setMode: setTm } = useTheme();
  const logout = useMutation({
    mutationFn: () => post("/api/auth/logout"),
    onSuccess: async () => { qc.clear(); nav("/login", { replace: true }); },
  });
  const isAdmin = me?.user.role === "admin";
  const ThemeIcon = tm === "dark" ? Moon : tm === "light" ? Sun : Monitor;

  return (
    <header className="glass sticky top-3 z-20 mx-auto max-w-6xl rounded-2xl px-3 sm:px-4 h-14 flex items-center gap-2 sm:gap-3">
      <Link to="/" className="flex items-center gap-2.5 font-semibold mr-1">
        <Logo size={32} />
        <span className="hidden sm:inline">{boot?.siteName ?? "YdisksPanel"}</span>
      </Link>

      <button onClick={onSearch} className="btn flex-1 min-w-0 max-w-md justify-start muted !font-normal whitespace-nowrap" aria-label="搜索服务">
        <Search size={15} className="shrink-0" /><span className="flex-1 text-left truncate hidden sm:inline">搜索服务</span>
        <kbd className="hidden sm:inline text-[11px] px-1.5 py-0.5 rounded-md border border-[var(--line)]">⌘K</kbd>
      </button>
      <div className="flex-1 hidden md:block" />

      <Popover trigger={(o) => (
        <button className="btn btn-sm gap-1.5" aria-haspopup="menu" aria-expanded={o} title={net ? `你的 IP：${net.ip}` : undefined}>
          {zone === "lan" ? <HomeIcon size={14} className="text-emerald-500" /> : <Globe size={14} className="text-sky-500" />}
          <span className="hidden sm:inline">{zone === "lan" ? "内网" : "公网"}{mode !== "auto" && "·手动"}</span>
          <ChevronDown size={13} className="muted" />
        </button>
      )}>
        {(close) => (
          <>
            <div className="px-3 py-2 text-xs muted"><Wifi size={12} className="inline mr-1" />检测到：{net ? `${net.zone === "lan" ? "内网" : "公网"} · ${net.ip}` : "…"}</div>
            {(["auto", "lan", "wan"] as NetMode[]).map((m) => (
              <Item key={m} active={mode === m} onClick={() => { setMode(m); close(); }}>{modeText[m]}</Item>
            ))}
          </>
        )}
      </Popover>

      <Popover trigger={() => <button className="btn btn-sm btn-icon" aria-label="切换主题"><ThemeIcon size={15} /></button>}>
        {(close) => (
          <>
            <Item active={tm === "system"} icon={<Monitor size={15} />} onClick={() => { setTm("system"); close(); }}>跟随系统</Item>
            <Item active={tm === "light"} icon={<Sun size={15} />} onClick={() => { setTm("light"); close(); }}>浅色</Item>
            <Item active={tm === "dark"} icon={<Moon size={15} />} onClick={() => { setTm("dark"); close(); }}>深色</Item>
          </>
        )}
      </Popover>

      {isAdmin && (
        <button className={`btn btn-sm ${edit ? "btn-primary" : ""}`} onClick={() => setEdit(!edit)} aria-pressed={edit}>
          {edit ? <Check size={14} /> : <Pencil size={14} />}<span className="hidden sm:inline">{edit ? "完成" : "编辑"}</span>
        </button>
      )}

      <Popover trigger={() => (
        <button className="h-8 w-8 rounded-full grid place-items-center text-sm font-semibold text-white cursor-pointer" style={{ background: "linear-gradient(135deg,var(--accent),var(--accent-2))" }} aria-label="账户菜单">
          {me?.user.displayName.charAt(0).toUpperCase()}
        </button>
      )}>
        {(close) => (
          <>
            <div className="px-3 py-2"><div className="text-sm font-medium">{me?.user.displayName}</div><div className="text-xs muted">@{me?.user.username}</div></div>
            <Item icon={<Settings size={15} />} onClick={() => { close(); nav("/settings"); }}>设置</Item>
            <Item icon={<LogOut size={15} />} onClick={() => logout.mutate()}>退出登录</Item>
          </>
        )}
      </Popover>
    </header>
  );
}
