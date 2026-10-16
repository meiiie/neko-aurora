import { useEffect, useRef, useState } from "react";
import type { NekoBotLocalBridge } from "../nekobot-local-bridge";
import { projectNekoBotReply } from "../nekobot-local-wiii";
import { clearNekoComposerDraft, readNekoComposerDraft, writeNekoComposerDraft } from "../composer-drafts";

export interface WiiiNekoBotSurface {
  bridge: NekoBotLocalBridge;
  projectId: string;
  channelName: string;
  bots: readonly { id: string; name: string }[];
}

function historyNotice(code: string | null): string | null {
  if (!code) return null;
  if (code === "history_repair_required") return "Lịch sử đã lưu đang ẩn sau khi mở lại. Chọn Khôi phục lịch sử kênh để kiểm tra quyền và dữ liệu hiện tại; tin đã lưu được giữ nguyên.";
  if (code === "history_repair_unsupported") return "Neko Bot đang kết nối chưa hỗ trợ khôi phục lịch sử. Tin và nonce đã lưu được giữ nguyên.";
  if (["adapter_history_scope_changed", "adapter_history_authority_changed", "adapter_history_binding_mismatch", "connection_changed", "unknown_link", "disconnected"].includes(code))
    return `Phiên hoặc quyền kênh đã đổi (${code}). Lịch sử đang ẩn; kiểm tra kết nối đúng kênh. Tin đã lưu được giữ nguyên.`;
  if (["adapter_history_cursor_invalid", "adapter_history_snapshot_expired"].includes(code))
    return `Lần đối chiếu đã hết hiệu lực (${code}). Có thể chọn khôi phục lại; tin đã lưu được giữ nguyên.`;
  if (["http_409", "http_413", "adapter_history_budget_exceeded", "history_storage_budget_exceeded"].includes(code))
    return `Lịch sử dừng ở giới hạn (${code}). Chọn khôi phục để đối chiếu từng trang; không có tin nào tự gửi lại.`;
  return `Chưa đối chiếu được lịch sử (${code}). Tin và nonce đã lưu được giữ; lịch sử đang ẩn.`;
}

/** Separate from ACP drafts: this surface sends explicit local Neko Bot intent, never a workspace or grant. */
export function NekoBotLocalPanel({ surface }: { surface: WiiiNekoBotSurface }) {
  const { bridge } = surface;
  const draftScope = `nekobot:${surface.projectId}:${bridge.source.sourceSessionId}:${bridge.source.sourceThreadId}:${bridge.source.channelId}:${bridge.source.expectedChannelRevision}`;
  const [, changed] = useState(0);
  const [draft, setDraftState] = useState(() => readNekoComposerDraft(draftScope));
  const setDraft = (value: string) => { setDraftState(value); writeNekoComposerDraft(draftScope, value); };
  const [botId, setBotId] = useState(surface.bots[0]?.id ?? "");
  const [pending, setPending] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [historyPage, setHistoryPage] = useState(0);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const lifetime = useRef(0);
  const sending = useRef(false);
  useEffect(() => {
    lifetime.current += 1;
    const generation = lifetime.current;
    const unsubscribe = bridge.subscribe(() => changed(n => n + 1));
    void bridge.hydrate().catch(() => {
      if (lifetime.current === generation) setNotice("Không đọc được trạng thái bridge. Dữ liệu cũ được giữ nguyên.");
    });
    return () => { lifetime.current += 1; unsubscribe(); };
  }, [bridge]);

  const run = async (action: () => Promise<void>) => {
    if (sending.current) return;
    sending.current = true;
    const generation = lifetime.current;
    setPending(true); setNotice(null);
    try { await action(); } catch {
      if (lifetime.current === generation && bridge.error) { setNotice(historyNotice(bridge.error)); return; }
      if (lifetime.current === generation) setNotice("Chưa xác nhận được kết quả. Tin và nonce cũ vẫn được giữ; kiểm tra hoặc thử lại đúng tin.");
    } finally {
      sending.current = false;
      if (lifetime.current === generation) setPending(false);
    }
  };
  const send = () => {
    const text = draft;
    const selectedBot = botId;
    const generation = lifetime.current;
    if (!text.trim() || !surface.bots.some(b => b.id === selectedBot) || !bridge.ready) return;
    void run(async () => {
      const item = await bridge.enqueue({ sourceMessageId: `wiii-message-${crypto.randomUUID()}`, botId: selectedBot,
        text, lane: "foreground" });
      if (lifetime.current === generation && draftRef.current === text) {
        if (readNekoComposerDraft(draftScope) === text) clearNekoComposerDraft(draftScope);
        setDraftState("");
      }
      await bridge.dispatch(item.input.sourceMessageId);
      await bridge.refresh();
    });
  };
  const replyCount = bridge.replyCount;
  const page = Math.min(historyPage, Math.max(0, Math.ceil(replyCount / 100) - 1));
  const replies = bridge.replyWindow(page * 100);
  return <section aria-label="Neko Bot local" className="mb-4 rounded-xl border border-[var(--nk-border)] p-3 text-[var(--nk-text)]">
    <h2 className="text-sm font-medium">Neko Bot · {surface.channelName}</h2>
    <p className="mt-1 text-xs text-[var(--nk-text-2)]">Trao đổi trong kênh đã nối với Wiii. Project và quyền chạy trên máy không được gửi theo.</p>
    <label className="mt-3 block text-xs">Bot nhận yêu cầu
      <select value={botId} onChange={e => setBotId(e.target.value)} className="mt-1 block min-h-11 w-full rounded border bg-[var(--nk-composer)] p-2">
        {surface.bots.map(bot => <option key={bot.id} value={bot.id}>{bot.name}</option>)}
      </select>
    </label>
    <label className="mt-3 block text-xs">Tin gửi tới Neko Bot
      <textarea value={draft} onChange={e => setDraft(e.target.value)} rows={3} className="mt-1 block w-full rounded border bg-[var(--nk-composer)] p-2 text-base" />
    </label>
    <div className="mt-2 flex flex-wrap gap-2">
      <button type="button" aria-disabled={pending || !bridge.ready || !draft.trim() || !botId} onClick={send}
        className="min-h-11 rounded border px-3 text-sm">Gửi vào kênh</button>
      <button type="button" aria-disabled={pending || !bridge.ready} onClick={() => { if (bridge.ready) void run(() => bridge.refresh()); }}
        className="min-h-11 rounded border px-3 text-sm">Cập nhật trả lời</button>
    </div>
    {bridge.snapshot.adapterSession && <button type="button" disabled={pending || !bridge.ready || bridge.historySupport === "unsupported"}
      onClick={() => { const generation = lifetime.current; void run(() => bridge.repairHistory(() => lifetime.current === generation)); }}
      className="mt-2 min-h-11 rounded border px-3 text-sm">Khôi phục lịch sử kênh</button>}
    {bridge.historySupport === "unsupported" && <p className="mt-2 text-xs">Kết nối này chưa hỗ trợ khôi phục lịch sử theo trang.</p>}
    <p role="status" aria-live="polite" className="mt-2 text-xs">{pending ? "Đang đối chiếu với Neko Bot…" : notice ?? historyNotice(bridge.error) ?? (bridge.snapshot.blocked ? "Quyền hoặc phiên kênh đã thay đổi. Cần nối lại đúng kênh; lịch sử bị ẩn." : "Bridge local; provider thật chưa được xác minh.")}</p>
    <ul className="mt-3 space-y-2" aria-label="Tin đã lưu để gửi">
      {bridge.snapshot.outbox.filter(item => item.state !== "accepted").map(item => <li key={item.input.sourceMessageId} className="rounded border p-2 text-xs">
        <p className="whitespace-pre-wrap break-words">{item.input.text}</p>
        <span>{item.state === "rejected" ? "Bị từ chối" : item.state === "uncertain" ? "Chưa rõ kết quả gửi" : "Đã lưu, chưa gửi"}</span>
        {item.state !== "rejected" && <button type="button" aria-disabled={pending} className="ml-2 min-h-11 underline"
          onClick={() => { void run(async () => { await bridge.dispatch(item.input.sourceMessageId); await bridge.refresh(); }); }}>Thử lại đúng tin</button>}
        {item.state !== "uncertain" && <button type="button" aria-disabled={pending} className="ml-2 min-h-11 underline"
          onClick={() => { void run(() => bridge.discard(item.input.sourceMessageId)); }}>Bỏ khỏi hàng đợi</button>}
      </li>)}
    </ul>
    <ol className="mt-3 space-y-2" aria-label="Trả lời từ kênh Neko Bot">
      {replies.map(reply => <li key={reply.replyId} className="rounded bg-[var(--nk-inset)] p-2">
        <p className="text-xs">{surface.bots.find(bot => bot.id === reply.botId)?.name ?? reply.botId} · {reply.status}</p>
        <p className="mt-1 whitespace-pre-wrap break-words text-sm">{projectNekoBotReply(reply).blocks[0].content}</p>
        {"availability" in reply && reply.availability === "source_unavailable" && <p className="mt-1 text-xs">Nguồn trả lời hiện chưa được phép xem. Định danh và trạng thái được giữ; nội dung và tệp đang ẩn.</p>}
        {reply.artifacts.map(file => <p key={file.id} className="mt-1 break-words text-xs">Tệp trả về: {file.filename} · {file.bytes} bytes · SHA256 {file.sha256}</p>)}
      </li>)}
    </ol>
    {replyCount > 0 && <nav aria-label="Trang lịch sử Neko Bot" className="mt-2 flex flex-wrap items-center gap-2 text-xs">
      <button type="button" disabled={page === 0} className="min-h-11 rounded border px-3" onClick={() => setHistoryPage(page - 1)}>Trang trước</button>
      <span>{page * 100 + 1}–{Math.min((page + 1) * 100, replyCount)} / {replyCount} trả lời</span>
      <button type="button" disabled={(page + 1) * 100 >= replyCount} className="min-h-11 rounded border px-3" onClick={() => setHistoryPage(page + 1)}>Trang sau</button>
    </nav>}
  </section>;
}
