import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Check, Copy, Palette, ScrollText, Settings2, ShieldCheck, Trash2, User as UserIcon, Users } from "lucide-react";
import QRCode from "qrcode";
import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { ErrorText, Field, Modal, Spinner, Switch, useToast } from "@/components/ui";
import { api, del, get, patch, post, put } from "@/lib/api";
import { errMsg, useMe } from "@/lib/hooks";
import { ACCENTS, useTheme } from "@/lib/theme";
import type { AuditEntry, SessionInfo, Settings, Team, User } from "@/lib/types";

const Card = ({ title, desc, children }: { title: string; desc?: string; children: ReactNode }) => (
  <section className="glass rounded-3xl p-6">
    <h2 className="text-base font-semibold">{title}</h2>
    {desc && <p className="text-sm muted mt-1">{desc}</p>}
    <div className="mt-5">{children}</div>
  </section>
);
const fmt = (ts: number) => new Date(ts * 1000).toLocaleString("zh-CN", { hour12: false });

export default function SettingsPage() {
  const { data: me } = useMe();
  const nav = useNavigate();
  const isAdmin = me?.user.role === "admin";
  const links: [string, string, ReactNode, boolean][] = [
    ["profile", "个人资料", <UserIcon size={16} />, true],
    ["security", "安全", <ShieldCheck size={16} />, true],
    ["appearance", "外观", <Palette size={16} />, true],
    ["users", "用户与团队", <Users size={16} />, !!isAdmin],
    ["general", "站点设置", <Settings2 size={16} />, !!isAdmin],
    ["audit", "审计日志", <ScrollText size={16} />, !!isAdmin],
  ];
  return (
    <div className="px-4 sm:px-6 py-6 mx-auto max-w-5xl">
      <button className="btn btn-sm mb-5" onClick={() => nav("/")} disabled={me?.mustEnroll2fa}><ArrowLeft size={14} />返回面板</button>
      {me?.mustEnroll2fa && <p className="glass rounded-2xl p-4 mb-5 text-sm">管理员要求所有账号启用两步验证。请先完成下方的设置。</p>}
      <div className="grid md:grid-cols-[13rem_1fr] gap-6">
        <nav className="glass rounded-2xl p-2 h-fit flex md:flex-col gap-1 overflow-x-auto" aria-label="设置导航">
          {links.filter((l) => l[3]).map(([to, label, icon]) => (
            <NavLink key={to} to={`/settings/${to}`} className={({ isActive }) =>
              `flex items-center gap-2.5 px-3 py-2 rounded-xl text-sm whitespace-nowrap ${isActive ? "bg-[var(--accent)] text-white" : "hover:bg-[color-mix(in_srgb,var(--accent)_12%,transparent)]"}`}>
              {icon}{label}
            </NavLink>
          ))}
        </nav>
        <div className="space-y-6 min-w-0">
          <Routes>
            <Route index element={<Navigate to="profile" replace />} />
            <Route path="profile" element={<Profile />} />
            <Route path="security" element={<Security />} />
            <Route path="appearance" element={<Appearance />} />
            {isAdmin && <Route path="users" element={<UsersAndTeams />} />}
            {isAdmin && <Route path="general" element={<General />} />}
            {isAdmin && <Route path="audit" element={<Audit />} />}
            <Route path="*" element={<Navigate to="profile" replace />} />
          </Routes>
        </div>
      </div>
    </div>
  );
}

/* ---------------- Profile ---------------- */
function Profile() {
  const { data: me } = useMe();
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState(me?.user.displayName ?? "");
  const save = useMutation({
    mutationFn: () => patch("/api/me", { displayName: name }),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: ["me"] }); toast("已保存"); },
  });
  return (
    <Card title="个人资料">
      <form className="space-y-4 max-w-sm" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
        <Field label="用户名"><input className="input" value={me?.user.username ?? ""} disabled /></Field>
        <Field label="显示名称"><input className="input" value={name} maxLength={40} onChange={(e) => setName(e.target.value)} required /></Field>
        <ErrorText>{save.isError ? errMsg(save.error) : null}</ErrorText>
        <button className="btn btn-primary" disabled={save.isPending}>保存</button>
      </form>
    </Card>
  );
}

/* ---------------- Security ---------------- */
function Security() {
  return (<><TwoFactor /><Password /><Sessions /></>);
}

function Password() {
  const toast = useToast();
  const [cur, setCur] = useState(""); const [nw, setNw] = useState(""); const [cf, setCf] = useState(""); const [local, setLocal] = useState("");
  const m = useMutation({
    mutationFn: () => put("/api/me/password", { current: cur, new: nw }),
    onSuccess: () => { setCur(""); setNw(""); setCf(""); toast("密码已更新，其他设备已登出"); },
  });
  const submit = (e: FormEvent) => { e.preventDefault(); setLocal(""); if (nw !== cf) return setLocal("两次输入的新密码不一致"); m.mutate(); };
  return (
    <Card title="修改密码" desc="修改后，其他设备上的登录会被注销。">
      <form onSubmit={submit} className="space-y-4 max-w-sm">
        <Field label="当前密码"><input className="input" type="password" autoComplete="current-password" value={cur} onChange={(e) => setCur(e.target.value)} required /></Field>
        <Field label="新密码" hint="至少 10 个字符"><input className="input" type="password" autoComplete="new-password" minLength={10} value={nw} onChange={(e) => setNw(e.target.value)} required /></Field>
        <Field label="确认新密码"><input className="input" type="password" autoComplete="new-password" value={cf} onChange={(e) => setCf(e.target.value)} required /></Field>
        <ErrorText>{local || (m.isError ? errMsg(m.error) : null)}</ErrorText>
        <button className="btn btn-primary" disabled={m.isPending}>{m.isPending && <Spinner />}更新密码</button>
      </form>
    </Card>
  );
}

function TwoFactor() {
  const { data: me } = useMe();
  const qc = useQueryClient();
  const toast = useToast();
  const [enroll, setEnroll] = useState<{ secret: string; uri: string; qr: string } | null>(null);
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  const [disableOpen, setDisableOpen] = useState(false);
  const [pw, setPw] = useState("");
  const refresh = () => qc.invalidateQueries({ queryKey: ["me"] });

  const start = useMutation({
    mutationFn: async () => { const r = await post<{ secret: string; uri: string }>("/api/me/totp/enroll"); return { ...r, qr: await QRCode.toDataURL(r.uri, { margin: 1, width: 220 }) }; },
    onSuccess: setEnroll,
  });
  const enable = useMutation({
    mutationFn: () => post<{ recoveryCodes: string[] }>("/api/me/totp/enable", { code }),
    onSuccess: async (r) => { setCodes(r.recoveryCodes); setEnroll(null); setCode(""); await refresh(); },
  });
  const disable = useMutation({
    mutationFn: () => post("/api/me/totp/disable", { password: pw }),
    onSuccess: async () => { setDisableOpen(false); setPw(""); await refresh(); toast("两步验证已关闭"); },
  });

  const on = me?.user.totpEnabled;
  return (
    <Card title="两步验证 (TOTP)" desc="使用 Google Authenticator、Microsoft Authenticator、1Password 等应用生成验证码。公网部署强烈建议开启。">
      <div className="flex items-center gap-3 mb-4">
        <span className={`chip ${on ? "!text-emerald-600 dark:!text-emerald-400" : ""}`}>{on ? <><Check size={12} />已启用</> : "未启用"}</span>
        {on ? <button className="btn btn-sm btn-danger" onClick={() => setDisableOpen(true)}>关闭</button>
          : !enroll && <button className="btn btn-sm btn-primary" onClick={() => start.mutate()} disabled={start.isPending}>{start.isPending && <Spinner />}开始设置</button>}
      </div>
      <ErrorText>{start.isError ? errMsg(start.error) : null}</ErrorText>
      {enroll && (
        <div className="grid sm:grid-cols-[auto_1fr] gap-6 items-start">
          <img src={enroll.qr} alt="两步验证二维码" className="rounded-2xl bg-white p-2" width={220} height={220} />
          <form className="space-y-4 max-w-xs" onSubmit={(e) => { e.preventDefault(); enable.mutate(); }}>
            <p className="text-sm">1. 用验证器应用扫描二维码，或手动输入密钥：</p>
            <code className="block text-xs break-all rounded-xl border border-[var(--line)] p-2 select-all">{enroll.secret}</code>
            <Field label="2. 输入应用显示的 6 位验证码"><input className="input tracking-widest text-center" inputMode="numeric" maxLength={6} autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} required /></Field>
            <ErrorText>{enable.isError ? errMsg(enable.error) : null}</ErrorText>
            <button className="btn btn-primary" disabled={enable.isPending}>{enable.isPending && <Spinner />}启用</button>
          </form>
        </div>
      )}
      <Modal open={!!codes} onOpenChange={(o) => !o && setCodes(null)} title="保存你的恢复码" description="每个恢复码只能使用一次，用于在丢失验证器时登录。它们只会显示这一次。">
        <div className="grid grid-cols-2 gap-2 font-mono text-sm">{codes?.map((c) => <code key={c} className="rounded-lg border border-[var(--line)] px-3 py-1.5 text-center select-all">{c}</code>)}</div>
        <div className="flex justify-end gap-2 mt-5">
          <button className="btn" onClick={() => { navigator.clipboard?.writeText(codes!.join("\n")); toast("已复制"); }}><Copy size={14} />复制</button>
          <button className="btn btn-primary" onClick={() => setCodes(null)}>我已保存</button>
        </div>
      </Modal>
      <Modal open={disableOpen} onOpenChange={setDisableOpen} title="关闭两步验证" description="请输入当前密码确认。">
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); disable.mutate(); }}>
          <Field label="密码"><input className="input" type="password" autoComplete="current-password" autoFocus value={pw} onChange={(e) => setPw(e.target.value)} required /></Field>
          <ErrorText>{disable.isError ? errMsg(disable.error) : null}</ErrorText>
          <div className="flex justify-end gap-2"><button type="button" className="btn" onClick={() => setDisableOpen(false)}>取消</button><button className="btn btn-primary" disabled={disable.isPending}>关闭</button></div>
        </form>
      </Modal>
    </Card>
  );
}

function Sessions() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["sessions"], queryFn: () => get<SessionInfo[]>("/api/me/sessions") });
  const revoke = useMutation({ mutationFn: (id: string) => del(`/api/me/sessions/${id}`), onSuccess: () => qc.invalidateQueries({ queryKey: ["sessions"] }) });
  return (
    <Card title="登录会话" desc="查看并注销你在其他设备上的登录。">
      <ul className="divide-y divide-[var(--line)]">
        {q.data?.map((s) => (
          <li key={s.id} className="py-3 flex items-center gap-3">
            <div className="min-w-0 flex-1">
              <div className="text-sm truncate" title={s.userAgent}>{s.userAgent || "未知设备"}</div>
              <div className="text-xs muted flex items-center gap-2 flex-wrap">{s.current && <span className="chip">当前设备</span>}{s.ip} · 最近活动 {fmt(s.lastSeen)}</div>
            </div>
            {!s.current && <button className="btn btn-sm btn-danger" onClick={() => revoke.mutate(s.id)}>注销</button>}
          </li>
        ))}
      </ul>
    </Card>
  );
}

/* ---------------- Appearance ---------------- */
function Appearance() {
  const { mode, setMode, accent, setAccent } = useTheme();
  const swatch: Record<string, string> = { brand: "#0a8cf8", cyan: "#06b6d4", emerald: "#10b981", amber: "#f59e0b", rose: "#f43f5e", violet: "#7c6cf0" };
  return (
    <Card title="外观" desc="仅保存在当前浏览器中。">
      <div className="space-y-6">
        <div>
          <span className="label">主题</span>
          <div className="flex gap-2">
            {([["system", "跟随系统"], ["light", "浅色"], ["dark", "深色"]] as const).map(([m, l]) => (
              <button key={m} className={`btn ${mode === m ? "btn-primary" : ""}`} onClick={() => setMode(m)} aria-pressed={mode === m}>{l}</button>
            ))}
          </div>
        </div>
        <div>
          <span className="label">主题色</span>
          <div className="flex gap-3">
            {ACCENTS.map((a) => (
              <button key={a} aria-label={a} aria-pressed={accent === a} onClick={() => setAccent(a)}
                className={`h-9 w-9 rounded-full grid place-items-center text-white cursor-pointer ring-offset-2 ring-offset-transparent transition ${accent === a ? "ring-2 ring-[var(--text)]" : ""}`}
                style={{ background: swatch[a] }}>{accent === a && <Check size={16} />}</button>
            ))}
          </div>
        </div>
      </div>
    </Card>
  );
}

/* ---------------- Users & Teams ---------------- */
function UsersAndTeams() {
  const qc = useQueryClient();
  const toast = useToast();
  const { data: me } = useMe();
  const users = useQuery({ queryKey: ["users"], queryFn: () => get<User[]>("/api/users") });
  const teams = useQuery({ queryKey: ["teams"], queryFn: () => get<Team[]>("/api/teams") });
  const [dlg, setDlg] = useState<{ open: boolean; user: User | null }>({ open: false, user: null });
  const [team, setTeam] = useState("");
  const refresh = () => { qc.invalidateQueries({ queryKey: ["users"] }); qc.invalidateQueries({ queryKey: ["teams"] }); };
  const addTeam = useMutation({ mutationFn: () => post("/api/teams", { name: team }), onSuccess: () => { setTeam(""); refresh(); }, onError: (e) => toast(errMsg(e), "err") });
  const delTeam = useMutation({ mutationFn: (id: number) => del(`/api/teams/${id}`), onSuccess: refresh, onError: (e) => toast(errMsg(e), "err") });
  const delUser = useMutation({ mutationFn: (id: number) => del(`/api/users/${id}`), onSuccess: refresh, onError: (e) => toast(errMsg(e), "err") });
  const tname = (id: number) => teams.data?.find((t) => t.id === id)?.name ?? id;

  return (
    <>
      <Card title="用户" desc="面板不开放注册，账号只能由管理员创建。">
        <ul className="divide-y divide-[var(--line)]">
          {users.data?.map((u) => (
            <li key={u.id} className="py-3 flex items-center gap-3 flex-wrap">
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium">{u.displayName} <span className="muted font-normal">@{u.username}</span></div>
                <div className="flex gap-1.5 mt-1 flex-wrap">
                  <span className="chip">{u.role === "admin" ? "管理员" : "用户"}</span>
                  {u.totpEnabled && <span className="chip">2FA</span>}
                  {u.disabled && <span className="chip !text-[var(--bad)]">已停用</span>}
                  {u.teams.map((t) => <span key={t} className="chip">{tname(t)}</span>)}
                </div>
              </div>
              <button className="btn btn-sm" onClick={() => setDlg({ open: true, user: u })}>编辑</button>
              {u.id !== me?.user.id && <button className="btn btn-sm btn-icon btn-danger" aria-label={`删除 ${u.username}`} onClick={() => confirm(`删除用户 ${u.username}？`) && delUser.mutate(u.id)}><Trash2 size={14} /></button>}
            </li>
          ))}
        </ul>
        <button className="btn btn-primary mt-4" onClick={() => setDlg({ open: true, user: null })}>添加用户</button>
      </Card>
      <Card title="团队" desc="用于限制服务的可见范围。未指定团队的服务对所有登录用户可见。">
        <div className="flex flex-wrap gap-2 mb-4">
          {teams.data?.map((t) => (
            <span key={t.id} className="chip !text-sm !px-3 !py-1">{t.name}
              <button aria-label={`删除团队 ${t.name}`} className="cursor-pointer muted hover:text-[var(--bad)]" onClick={() => delTeam.mutate(t.id)}>×</button>
            </span>
          ))}
          {teams.data?.length === 0 && <span className="text-sm muted">暂无团队</span>}
        </div>
        <form className="flex gap-2 max-w-sm" onSubmit={(e) => { e.preventDefault(); addTeam.mutate(); }}>
          <input className="input" placeholder="新团队名称，如 家人" value={team} onChange={(e) => setTeam(e.target.value)} maxLength={40} required />
          <button className="btn shrink-0">添加</button>
        </form>
      </Card>
      <UserDialog key={dlg.user?.id ?? "new"} open={dlg.open} user={dlg.user} teams={teams.data ?? []} onClose={() => setDlg({ open: false, user: null })} onSaved={refresh} />
    </>
  );
}

function UserDialog({ open, user, teams, onClose, onSaved }: { open: boolean; user: User | null; teams: Team[]; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ username: "", displayName: "", password: "", role: "user", disabled: false, teams: [] as number[] });
  useEffect(() => {
    if (open) setF({ username: user?.username ?? "", displayName: user?.displayName ?? "", password: "", role: user?.role ?? "user", disabled: user?.disabled ?? false, teams: user?.teams ?? [] });
  }, [open, user]);
  const m = useMutation({
    mutationFn: () => (user ? patch(`/api/users/${user.id}`, f) : post("/api/users", f)),
    onSuccess: () => { onSaved(); onClose(); },
  });
  return (
    <Modal open={open} onOpenChange={(o) => !o && onClose()} title={user ? `编辑 @${user.username}` : "添加用户"}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); m.mutate(); }}>
        {!user && <Field label="用户名" hint="3-32 位：字母、数字、_ . -"><input className="input" value={f.username} onChange={(e) => setF({ ...f, username: e.target.value })} required autoFocus /></Field>}
        <Field label="显示名称"><input className="input" value={f.displayName} maxLength={40} onChange={(e) => setF({ ...f, displayName: e.target.value })} /></Field>
        <Field label={user ? "重置密码（留空则不修改）" : "初始密码"} hint="至少 10 个字符；重置后该用户所有会话会被注销">
          <input className="input" type="password" autoComplete="new-password" minLength={f.password ? 10 : undefined} value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} required={!user} />
        </Field>
        <Field label="角色">
          <select className="input" value={f.role} onChange={(e) => setF({ ...f, role: e.target.value })}><option value="user">用户</option><option value="admin">管理员</option></select>
        </Field>
        {teams.length > 0 && (
          <div>
            <span className="label">所属团队</span>
            <div className="flex flex-wrap gap-2">
              {teams.map((t) => { const on = f.teams.includes(t.id); return (
                <button key={t.id} type="button" onClick={() => setF({ ...f, teams: on ? f.teams.filter((x) => x !== t.id) : [...f.teams, t.id] })}
                  className={`chip cursor-pointer !text-xs !px-3 !py-1 ${on ? "!bg-[var(--accent)] !text-white !border-transparent" : ""}`}>{t.name}</button>); })}
            </div>
          </div>
        )}
        {user && <Switch label="停用账号" checked={f.disabled} onChange={(v) => setF({ ...f, disabled: v })} />}
        <ErrorText>{m.isError ? errMsg(m.error) : null}</ErrorText>
        <div className="flex justify-end gap-2"><button type="button" className="btn" onClick={onClose}>取消</button><button className="btn btn-primary" disabled={m.isPending}>{m.isPending && <Spinner />}保存</button></div>
      </form>
    </Modal>
  );
}

/* ---------------- General ---------------- */
function General() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["settings"], queryFn: () => get<Settings>("/api/settings") });
  const [f, setF] = useState({ siteName: "", wallpaper: "", require2fa: false });
  useEffect(() => { if (q.data) setF({ siteName: q.data.siteName, wallpaper: q.data.wallpaper, require2fa: q.data.require2fa }); }, [q.data]);
  const save = useMutation({
    mutationFn: () => put("/api/settings", f),
    onSuccess: async () => { await Promise.all([qc.invalidateQueries({ queryKey: ["settings"] }), qc.invalidateQueries({ queryKey: ["bootstrap"] })]); toast("已保存"); },
  });
  const up = useMutation({
    mutationFn: (file: File) => { const fd = new FormData(); fd.append("file", file); fd.append("kind", "wallpaper"); return api<{ url: string }>("POST", "/api/uploads/icon", fd); },
    onSuccess: (r) => setF((x) => ({ ...x, wallpaper: r.url })),
  });
  return (
    <>
      <Card title="站点设置">
        <form className="space-y-4 max-w-lg" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
          <Field label="站点名称"><input className="input" value={f.siteName} maxLength={40} onChange={(e) => setF({ ...f, siteName: e.target.value })} required /></Field>
          <Field label="壁纸（图片 URL 或上传，可留空）" hint="上传的壁纸会自动压缩为最长边 1920px 的 JPEG。">
            <div className="flex gap-2">
              <input className="input" value={f.wallpaper} placeholder="https://…" onChange={(e) => setF({ ...f, wallpaper: e.target.value.trim() })} />
              <label className="btn shrink-0 cursor-pointer">上传<input type="file" hidden accept="image/png,image/jpeg,image/webp" onChange={(e) => e.target.files?.[0] && up.mutate(e.target.files[0])} /></label>
            </div>
          </Field>
          <div className="rounded-2xl border border-[var(--line)] px-4 py-2"><Switch label="强制所有用户启用两步验证" checked={f.require2fa} onChange={(v) => setF({ ...f, require2fa: v })} /></div>
          <ErrorText>{save.isError ? errMsg(save.error) : up.isError ? errMsg(up.error) : null}</ErrorText>
          <button className="btn btn-primary" disabled={save.isPending}>{save.isPending && <Spinner />}保存</button>
        </form>
      </Card>
      <Card title="内网判定网段" desc="来源 IP 落在这些网段内，将被视为内网访问。通过环境变量 YDP_LAN_CIDRS 修改；经反向代理访问时还需配置 YDP_TRUSTED_PROXIES。">
        <div className="flex flex-wrap gap-2">{q.data?.lanCidrs.map((c) => <code key={c} className="chip !text-xs">{c}</code>)}</div>
      </Card>
    </>
  );
}

/* ---------------- Audit ---------------- */
function Audit() {
  const [rows, setRows] = useState<AuditEntry[]>([]);
  const [done, setDone] = useState(false);
  const load = useMutation({
    mutationFn: () => get<AuditEntry[]>(`/api/audit?limit=50&offset=${rows.length}`),
    onSuccess: (r) => { setRows((x) => [...x, ...r]); setDone(r.length < 50); },
  });
  useEffect(() => { load.mutate(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Card title="审计日志" desc="登录、账号与配置变更记录。">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead><tr className="text-left muted text-xs"><th className="py-2 pr-4 font-medium">时间</th><th className="pr-4 font-medium">用户</th><th className="pr-4 font-medium">事件</th><th className="pr-4 font-medium">详情</th><th className="font-medium">IP</th></tr></thead>
          <tbody className="divide-y divide-[var(--line)]">
            {rows.map((r) => (
              <tr key={r.id}><td className="py-2 pr-4 whitespace-nowrap">{fmt(r.ts)}</td><td className="pr-4">{r.username || "—"}</td>
                <td className={`pr-4 font-mono text-xs ${r.action.includes("failed") ? "text-[var(--bad)]" : ""}`}>{r.action}</td><td className="pr-4 muted">{r.detail}</td><td className="font-mono text-xs">{r.ip}</td></tr>
            ))}
          </tbody>
        </table>
      </div>
      {!done && <button className="btn mt-4" onClick={() => load.mutate()} disabled={load.isPending}>{load.isPending && <Spinner />}加载更多</button>}
    </Card>
  );
}
