import { Command } from "cmdk";
import { Search } from "lucide-react";
import { useEffect } from "react";
import { faviconUrl } from "@/lib/color";
import { resolveLink } from "@/lib/links";
import type { Service, Zone } from "@/lib/types";
import { openLink } from "./ServiceCard";
import { ServiceIcon } from "./ui";

export function CommandPalette({ open, setOpen, services, zone }: { open: boolean; setOpen: (o: boolean) => void; services: Service[]; zone: Zone }) {
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const typing = /input|textarea|select/i.test((e.target as HTMLElement)?.tagName ?? "");
      if ((e.key === "k" && (e.metaKey || e.ctrlKey)) || (e.key === "/" && !typing)) {
        e.preventDefault();
        setOpen(true);
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [setOpen]);

  return (
    <Command.Dialog open={open} onOpenChange={setOpen} label="搜索服务" overlayClassName="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
      contentClassName="glass-strong fixed z-50 left-1/2 top-[14vh] -translate-x-1/2 w-[min(92vw,34rem)] rounded-3xl overflow-hidden">
      <div className="flex items-center gap-3 px-5 border-b border-[var(--line)]">
        <Search size={18} className="muted" />
        <Command.Input autoFocus placeholder="搜索服务…" className="h-14 w-full bg-transparent !outline-none text-base" />
      </div>
      <Command.List className="max-h-[50vh] overflow-y-auto p-2">
        <Command.Empty className="py-10 text-center text-sm muted">没有匹配的服务</Command.Empty>
        {services.map((s) => {
          const l = resolveLink(s, zone);
          return (
            <Command.Item key={s.id} value={`${s.name} ${s.description}`}
              onSelect={() => { setOpen(false); openLink(l.url, s.newTab); }}
              className="flex items-center gap-3 rounded-xl px-3 py-2.5 cursor-pointer data-[selected=true]:bg-[color-mix(in_srgb,var(--accent)_16%,transparent)]">
              <ServiceIcon icon={s.icon} name={s.name} size={16} color={s.color} fit={s.iconFit} favicon={faviconUrl(s)} />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium truncate">{s.name}</div>
                {s.description && <div className="text-xs muted truncate">{s.description}</div>}
              </div>
              <span className="chip">{l.line === "lan" ? "内网" : "公网"}</span>
            </Command.Item>
          );
        })}
      </Command.List>
    </Command.Dialog>
  );
}
