import {
  closestCorners, DndContext, KeyboardSensor, PointerSensor, useDroppable, useSensor, useSensors,
  type DragEndEvent, type DragOverEvent,
} from "@dnd-kit/core";
import { arrayMove, rectSortingStrategy, SortableContext, sortableKeyboardCoordinates } from "@dnd-kit/sortable";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { motion } from "framer-motion";
import { ArrowDown, ArrowUp, Pencil, Plus, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { CommandPalette } from "@/components/CommandPalette";
import { ServiceCard } from "@/components/ServiceCard";
import { ServiceDialog } from "@/components/ServiceDialog";
import { TopBar } from "@/components/TopBar";
import { ErrorText, Field, Modal, Spinner, useToast } from "@/components/ui";
import { del, patch, post, put } from "@/lib/api";
import { errMsg, useDashboard, useMe, useNetwork } from "@/lib/hooks";
import { loadNetMode, saveNetMode, type NetMode } from "@/lib/links";
import type { Group, Service, Zone } from "@/lib/types";

type Layout = Record<string, string[]>; // "g-<id>" -> ["s-<id>", ...]

function greeting(d: Date) {
  const h = d.getHours();
  return h < 5 ? "夜深了" : h < 11 ? "早上好" : h < 14 ? "中午好" : h < 18 ? "下午好" : "晚上好";
}

function Clock() {
  const [now, setNow] = useState(new Date());
  useEffect(() => { const t = setInterval(() => setNow(new Date()), 15_000); return () => clearInterval(t); }, []);
  return (
    <div>
      <div className="text-4xl sm:text-5xl font-semibold tracking-tight tabular-nums">
        {now.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false })}
      </div>
      <div className="muted mt-1">{now.toLocaleDateString("zh-CN", { month: "long", day: "numeric", weekday: "long" })}</div>
    </div>
  );
}

function NameDialog({ open, title, initial, onClose, onSubmit }: { open: boolean; title: string; initial: string; onClose: () => void; onSubmit: (n: string) => Promise<unknown> }) {
  const [name, setName] = useState(initial);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => { if (open) { setName(initial); setErr(""); } }, [open, initial]);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try { await onSubmit(name.trim()); onClose(); } catch (x) { setErr(errMsg(x)); } finally { setBusy(false); }
  };
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={title}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="名称"><input className="input" autoFocus value={name} maxLength={40} onChange={(e) => setName(e.target.value)} required /></Field>
        <ErrorText>{err}</ErrorText>
        <div className="flex justify-end gap-2"><button type="button" className="btn" onClick={onClose}>取消</button><button className="btn btn-primary" disabled={busy}>{busy && <Spinner />}保存</button></div>
      </form>
    </Modal>
  );
}

function GroupSection(props: {
  group: Group; ids: string[]; byId: Map<string, Service>; zone: Zone; edit: boolean; first: boolean; last: boolean;
  onAdd: () => void; onEditSvc: (s: Service) => void; onDelSvc: (s: Service) => void;
  onRename: () => void; onDelete: () => void; onMove: (d: -1 | 1) => void;
}) {
  const { group, ids, byId, zone, edit } = props;
  const { setNodeRef, isOver } = useDroppable({ id: `g-${group.id}` });
  return (
    <section aria-label={group.name}>
      <div className="flex items-center gap-2 mb-3 px-1">
        <h2 className="text-sm font-semibold uppercase tracking-wider muted">{group.name}</h2>
        <span className="text-xs muted">{ids.length}</span>
        {edit && (
          <div className="flex gap-1 ml-auto">
            <button className="btn btn-sm btn-icon" disabled={props.first} onClick={() => props.onMove(-1)} aria-label="上移分组"><ArrowUp size={14} /></button>
            <button className="btn btn-sm btn-icon" disabled={props.last} onClick={() => props.onMove(1)} aria-label="下移分组"><ArrowDown size={14} /></button>
            <button className="btn btn-sm btn-icon" onClick={props.onRename} aria-label="重命名分组"><Pencil size={14} /></button>
            <button className="btn btn-sm btn-icon btn-danger" onClick={props.onDelete} aria-label="删除分组"><Trash2 size={14} /></button>
            <button className="btn btn-sm" onClick={props.onAdd}><Plus size={14} />服务</button>
          </div>
        )}
      </div>
      <SortableContext items={ids} strategy={rectSortingStrategy}>
        <div ref={setNodeRef} className={`grid gap-3 sm:grid-cols-2 lg:grid-cols-3 rounded-2xl transition-colors ${edit && ids.length === 0 ? "min-h-24 border border-dashed border-[var(--line)] p-3" : ""} ${isOver && edit ? "bg-[color-mix(in_srgb,var(--accent)_8%,transparent)]" : ""}`}>
          {ids.map((id, i) => {
            const s = byId.get(id);
            return s ? <ServiceCard key={id} service={s} zone={zone} edit={edit} index={i} onEdit={props.onEditSvc} onDelete={props.onDelSvc} /> : null;
          })}
          {edit && ids.length === 0 && <p className="text-sm muted col-span-full self-center text-center">拖动服务到这里，或点击右上角“+ 服务”</p>}
        </div>
      </SortableContext>
    </section>
  );
}

export default function HomePage() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: me } = useMe();
  const isAdmin = me?.user.role === "admin";
  const dash = useDashboard();
  const net = useNetwork();
  const [mode, setModeState] = useState<NetMode>(loadNetMode);
  const setMode = (m: NetMode) => { setModeState(m); saveNetMode(m); };
  const zone: Zone = mode === "auto" ? net.data?.zone ?? "wan" : mode;
  const [edit, setEdit] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [svcDlg, setSvcDlg] = useState<{ open: boolean; service: Service | null; group?: number }>({ open: false, service: null });
  const [nameDlg, setNameDlg] = useState<{ open: boolean; group: Group | null }>({ open: false, group: null });

  const groups = dash.data?.groups ?? [];
  const services = dash.data?.services ?? [];
  const byId = useMemo(() => new Map(services.map((s) => [`s-${s.id}`, s])), [services]);

  const [layout, setLayout] = useState<Layout>({});
  const layoutRef = useRef<Layout>({});
  const dragging = useRef(false);
  useEffect(() => {
    if (!dash.data || dragging.current) return;
    const l: Layout = {};
    groups.forEach((g) => (l[`g-${g.id}`] = services.filter((s) => s.groupId === g.id).map((s) => `s-${s.id}`)));
    layoutRef.current = l;
    setLayout(l);
  }, [dash.data]); // eslint-disable-line react-hooks/exhaustive-deps

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const containerOf = (l: Layout, id: string) => (id in l ? id : Object.keys(l).find((k) => l[k].includes(id)));
  const commit = (l: Layout) => { layoutRef.current = l; setLayout(l); };

  const onDragOver = ({ active, over }: DragOverEvent) => {
    if (!over) return;
    dragging.current = true;
    const l = layoutRef.current;
    const a = containerOf(l, String(active.id));
    const o = containerOf(l, String(over.id));
    if (!a || !o || a === o) return;
    const overItems = l[o];
    const overIdx = overItems.indexOf(String(over.id));
    const below = overIdx >= 0 && active.rect.current.translated && active.rect.current.translated.top > over.rect.top + over.rect.height / 2;
    const idx = overIdx >= 0 ? overIdx + (below ? 1 : 0) : overItems.length;
    commit({ ...l, [a]: l[a].filter((x) => x !== String(active.id)), [o]: [...overItems.slice(0, idx), String(active.id), ...overItems.slice(idx)] });
  };

  const saveLayout = useMutation({
    mutationFn: (l: Layout) => {
      const body: Record<string, number[]> = {};
      for (const [k, v] of Object.entries(l)) body[k.slice(2)] = v.map((x) => Number(x.slice(2)));
      return put("/api/layout", { layout: body });
    },
    onSettled: async () => { dragging.current = false; await qc.invalidateQueries({ queryKey: ["dashboard"] }); },
    onError: (e) => toast(errMsg(e), "err"),
  });

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    let l = layoutRef.current;
    if (over) {
      const a = containerOf(l, String(active.id));
      const o = containerOf(l, String(over.id));
      if (a && a === o) {
        const from = l[a].indexOf(String(active.id));
        const to = l[a].indexOf(String(over.id));
        if (to >= 0 && from !== to) { l = { ...l, [a]: arrayMove(l[a], from, to) }; commit(l); }
      }
    }
    saveLayout.mutate(l);
  };

  const invalidate = () => qc.invalidateQueries({ queryKey: ["dashboard"] });
  const delSvc = useMutation({ mutationFn: (s: Service) => del(`/api/services/${s.id}`), onSuccess: invalidate, onError: (e) => toast(errMsg(e), "err") });
  const delGroup = useMutation({ mutationFn: (g: Group) => del(`/api/groups/${g.id}`), onSuccess: invalidate, onError: (e) => toast(errMsg(e), "err") });
  const reorder = useMutation({ mutationFn: (ids: number[]) => put("/api/groups/order", { ids }), onSuccess: invalidate });

  const moveGroup = (i: number, d: -1 | 1) => {
    const ids = groups.map((g) => g.id);
    [ids[i], ids[i + d]] = [ids[i + d], ids[i]];
    reorder.mutate(ids);
  };

  const loading = dash.isLoading || (mode === "auto" && net.isLoading);
  const name = me?.user.displayName ?? "";

  return (
    <div className="px-4 pb-16 pt-3 sm:px-6">
      <TopBar net={net.data} zone={zone} mode={mode} setMode={setMode} edit={edit} setEdit={setEdit} onSearch={() => setSearchOpen(true)} />
      <CommandPalette open={searchOpen} setOpen={setSearchOpen} services={services} zone={zone} />

      <main className="mx-auto max-w-6xl">
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} className="py-10 sm:py-14 flex flex-wrap items-end justify-between gap-6">
          <div>
            <p className="muted text-lg">{greeting(new Date())}，{name}</p>
            <div className="mt-2"><Clock /></div>
          </div>
        </motion.div>

        {dash.isError && <ErrorText>{errMsg(dash.error)}</ErrorText>}

        {loading ? (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => <div key={i} className="glass h-24 rounded-2xl animate-pulse" />)}
          </div>
        ) : (
          <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={() => (dragging.current = true)} onDragOver={onDragOver} onDragEnd={onDragEnd} onDragCancel={() => { dragging.current = false; invalidate(); }}>
            <div className="space-y-10">
              {groups.map((g, i) => (
                <GroupSection key={g.id} group={g} ids={layout[`g-${g.id}`] ?? []} byId={byId} zone={zone} edit={edit}
                  first={i === 0} last={i === groups.length - 1}
                  onAdd={() => setSvcDlg({ open: true, service: null, group: g.id })}
                  onEditSvc={(s) => setSvcDlg({ open: true, service: s })}
                  onDelSvc={(s) => confirm(`删除服务“${s.name}”？`) && delSvc.mutate(s)}
                  onRename={() => setNameDlg({ open: true, group: g })}
                  onDelete={() => { const n = (layout[`g-${g.id}`] ?? []).length; confirm(`删除分组“${g.name}”${n ? `及其中的 ${n} 个服务` : ""}？此操作不可撤销。`) && delGroup.mutate(g); }}
                  onMove={(d) => moveGroup(i, d)} />
              ))}
            </div>
          </DndContext>
        )}

        {!loading && groups.length === 0 && (
          <div className="glass rounded-3xl p-10 text-center">
            <p className="text-lg font-medium">{isAdmin ? "还没有任何服务" : "暂时没有可访问的服务"}</p>
            <p className="muted mt-1">{isAdmin ? "点击右上角“编辑”，创建分组并添加你的第一个服务。" : "请联系管理员为你分配服务。"}</p>
          </div>
        )}

        {edit && (
          <button className="btn mt-8" onClick={() => setNameDlg({ open: true, group: null })}><Plus size={16} />添加分组</button>
        )}
      </main>

      <ServiceDialog open={svcDlg.open} service={svcDlg.service} groups={groups} defaultGroup={svcDlg.group} onClose={() => setSvcDlg({ open: false, service: null })} />
      <NameDialog open={nameDlg.open} title={nameDlg.group ? "重命名分组" : "添加分组"} initial={nameDlg.group?.name ?? ""}
        onClose={() => setNameDlg({ open: false, group: null })}
        onSubmit={async (n) => { await (nameDlg.group ? patch(`/api/groups/${nameDlg.group.id}`, { name: n }) : post("/api/groups", { name: n })); await invalidate(); }} />
    </div>
  );
}
