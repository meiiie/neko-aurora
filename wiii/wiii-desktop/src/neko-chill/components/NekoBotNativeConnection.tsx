import { useEffect, useRef, useState } from "react";
import { loadStoreStrict, saveStoreStrict } from "../../lib/storage";
import { NekoBotNativeClient, nativeRuntimeAvailable, nativeSourceIdentity, type NativeStatus } from "../nekobot-native";
import { createWiiiNekoBotBridge } from "../nekobot-local-wiii";
import { NekoBotLocalPanel, type WiiiNekoBotSurface } from "./NekoBotLocalPanel";

export function NekoBotNativeConnection({ projectId }: { projectId: string }) {
  const [client, setClient] = useState<NekoBotNativeClient | null>(null);
  const [status, setStatus] = useState<NativeStatus | null>(null);
  const [port, setPort] = useState("8765");
  const [channelId, setChannelId] = useState("");
  const [surface, setSurface] = useState<WiiiNekoBotSurface | null>(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const generation = useRef(0);
  const pending = useRef(false);
  useEffect(() => {
    const ticket = ++generation.current;
    pending.current = false; setBusy(false);
    setSurface(null); setChannelId(""); setStatus(null); setClient(null);
    if (nativeRuntimeAvailable()) {
      void import("@tauri-apps/api/core").then(async ({ invoke }) => {
        const client = new NekoBotNativeClient((command, args) => invoke(command, args));
        const status = await client.status();
        if (generation.current === ticket) { setClient(client); setStatus(status); }
      }).catch(() => { if (generation.current === ticket) setNotice("Kết nối native chưa sẵn sàng. Dữ liệu và bản nháp được giữ nguyên."); });
    }
    return () => { ++generation.current; };
  }, [projectId]);
  const run = async (action: (ticket: number) => Promise<void>) => {
    if (pending.current) return;
    pending.current = true; setBusy(true); setNotice(""); const ticket = generation.current;
    try { await action(ticket); } catch { if (ticket === generation.current) setNotice("Chưa kết nối hoặc binding đã đổi. Không tự đổi kênh hay gửi lại; kiểm tra service rồi chọn kết nối rõ ràng."); }
    finally { if (ticket === generation.current) { pending.current = false; setBusy(false); } }
  };
  if (!client) return <section aria-label="Neko Bot native" className="mb-4 rounded-xl border border-[var(--nk-border)] p-3 text-[var(--nk-text)]">
    <p>Neko Bot local · Chưa kết nối</p><p className="text-sm">Kết nối này cần Wiii desktop và xác nhận native của chủ sở hữu.</p><p role="status">{notice}</p>
  </section>;
  return <section aria-label="Kết nối Neko Bot local" className="mb-4 rounded-xl border border-[var(--nk-border)] p-3 text-[var(--nk-text)]">
    <p className="font-medium">Neko Bot local · {status?.connected ? "Đã kết nối" : "Chưa kết nối"}</p>
    <p className="text-sm">Cookie chỉ ở Rust trong phiên hiện tại. Chọn kênh rồi gửi tin riêng; không tự cấp quyền project, không đưa lịch sử vào ACP.</p>
    {!status?.connected ? <form onSubmit={event => { event.preventDefault(); void run(async ticket => {
      setSurface(null); const next = await client.connect(Number(port));
      if (ticket === generation.current) { setStatus(next); setChannelId(""); }
    }); }} className="flex flex-wrap items-end gap-2 mt-2">
      <label>Cổng local <input aria-label="Cổng Neko Bot local" type="number" min="1" max="65535" value={port} onChange={e => setPort(e.target.value)} className="block min-h-11 w-28 rounded border p-2 bg-transparent" /></label>
      <button type="submit" aria-disabled={busy} className="min-h-11 rounded border px-3">Kết nối và xác nhận</button>
    </form> : <div className="flex flex-wrap items-end gap-2 mt-2">
      <label>Kênh Neko Bot <select aria-label="Kênh Neko Bot" value={channelId} onChange={e => { ++generation.current; pending.current = false; setBusy(false); setChannelId(e.target.value); setSurface(null); }} className="block min-h-11 rounded border p-2 bg-transparent">
        <option value="">Chọn kênh rõ ràng</option>{status.catalog?.channels.map(c => <option key={c.id} value={c.id}>{c.name} · rev {c.revision}</option>)}
      </select></label>
      <button type="button" aria-disabled={busy || !channelId} className="min-h-11 rounded border px-3" onClick={() => { if (!channelId) return; void run(async ticket => {
        const catalog = status.catalog!; const selected = channelId;
        const identity = await nativeSourceIdentity({ read: key => loadStoreStrict("nekobot-native-source-v1.json", key, null), write: (key, value) => saveStoreStrict("nekobot-native-source-v1.json", key, value) }, catalog, selected, projectId);
        const link = await client.link(catalog, selected, projectId, identity);
        if (ticket === generation.current) setSurface({ bridge: createWiiiNekoBotBridge(link.source, client.transport(link)), projectId, channelName: link.channelName, bots: link.bots });
      }); }}>Mở bridge cho kênh này</button>
      <button type="button" className="min-h-11 rounded border px-3" onClick={() => {
        const ticket = ++generation.current; setSurface(null); setChannelId(""); setStatus(null); setBusy(false); pending.current = false;
        void client.disconnect().then(() => client.status()).then(next => { if (ticket === generation.current) setStatus(next); }).catch(() => { if (ticket === generation.current) setNotice("Chưa xác nhận được trạng thái ngắt kết nối."); });
      }}>Ngắt kết nối</button>
    </div>}
    <p role="status" aria-live="polite">{busy ? "Đang chờ native…" : notice}</p>
    {surface && <NekoBotLocalPanel key={`${projectId}:${surface.bridge.source.channelId}:${surface.bridge.source.expectedChannelRevision}`} surface={surface} />}
  </section>;
}
