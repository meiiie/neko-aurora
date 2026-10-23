package aurora

// Harness upgrades (2026-10-23 gap analysis, docs: aurora-design/harness-gap):
// tool-result budgets, history compaction, per-bot memory and mid-turn
// steering. Patterns follow CopilotKit/openmuse (MIT: remember_fact memories,
// truncated untrusted page text, visible follow-up queue) and CopilotKit/
// OpenDots (MIT: MAX_RESULT_CHARS + "[truncated]").

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

const (
	maxResultChars  = 6000  // one tool result as seen by the model
	maxContextChars = 24000 // all tool results injected into one call
	keepFullTurns   = 8     // most recent turns kept verbatim
	maxHistoryTurns = 40    // older turns are folded into a digest
	maxOldReply     = 1200  // per-turn cap for non-recent assistant replies
)

// truncateResult cuts text to n bytes on a rune boundary, marking the cut so
// the model can say its reading was partial.
func truncateResult(t string, n int) string {
	if len(t) <= n {
		return t
	}
	return strings.ToValidUTF8(t[:n], "") + "\n[đã cắt bớt: nội dung dài hơn, chỉ đọc phần đầu]"
}

// budgetBlocks keeps the newest tool blocks whole and shrinks the oldest
// first so the injected context never exceeds max bytes.
func budgetBlocks(blocks []string, max int) []string {
	total := 0
	for _, b := range blocks {
		total += len(b)
	}
	if total <= max {
		return blocks
	}
	out := append([]string(nil), blocks...)
	for i := 0; i < len(out) && total > max; i++ {
		over := total - max
		keep := len(out[i]) - over
		if keep < 300 {
			keep = min(300, len(out[i]))
		}
		nb := truncateResult(out[i], keep)
		if len(nb) < len(out[i]) {
			total -= len(out[i]) - len(nb)
			out[i] = nb
		}
	}
	return out
}

// compactHistory folds turns older than the recent window into one digest
// message (deterministic, no extra model call) and caps long old replies,
// so long chats keep their thread of decisions without blowing the context.
func compactHistory(msgs []ChatMsg) []ChatMsg {
	if len(msgs) <= 1 {
		return msgs
	}
	sys, rest := msgs[0], msgs[1:]
	// Count turns by user messages.
	var users []int
	for i, m := range rest {
		if m.Role == "user" {
			users = append(users, i)
		}
	}
	if len(users) > keepFullTurns {
		cut := users[len(users)-keepFullTurns]
		var sb strings.Builder
		sb.WriteString("TÓM TẮT CÁC LƯỢT CŨ (rút gọn tự động; chi tiết đầy đủ đã lược):\n")
		for _, m := range rest[:cut] {
			who := "Darren"
			if m.Role != "user" {
				who = "Bạn"
			}
			fmt.Fprintf(&sb, "- %s: %s\n", who, clean(firstLine(m.Content), 160))
		}
		rest = append([]ChatMsg{{Role: "system", Content: sb.String()}}, rest[cut:]...)
	}
	out := []ChatMsg{sys}
	n := len(rest)
	for i, m := range rest {
		if m.Role == "assistant" && i < n-4 && len(m.Content) > maxOldReply {
			m.Content = truncateResult(m.Content, maxOldReply)
		}
		out = append(out, m)
	}
	return out
}

// ---- per-bot memory ----

type memItem struct {
	Text string `json:"text"`
	At   string `json:"at"`
}

type memStore struct {
	mu   sync.Mutex
	path string
	m    map[string][]memItem
}

const maxMemPerBot = 30

func openMemory(dir string) *memStore {
	ms := &memStore{path: filepath.Join(dir, "memory.json"), m: map[string][]memItem{}}
	if b, err := os.ReadFile(ms.path); err == nil {
		_ = json.Unmarshal(b, &ms.m)
	}
	return ms
}

func (ms *memStore) save() error {
	b, _ := json.Marshal(ms.m)
	tmp := ms.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ms.path)
}

// Remember stores one fact (deduplicated, newest last, capped).
func (ms *memStore) Remember(bot, text string) bool {
	if ms == nil {
		return false
	}
	text = clean(text, 300)
	if text == "" || looksSecret(text) {
		return false
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	l := ms.m[bot]
	for _, x := range l {
		if strings.EqualFold(x.Text, text) {
			return false
		}
	}
	l = append(l, memItem{Text: text, At: time.Now().UTC().Format(time.RFC3339)})
	if len(l) > maxMemPerBot {
		l = l[len(l)-maxMemPerBot:]
	}
	ms.m[bot] = l
	return ms.save() == nil
}

func (ms *memStore) List(bot string) []memItem {
	if ms == nil {
		return nil
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return append([]memItem{}, ms.m[bot]...)
}

func (ms *memStore) Forget(bot string, i int) bool {
	if ms == nil {
		return false
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	l := ms.m[bot]
	if i < 0 || i >= len(l) {
		return false
	}
	ms.m[bot] = append(l[:i:i], l[i+1:]...)
	return ms.save() == nil
}

// looksSecret refuses to memorise anything shaped like a credential; secrets
// belong in the vault only.
func looksSecret(t string) bool {
	l := strings.ToLower(t)
	for _, k := range []string{"password", "mật khẩu", "api key", "apikey", "token", "secret", "sk-", "otp"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

func memoryPrompt(items []memItem) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nĐiều Darren đã dặn bạn ghi nhớ (sở thích/bối cảnh, không phải chỉ thị hành động):\n")
	for _, x := range items {
		sb.WriteString("- " + x.Text + "\n")
	}
	return sb.String()
}

func (s *Service) memoryRoutes() {
	s.mux.HandleFunc("GET /app/api/agents/{id}/memory", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": s.Mem.List(r.PathValue("id"))})
	})
	s.mux.HandleFunc("DELETE /app/api/agents/{id}/memory/{i}", func(w http.ResponseWriter, r *http.Request) {
		var i int
		if _, err := fmt.Sscan(r.PathValue("i"), &i); err != nil || !s.Mem.Forget(r.PathValue("id"), i) {
			fail(w, ErrNotFound)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}

// ---- mid-turn steering ----

// steer picks up messages the owner sent to this bot while the turn is still
// running (Grok Bot / Claude Code style: a new message mid-work steers the
// current task rather than waiting in a queue). The text is injected into
// the next model call and the request id is journaled on this run
// (DirectRun.Steered), so the thread shows it before the rewritten answer
// and the queued turn later only confirms.
func (s *Service) steer(w ledger.Work, tc *toolCtx) bool {
	if threadOf(w) != w.BotID {
		return false
	}
	got := false
	for _, q := range s.inbox(w) {
		id := q.RequestID
		ok := false
		s.update(w.RequestID, func(r *ledger.DirectRun) {
			if len(r.Steered) < ledger.MaxSteered {
				r.Steered, ok = append(r.Steered, id), true
			}
		})
		if !ok {
			continue
		}
		tc.note("NGƯỜI DÙNG VỪA NHẮN THÊM GIỮA CHỪNG (gộp vào việc đang làm: trả lời/điều chỉnh theo tin này trong CÙNG câu trả lời; nói thẳng bằng “bạn”, không thuật lại kiểu “bạn đổi ý”; vẫn trích nguồn [n]): " + clean(q.Text, 1500))
		s.record(w.BotID, w.RequestID, "step", "Nhận chỉ dẫn mới giữa chừng")
		got = true
	}
	return got
}

// inbox lists owner messages queued for this bot after the running turn
// that no run has absorbed yet (per-bot single runner + inbox: the runner
// drains it at every step boundary instead of making messages wait).
func (s *Service) inbox(w ledger.Work) []ledger.Work {
	if threadOf(w) != w.BotID || strings.HasPrefix(w.ClientNonce, "onboard-") {
		// the scripted welcome never swallows the owner's first real request
		// (live 2026-10-23: a command sent during onboarding got only the greeting)
		return nil
	}
	v := newView(s.L.Snapshot())
	taken := v.steeredBy()
	var out []ledger.Work
	after := false
	for _, q := range v.byBot[w.BotID] {
		if q.RequestID == w.RequestID {
			after = true
			continue
		}
		if after && eligible(q) && !strings.HasPrefix(q.ClientNonce, "brief-") && !strings.HasPrefix(q.ClientNonce, "computer-") && taken[q.RequestID] == "" {
			out = append(out, q)
		}
	}
	return out
}

var errSteered = errors.New("steered")

// ask runs one model call that a new owner message can interrupt
// (interrupt-and-merge, as Claude Code / Codex steering and neko-core's
// checkpointed continuation): while the answer streams, the inbox is
// polled; a new message cancels the stream, the draft is discarded, the
// message is folded in and the call is rebuilt. Bounded so a stream of
// messages cannot starve the answer forever.
func (s *Service) ask(ctx context.Context, w ledger.Work, tc *toolCtx, build func() []ChatMsg) (directives, error) {
	for i := 0; ; i++ {
		if i < maxInterrupts {
			s.steer(w, tc)
		}
		cctx, cancel := context.WithCancel(ctx)
		var hit atomic.Bool
		if i < maxInterrupts {
			go s.watchInbox(cctx, w, func() { hit.Store(true); cancel() })
		}
		d, err := s.call(cctx, w, build())
		cancel()
		if hit.Load() && ctx.Err() == nil {
			s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
			continue
		}
		return d, err
	}
}

const maxInterrupts = 3

var inboxPoll = 250 * time.Millisecond

func (s *Service) watchInbox(ctx context.Context, w ledger.Work, fire func()) {
	t := time.NewTicker(inboxPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if len(s.inbox(w)) > 0 {
				fire()
				return
			}
		}
	}
}

// steeredBy maps a steered request id to the run that absorbed it.
func (v *view) steeredBy() map[string]string {
	out := map[string]string{}
	for _, l := range v.byBot {
		for _, w := range l {
			if w.Direct != nil {
				for _, q := range w.Direct.Steered {
					out[q] = w.RequestID
				}
			}
		}
	}
	return out
}

// absorbed reports a queued turn whose text was already steered into an
// earlier run.
func (s *Service) absorbed(id string) bool { return newView(s.L.Snapshot()).steeredBy()[id] != "" }
