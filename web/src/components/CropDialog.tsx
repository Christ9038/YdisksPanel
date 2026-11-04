import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "@/lib/api";
import { errMsg } from "@/lib/hooks";
import { ErrorText, Modal, Spinner } from "./ui";

const V = 288; // viewport side in CSS px
const MAX_BYTES = 5 << 20;

/** Pick the square part of an image to use as an icon; the server does the actual crop and resize. */
export function CropDialog({ file, onClose, onUploaded }: { file: File | null; onClose: () => void; onUploaded: (url: string) => void }) {
  return (
    <Modal open={!!file} onOpenChange={(o) => !o && onClose()} title="裁切图标" description="拖动图片调整位置，用滑块缩放。图标会被裁成正方形并缩小到最长边 256px。">
      {file && <Cropper key={`${file.name}-${file.size}-${file.lastModified}`} file={file} onClose={onClose} onUploaded={onUploaded} />}
    </Modal>
  );
}

function Cropper({ file, onClose, onUploaded }: { file: File; onClose: () => void; onUploaded: (url: string) => void }) {
  const url = useMemo(() => URL.createObjectURL(file), [file]);
  useEffect(() => () => URL.revokeObjectURL(url), [url]);

  const [nat, setNat] = useState<{ w: number; h: number } | null>(null);
  const [s, setS] = useState(1); // crop side, in source pixels
  const [c, setC] = useState({ x: 0, y: 0 }); // crop centre, in source pixels
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const drag = useRef<{ x: number; y: number } | null>(null);

  const tooBig = file.size > MAX_BYTES;
  const sMax = nat ? Math.max(nat.w, nat.h) : 1; // whole picture fits inside the square
  const sMin = nat ? Math.max(16, Math.min(nat.w, nat.h) / 4) : 1; // 4x zoom-in
  const k = V / s;

  const clampC = (x: number, y: number) => (nat ? { x: Math.min(nat.w, Math.max(0, x)), y: Math.min(nat.h, Math.max(0, y)) } : { x, y });
  const fill = () => nat && (setS(Math.min(nat.w, nat.h)), setC({ x: nat.w / 2, y: nat.h / 2 }));
  const fitAll = () => nat && (setS(Math.max(nat.w, nat.h)), setC({ x: nat.w / 2, y: nat.h / 2 }));

  // slider is logarithmic so zooming feels even across a wide range
  const t = nat && sMax > sMin ? Math.log(s / sMax) / Math.log(sMin / sMax) : 0;
  const setT = (v: number) => setS(sMax * Math.pow(sMin / sMax, v));

  const confirm = async () => {
    if (!nat) return;
    setBusy(true);
    setErr("");
    try {
      const fd = new FormData();
      fd.append("file", file);
      fd.append("cropX", String(Math.round(c.x - s / 2)));
      fd.append("cropY", String(Math.round(c.y - s / 2)));
      fd.append("cropSize", String(Math.max(8, Math.round(s))));
      const r = await api<{ url: string }>("POST", "/api/uploads/icon", fd);
      onUploaded(r.url);
      onClose();
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-center">
        <div
          className="relative overflow-hidden rounded-[22%] touch-none cursor-grab active:cursor-grabbing ring-1 ring-[var(--line)]"
          style={{ width: V, height: V, backgroundColor: "#fff", backgroundImage: "conic-gradient(#e5e7eb 25%, transparent 0 50%, #e5e7eb 0 75%, transparent 0)", backgroundSize: "16px 16px" }}
          onPointerDown={(e) => { e.currentTarget.setPointerCapture(e.pointerId); drag.current = { x: e.clientX, y: e.clientY }; }}
          onPointerMove={(e) => {
            if (!drag.current || !nat) return;
            const dx = e.clientX - drag.current.x, dy = e.clientY - drag.current.y;
            drag.current = { x: e.clientX, y: e.clientY };
            setC((p) => clampC(p.x - dx / k, p.y - dy / k));
          }}
          onPointerUp={() => (drag.current = null)}
          onPointerCancel={() => (drag.current = null)}
        >
          <img
            src={url} alt="待裁切的图片" draggable={false}
            onLoad={(e) => {
              const w = e.currentTarget.naturalWidth, h = e.currentTarget.naturalHeight;
              setNat({ w, h });
              setS(Math.min(w, h));
              setC({ x: w / 2, y: h / 2 });
            }}
            style={nat ? { position: "absolute", maxWidth: "none", width: nat.w * k, height: nat.h * k, left: -(c.x - s / 2) * k, top: -(c.y - s / 2) * k, userSelect: "none" } : { visibility: "hidden" }}
          />
        </div>
      </div>

      <div className="flex items-center gap-3">
        <span className="text-xs muted shrink-0">缩小</span>
        <input type="range" min={0} max={1} step={0.001} value={t} onChange={(e) => setT(Number(e.target.value))} className="w-full accent-[var(--accent)]" aria-label="缩放" disabled={!nat} />
        <span className="text-xs muted shrink-0">放大</span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <button type="button" className="btn btn-sm" onClick={fill} disabled={!nat}>填满（裁掉多余部分）</button>
        <button type="button" className="btn btn-sm" onClick={fitAll} disabled={!nat}>完整显示（两侧留透明）</button>
        {nat && <span className="text-xs muted ml-auto">原图 {nat.w}×{nat.h}</span>}
      </div>

      <ErrorText>{tooBig ? "图片超过 5MB，请先压缩后再上传" : err}</ErrorText>
      <div className="flex justify-end gap-2 pt-1">
        <button type="button" className="btn" onClick={onClose}>取消</button>
        <button type="button" className="btn btn-primary" onClick={confirm} disabled={busy || !nat || tooBig}>{busy && <Spinner />}确认并上传</button>
      </div>
    </div>
  );
}
