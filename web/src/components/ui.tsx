import * as Dialog from "@radix-ui/react-dialog";
import { AnimatePresence, motion } from "framer-motion";
import { Check, X } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { contrastFg } from "@/lib/color";
import { ICONS } from "@/lib/icons";

/* ---------- Logo (same mark as www.ydisks.com) ---------- */
export function Logo({ size = 32 }: { size?: number }) {
  return <img src="/logo.png" alt="" width={size} height={size} className="shrink-0 select-none" style={{ width: size, height: size }} draggable={false} />;
}

/* ---------- Background ---------- */
export function Background({ wallpaper }: { wallpaper?: string }) {
  return (
    <div className="bg-scene" aria-hidden>
      {wallpaper ? (
        <>
          <img src={wallpaper} alt="" referrerPolicy="no-referrer" />
          <div className="veil" />
        </>
      ) : null}
    </div>
  );
}

/* ---------- Modal ---------- */
export function Modal({
  open, onOpenChange, title, description, children, wide,
}: {
  open: boolean; onOpenChange: (o: boolean) => void; title: string; description?: string; children: ReactNode; wide?: boolean;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <AnimatePresence>
        {open && (
          <Dialog.Portal forceMount>
            <Dialog.Overlay asChild>
              <motion.div className="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} />
            </Dialog.Overlay>
            <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-0 sm:p-4 pointer-events-none">
              <Dialog.Content asChild>
                <motion.div
                  className={`glass-strong pointer-events-auto w-full ${wide ? "sm:max-w-2xl" : "sm:max-w-md"} max-h-[92vh] overflow-y-auto rounded-t-3xl sm:rounded-3xl p-6`}
                  initial={{ opacity: 0, y: 24, scale: 0.98 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: 12 }}
                  transition={{ type: "spring", stiffness: 380, damping: 32 }}
                >
                  <div className="flex items-start justify-between gap-4 mb-5">
                    <div>
                      <Dialog.Title className="text-lg font-semibold">{title}</Dialog.Title>
                      <Dialog.Description className={description ? "text-sm muted mt-1" : "sr-only"}>{description ?? title}</Dialog.Description>
                    </div>
                    <Dialog.Close className="btn btn-sm btn-icon" aria-label="关闭"><X size={16} /></Dialog.Close>
                  </div>
                  {children}
                </motion.div>
              </Dialog.Content>
            </div>
          </Dialog.Portal>
        )}
      </AnimatePresence>
    </Dialog.Root>
  );
}

/* ---------- Form bits ---------- */
export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="block">
      <span className="label">{label}</span>
      {children}
      {hint && <span className="block text-xs muted mt-1">{hint}</span>}
    </label>
  );
}

export function Switch({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <button
      type="button" role="switch" aria-checked={checked} onClick={() => onChange(!checked)}
      className="flex items-center justify-between gap-4 w-full text-left py-1 cursor-pointer"
    >
      <span className="text-sm">{label}</span>
      <span className={`relative h-6 w-11 rounded-full transition-colors ${checked ? "bg-[var(--accent)]" : "bg-slate-400/40"}`}>
        <span className={`absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all ${checked ? "left-[22px]" : "left-0.5"}`} />
      </span>
    </button>
  );
}

export function ErrorText({ children }: { children?: ReactNode }) {
  if (!children) return null;
  return <p role="alert" className="text-sm text-[var(--bad)] bg-red-500/10 rounded-lg px-3 py-2">{children}</p>;
}

export function Spinner() {
  return <span className="inline-block h-4 w-4 rounded-full border-2 border-current border-t-transparent animate-spin" />;
}

/* ---------- Toasts ---------- */
const ToastCtx = createContext<(msg: string, kind?: "ok" | "err") => void>(() => {});
export const useToast = () => useContext(ToastCtx);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<{ id: number; msg: string; kind: "ok" | "err" }[]>([]);
  const push = useCallback((msg: string, kind: "ok" | "err" = "ok") => {
    const id = Date.now() + Math.random();
    setItems((x) => [...x, { id, msg, kind }]);
    setTimeout(() => setItems((x) => x.filter((i) => i.id !== id)), 3500);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="fixed bottom-4 left-1/2 -translate-x-1/2 z-[60] flex flex-col gap-2 items-center pointer-events-none" aria-live="polite">
        <AnimatePresence>
          {items.map((t) => (
            <motion.div key={t.id} initial={{ opacity: 0, y: 12, scale: 0.96 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0 }}
              className="glass-strong rounded-full px-4 py-2 text-sm flex items-center gap-2">
              {t.kind === "ok" ? <Check size={14} className="text-[var(--ok)]" /> : <X size={14} className="text-[var(--bad)]" />}
              {t.msg}
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastCtx.Provider>
  );
}

/* ---------- Service icon ---------- */
const GRADIENT = "linear-gradient(135deg,var(--accent),var(--accent-2))";

/**
 * Icon tile (rounded at display time). `icon` may be a built-in name, emoji, image URL or upload path;
 * when empty and `favicon` is given, the site's favicon is used.
 *  - fit="contain": the image sits centred on the tile (`color` is its background) - for small icons.
 *  - fit="cover":   the image fills the whole tile and is clipped by the rounded corners - for complete icons.
 */
export function ServiceIcon({ icon, name, size = 28, color, favicon, fit = "contain" }: {
  icon: string; name: string; size?: number; color?: string; favicon?: string; fit?: "contain" | "cover";
}) {
  const imgSrc = /^(https?:\/\/|\/icons\/)/.test(icon) ? icon : !icon && favicon ? favicon : "";
  const [state, setState] = useState<"loading" | "ok" | "err">("loading");
  useEffect(() => setState("loading"), [imgSrc]);

  const dim = size * 1.7;
  const base = "relative grid place-items-center rounded-2xl shrink-0 overflow-hidden";
  const fg = color ? contrastFg(color) : "#fff";
  const showImg = !!imgSrc && state === "ok";
  const cover = fit === "cover";

  // The image is in the tile from the start (invisible until loaded) so the letter/glyph is the
  // placeholder while it loads and the fallback if it fails.
  const Lucide = !imgSrc && /^[a-z0-9-]+$/.test(icon) ? ICONS[icon] : null;
  const glyph = icon && !imgSrc && !/^[a-z0-9-]+$/.test(icon) ? icon : name.trim().charAt(0).toUpperCase();
  const bg = showImg && !color ? undefined : color || GRADIENT;
  const imgClass = !showImg
    ? "absolute opacity-0 pointer-events-none"
    : cover ? "absolute inset-0 h-full w-full object-cover" : "object-contain";

  return (
    <div className={`${base} ${showImg && !color ? "bg-white/70 dark:bg-white/10" : "font-semibold"}`}
      style={{ width: dim, height: dim, background: bg, color: fg, fontSize: size * 0.8 }}>
      {!showImg && (Lucide ? <Lucide size={size} strokeWidth={1.8} /> : glyph)}
      {imgSrc && state !== "err" && (
        <img src={imgSrc} alt="" width={cover ? dim : size * 1.3} height={cover ? dim : size * 1.3} referrerPolicy="no-referrer" decoding="async"
          draggable={false} className={imgClass} onLoad={() => setState("ok")} onError={() => setState("err")} />
      )}
    </div>
  );
}
