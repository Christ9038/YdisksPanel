import { useMutation, useQueryClient } from "@tanstack/react-query";
import { motion } from "framer-motion";
import { KeyRound, ShieldCheck } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { post } from "@/lib/api";
import { errMsg, useBootstrap, useMe } from "@/lib/hooks";
import { ErrorText, Field, Logo, Spinner } from "@/components/ui";

function Shell({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) {
  const { data: b } = useBootstrap();
  return (
    <main className="min-h-screen grid place-items-center px-4 py-10">
      <motion.div initial={{ opacity: 0, y: 18, scale: 0.98 }} animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ type: "spring", stiffness: 260, damping: 26 }} className="glass-strong w-full max-w-sm rounded-3xl p-8">
        <div className="flex items-center gap-3 mb-6">
          <Logo size={44} />
          <div>
            <div className="text-sm muted">{b?.siteName ?? "YdisksPanel"}</div>
            <h1 className="text-xl font-semibold leading-tight">{title}</h1>
          </div>
        </div>
        <p className="text-sm muted mb-5">{subtitle}</p>
        {children}
      </motion.div>
    </main>
  );
}

export function LoginPage() {
  const qc = useQueryClient();
  const nav = useNavigate();
  const { data: boot } = useBootstrap();
  const { data: me } = useMe();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(false);
  const [challenge, setChallenge] = useState("");
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState(false);

  const done = async () => {
    await qc.invalidateQueries({ queryKey: ["me"] });
    nav("/", { replace: true });
  };
  const login = useMutation({
    mutationFn: () => post<{ totpRequired?: boolean; challenge?: string }>("/api/auth/login", { username, password, remember }),
    onSuccess: (r) => (r.totpRequired ? setChallenge(r.challenge!) : done()),
  });
  const verify = useMutation({
    mutationFn: () => post("/api/auth/totp", { challenge, code }),
    onSuccess: done,
  });

  if (boot?.needsSetup) return <Navigate to="/setup" replace />;
  if (me) return <Navigate to="/" replace />;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    (challenge ? verify : login).mutate();
  };
  const m = challenge ? verify : login;

  return (
    <Shell title={challenge ? "两步验证" : "欢迎回来"} subtitle={challenge ? (recovery ? "输入一个未使用的恢复码。" : "输入验证器应用中的 6 位验证码。") : "登录以访问你的家庭服务。"}>
      <form onSubmit={submit} className="space-y-4">
        {!challenge ? (
          <>
            <Field label="用户名"><input className="input" autoComplete="username" autoFocus value={username} onChange={(e) => setUsername(e.target.value)} required /></Field>
            <Field label="密码"><input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required /></Field>
            <label className="flex items-center gap-2 text-sm cursor-pointer select-none">
              <input type="checkbox" className="accent-[var(--accent)] h-4 w-4" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
              30 天内保持登录
            </label>
          </>
        ) : (
          <>
            <Field label={recovery ? "恢复码" : "验证码"}>
              <input className="input tracking-widest text-center text-lg" autoFocus autoComplete="one-time-code"
                inputMode={recovery ? "text" : "numeric"} value={code} maxLength={recovery ? 11 : 6}
                onChange={(e) => setCode(e.target.value)} required />
            </Field>
            <button type="button" className="text-sm text-[var(--accent)] cursor-pointer" onClick={() => { setRecovery(!recovery); setCode(""); }}>
              {recovery ? "使用验证器验证码" : "改用恢复码"}
            </button>
          </>
        )}
        <ErrorText>{m.isError ? errMsg(m.error) : null}</ErrorText>
        <button className="btn btn-primary w-full" disabled={m.isPending}>
          {m.isPending ? <Spinner /> : challenge ? <ShieldCheck size={16} /> : <KeyRound size={16} />}
          {challenge ? "验证" : "登录"}
        </button>
      </form>
    </Shell>
  );
}

export function SetupPage() {
  const qc = useQueryClient();
  const nav = useNavigate();
  const { data: boot, isLoading } = useBootstrap();
  const [token, setToken] = useState("");
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [localErr, setLocalErr] = useState("");
  const setup = useMutation({
    mutationFn: () => post("/api/setup", { token, username, password }),
    onSuccess: async () => {
      await qc.invalidateQueries();
      nav("/", { replace: true });
    },
  });
  if (isLoading) return null;
  if (!boot?.needsSetup) return <Navigate to="/login" replace />;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setLocalErr("");
    if (password !== confirm) return setLocalErr("两次输入的密码不一致");
    setup.mutate();
  };
  return (
    <Shell title="初始化面板" subtitle="创建管理员账号。安装令牌已打印在服务端启动日志中。">
      <form onSubmit={submit} className="space-y-4">
        <Field label="安装令牌"><input className="input font-mono" autoFocus value={token} onChange={(e) => setToken(e.target.value)} required /></Field>
        <Field label="管理员用户名"><input className="input" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} required /></Field>
        <Field label="密码" hint="至少 10 个字符"><input className="input" type="password" autoComplete="new-password" minLength={10} value={password} onChange={(e) => setPassword(e.target.value)} required /></Field>
        <Field label="确认密码"><input className="input" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required /></Field>
        <ErrorText>{localErr || (setup.isError ? errMsg(setup.error) : null)}</ErrorText>
        <button className="btn btn-primary w-full" disabled={setup.isPending}>{setup.isPending && <Spinner />}创建并登录</button>
      </form>
    </Shell>
  );
}
