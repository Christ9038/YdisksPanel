import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { motion } from "framer-motion";
import { ArrowLeftRight, GripVertical, Pencil, Trash2 } from "lucide-react";
import { faviconUrl } from "@/lib/color";
import { resolveLink } from "@/lib/links";
import type { Service, Zone } from "@/lib/types";
import { ServiceIcon } from "./ui";

const dot: Record<Service["status"], string> = {
  up: "bg-[var(--ok)] shadow-[0_0_8px_var(--ok)]",
  down: "bg-[var(--bad)]",
  unknown: "bg-slate-400/60",
};
const dotLabel = { up: "在线", down: "无法连接", unknown: "未检测" };

export function openLink(url: string, newTab: boolean) {
  if (newTab) window.open(url, "_blank", "noopener,noreferrer");
  else window.location.assign(url);
}

interface Props {
  service: Service;
  zone: Zone;
  edit: boolean;
  index: number;
  onEdit: (s: Service) => void;
  onDelete: (s: Service) => void;
}

export function ServiceCard({ service: s, zone, edit, index, onEdit, onDelete }: Props) {
  const sortable = useSortable({ id: `s-${s.id}`, disabled: !edit });
  const style = { transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition };
  const link = resolveLink(s, zone);
  const target = s.newTab ? "_blank" : undefined;

  const body = (
    <>
      <ServiceIcon icon={s.icon} name={s.name} color={s.color} fit={s.iconFit} favicon={faviconUrl(s)} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="font-semibold truncate">{s.name}</span>
          {s.healthCheck && <span className={`h-2 w-2 rounded-full shrink-0 ${dot[s.status]}`} title={dotLabel[s.status]} aria-label={dotLabel[s.status]} />}
        </div>
        <p className="text-sm muted truncate">{s.description || hostOf(link.url)}</p>
      </div>
    </>
  );

  return (
    <motion.div
      ref={sortable.setNodeRef} style={style} layout={!sortable.isDragging}
      initial={{ opacity: 0, y: 10 }} animate={{ opacity: sortable.isDragging ? 0.5 : 1, y: 0 }}
      transition={{ delay: Math.min(index * 0.03, 0.3), type: "spring", stiffness: 300, damping: 28 }}
      className={`glass group relative rounded-2xl p-4 transition-shadow hover:shadow-[0_14px_40px_color-mix(in_srgb,var(--accent)_22%,transparent)] ${edit ? "ring-1 ring-dashed ring-[var(--accent)]/40" : ""}`}
    >
      {edit ? (
        <div className="flex items-center gap-3">
          <button className="touch-none cursor-grab active:cursor-grabbing muted p-1 -ml-1" aria-label="拖动排序" {...sortable.attributes} {...sortable.listeners}>
            <GripVertical size={18} />
          </button>
          {body}
          <div className="flex gap-1 shrink-0">
            <button className="btn btn-sm btn-icon" onClick={() => onEdit(s)} aria-label={`编辑 ${s.name}`}><Pencil size={14} /></button>
            <button className="btn btn-sm btn-icon btn-danger" onClick={() => onDelete(s)} aria-label={`删除 ${s.name}`}><Trash2 size={14} /></button>
          </div>
        </div>
      ) : (
        <>
          <a href={link.url} target={target} rel="noopener noreferrer" className="flex items-center gap-3 after:absolute after:inset-0 after:rounded-2xl">
            {body}
          </a>
          <div className="relative z-10 mt-3 flex items-center justify-between gap-2">
            <span className="chip" title={link.only ? "仅配置了这一条线路" : "已按当前网络自动选择"}>
              <span className={`h-1.5 w-1.5 rounded-full ${link.line === "lan" ? "bg-emerald-500" : "bg-sky-500"}`} />
              {link.line === "lan" ? "内网" : "公网"}{link.only ? " · 唯一" : ""}
            </span>
            {link.other && (
              <a href={link.other.url} target={target} rel="noopener noreferrer"
                className="text-xs muted inline-flex items-center gap-1 hover:text-[var(--accent)] transition-colors"
                title={`改用${link.other.line === "lan" ? "内网" : "公网"}线路：${link.other.url}`}>
                <ArrowLeftRight size={12} />{link.other.line === "lan" ? "内网" : "公网"}
              </a>
            )}
          </div>
        </>
      )}
    </motion.div>
  );
}

function hostOf(u: string) {
  try {
    return new URL(u).host;
  } catch {
    return u;
  }
}
