import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Globe, Upload } from "lucide-react";
import { useRef, useState, type FormEvent } from "react";
import { get, post, put } from "@/lib/api";
import { COLOR_PRESETS, contrastFg, faviconUrl } from "@/lib/color";
import { errMsg } from "@/lib/hooks";
import { ICONS } from "@/lib/icons";
import type { Group, Service, Team } from "@/lib/types";
import { CropDialog } from "./CropDialog";
import { ErrorText, Field, Modal, ServiceIcon, Spinner, Switch } from "./ui";

export function ServiceDialog({ open, onClose, service, groups, defaultGroup }: {
  open: boolean; onClose: () => void; service: Service | null; groups: Group[]; defaultGroup?: number;
}) {
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={service ? "编辑服务" : "添加服务"} description="可同时配置内网和公网地址，面板会按访问者所在网络自动选择。" wide>
      {open && <Form key={service?.id ?? "new"} service={service} groups={groups} defaultGroup={defaultGroup} onClose={onClose} />}
    </Modal>
  );
}

function Form({ service, groups, defaultGroup, onClose }: { service: Service | null; groups: Group[]; defaultGroup?: number; onClose: () => void }) {
  const qc = useQueryClient();
  const teams = useQuery({ queryKey: ["teams"], queryFn: () => get<Team[]>("/api/teams") });
  const file = useRef<HTMLInputElement>(null);
  const [f, setF] = useState({
    name: service?.name ?? "", description: service?.description ?? "", icon: service?.icon ?? "", color: service?.color ?? "", iconFit: service?.iconFit ?? "contain",
    groupId: service?.groupId ?? defaultGroup ?? groups[0]?.id ?? 0,
    lanUrl: service?.lanUrl ?? "", wanUrl: service?.wanUrl ?? "",
    newTab: service?.newTab ?? true, healthCheck: service?.healthCheck ?? true, teams: service?.teams ?? [],
  });
  const [cropFile, setCropFile] = useState<File | null>(null);
  const set = <K extends keyof typeof f>(k: K, v: (typeof f)[K]) => setF((x) => ({ ...x, [k]: v }));

  const save = useMutation({
    mutationFn: () => (service ? put(`/api/services/${service.id}`, f) : post("/api/services", f)),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: ["dashboard"] }); onClose(); },
  });
  // A freshly uploaded picture is normally a complete icon, so default to filling the tile.
  const onUploaded = (url: string) => setF((x) => ({ ...x, icon: url, iconFit: "cover" }));
  const isImage = f.icon === "" || /^(https?:\/\/|\/icons\/)/.test(f.icon);

  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate(); };
  const err = save.isError ? errMsg(save.error) : null;

  return (
    <>
    <form onSubmit={submit} className="space-y-4">
      <div className="grid sm:grid-cols-2 gap-4">
        <Field label="名称"><input className="input" value={f.name} onChange={(e) => set("name", e.target.value)} required maxLength={60} autoFocus /></Field>
        <Field label="分组">
          <select className="input" value={f.groupId} onChange={(e) => set("groupId", Number(e.target.value))}>
            {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
          </select>
        </Field>
      </div>
      <Field label="描述（可选）"><input className="input" value={f.description} onChange={(e) => set("description", e.target.value)} maxLength={200} /></Field>
      <div className="grid sm:grid-cols-2 gap-4">
        <Field label="内网地址" hint="如 http://192.168.1.10:8096"><input className="input" inputMode="url" placeholder="http://" value={f.lanUrl} onChange={(e) => set("lanUrl", e.target.value)} /></Field>
        <Field label="公网地址" hint="如 https://jellyfin.example.com"><input className="input" inputMode="url" placeholder="https://" value={f.wanUrl} onChange={(e) => set("wanUrl", e.target.value)} /></Field>
      </div>
      <p className="text-xs muted -mt-2">只填写一个地址时，无论在内网还是公网都会使用该地址。</p>

      <div className="space-y-4">
        <div className="flex items-center gap-4">
          <ServiceIcon icon={f.icon} name={f.name || "?"} size={26} color={f.color} fit={f.iconFit} favicon={service ? faviconUrl({ ...service, lanUrl: f.lanUrl, wanUrl: f.wanUrl }) : undefined} />
          <p className="text-xs muted leading-relaxed">
            {f.icon ? "已选择自定义图标。" : service ? "未选择图标：自动使用网站的 favicon，抓取失败时显示首字母。" : "未选择图标：保存后将自动抓取网站的 favicon。"}
          </p>
        </div>

        <div>
          <span className="label">图标</span>
          <div className="flex items-center gap-2 mb-3">
            <input className="input" placeholder="图标名、emoji 或图片 URL（留空＝网站图标）" value={f.icon} onChange={(e) => set("icon", e.target.value.trim())} />
            <input ref={file} type="file" accept="image/png,image/jpeg,image/webp,image/gif" hidden onChange={(e) => { const picked = e.target.files?.[0]; e.target.value = ""; if (picked) setCropFile(picked); }} />
            <button type="button" className="btn shrink-0" onClick={() => file.current?.click()}>
              <Upload size={16} />上传
            </button>
          </div>
          <p className="text-xs muted -mt-1 mb-2">上传后可裁切为正方形，并自动缩小到最长边 256px、转为 PNG（保留透明），仅保存在本机数据库中。</p>
          <div className="grid grid-cols-8 sm:grid-cols-10 gap-1.5 max-h-28 overflow-y-auto p-1">
            <button type="button" title="自动（网站图标）" onClick={() => set("icon", "")}
              className={`h-9 rounded-lg grid place-items-center cursor-pointer transition-colors ${f.icon === "" ? "bg-[var(--accent)] text-white" : "hover:bg-[color-mix(in_srgb,var(--accent)_14%,transparent)]"}`}>
              <Globe size={16} />
            </button>
            {Object.entries(ICONS).map(([name, Ico]) => (
              <button key={name} type="button" title={name} onClick={() => set("icon", name)}
                className={`h-9 rounded-lg grid place-items-center cursor-pointer transition-colors ${f.icon === name ? "bg-[var(--accent)] text-white" : "hover:bg-[color-mix(in_srgb,var(--accent)_14%,transparent)]"}`}>
                <Ico size={16} />
              </button>
            ))}
          </div>
        </div>

        {isImage && (
          <div>
            <span className="label">图片显示方式</span>
            <div className="inline-flex rounded-xl border border-[var(--line)] p-0.5" role="radiogroup" aria-label="图片显示方式">
              {([["contain", "居中"], ["cover", "平铺"]] as const).map(([v, label]) => (
                <button key={v} type="button" role="radio" aria-checked={f.iconFit === v} onClick={() => set("iconFit", v)}
                  className={`px-4 h-8 rounded-[10px] text-sm cursor-pointer transition-colors ${f.iconFit === v ? "bg-[var(--accent)] text-white" : "hover:bg-[color-mix(in_srgb,var(--accent)_12%,transparent)]"}`}>{label}</button>
              ))}
            </div>
            <p className="text-xs muted mt-1.5">
              {f.iconFit === "contain" ? "居中：图片缩小后放在底色上，适合小图标、透明 PNG、网站图标。" : "平铺：图片铺满整个图标区域并裁成圆角，适合完整的图标图片。"}
            </p>
          </div>
        )}

        <div>
          <span className="label">底色{isImage && f.iconFit === "cover" ? "（仅图片有透明部分时可见）" : ""}</span>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" title="默认（主题渐变）" aria-label="默认底色" aria-pressed={f.color === ""} onClick={() => set("color", "")}
              className={`h-8 w-8 rounded-full cursor-pointer grid place-items-center text-white ring-offset-2 ring-offset-transparent ${f.color === "" ? "ring-2 ring-[var(--text)]" : ""}`}
              style={{ background: "linear-gradient(135deg,var(--accent),var(--accent-2))" }}>{f.color === "" && <Check size={14} />}</button>
            {COLOR_PRESETS.map((c) => (
              <button key={c} type="button" title={c} aria-label={`底色 ${c}`} aria-pressed={f.color.toLowerCase() === c} onClick={() => set("color", c)}
                className={`h-8 w-8 rounded-full cursor-pointer grid place-items-center border border-[var(--line)] ring-offset-2 ring-offset-transparent ${f.color.toLowerCase() === c ? "ring-2 ring-[var(--text)]" : ""}`}
                style={{ background: c, color: contrastFg(c) }}>{f.color.toLowerCase() === c && <Check size={14} />}</button>
            ))}
            <label className="h-8 px-3 rounded-full border border-[var(--line)] text-xs inline-flex items-center gap-2 cursor-pointer" title="自定义颜色">
              <input type="color" className="h-5 w-5 rounded cursor-pointer bg-transparent border-0 p-0" value={/^#[0-9a-f]{6}$/i.test(f.color) ? f.color : "#0a8cf8"} onChange={(e) => set("color", e.target.value)} />
              自定义
            </label>
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-[var(--line)] px-4 py-2 divide-y divide-[var(--line)]">
        <Switch label="在新标签页打开" checked={f.newTab} onChange={(v) => set("newTab", v)} />
        <Switch label="检测在线状态" checked={f.healthCheck} onChange={(v) => set("healthCheck", v)} />
      </div>

      {(teams.data?.length ?? 0) > 0 && (
        <div>
          <span className="label">可见范围</span>
          <div className="flex flex-wrap gap-2">
            {teams.data!.map((t) => {
              const on = f.teams.includes(t.id);
              return (
                <button key={t.id} type="button" onClick={() => set("teams", on ? f.teams.filter((x) => x !== t.id) : [...f.teams, t.id])}
                  className={`chip cursor-pointer !text-xs !px-3 !py-1 ${on ? "!bg-[var(--accent)] !text-white !border-transparent" : ""}`}>{t.name}</button>
              );
            })}
          </div>
          <p className="text-xs muted mt-1.5">{f.teams.length === 0 ? "未选择团队：所有已登录用户可见。" : "仅所选团队成员与管理员可见。"}</p>
        </div>
      )}

      <ErrorText>{err}</ErrorText>
      <div className="flex justify-end gap-2 pt-1">
        <button type="button" className="btn" onClick={onClose}>取消</button>
        <button className="btn btn-primary" disabled={save.isPending}>{save.isPending && <Spinner />}保存</button>
      </div>
    </form>
    <CropDialog file={cropFile} onClose={() => setCropFile(null)} onUploaded={onUploaded} />
    </>
  );
}
