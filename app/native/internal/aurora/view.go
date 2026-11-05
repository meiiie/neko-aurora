package aurora

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

var (
	ErrNotFound = errors.New("not_found")
	ErrInvalid  = errors.New("invalid")
	ErrConflict = errors.New("conflict")
)

func newID(p string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return p + hex.EncodeToString(b[:])
}

func clean(v string, n int) string {
	v = strings.TrimSpace(strings.ToValidUTF8(v, ""))
	if r := []rune(v); len(r) > n {
		v = string(r[:n])
	}
	return v
}

var (
	mdLead  = regexp.MustCompile(`^\s*(#{1,6}\s+|>\s*|[-*+]\s+|\d+[.)]\s+)`)
	mdLink  = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	mdCite  = regexp.MustCompile(`\s?\[\d+\]`)
	mdMarks = strings.NewReplacer("**", "", "__", "", "`", "")
)

// plainMD turns one markdown line into list-preview text: no heading hashes,
// bullets, emphasis marks, citation markers or link URLs.
func plainMD(v string) string {
	v = mdLead.ReplaceAllString(v, "")
	v = mdLink.ReplaceAllString(v, "$1")
	v = mdCite.ReplaceAllString(v, "")
	return strings.TrimSpace(mdMarks.Replace(v))
}

// ---- views: the mobile JSON shapes, projected from ledger bots and work ----

type Agent struct {
	ActiveModel string         `json:"activeModel,omitempty"`
	AI          map[string]any `json:"ai,omitempty"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Desc        string         `json:"desc"`
	Tag         string         `json:"tag,omitempty"`
	Face        *Face          `json:"av,omitempty"`
	Status      string         `json:"status"` // idle|working|waiting|done|error
	Task        string         `json:"task"`   // OpenMuse-style TaskStatus of the latest task
	Snip        string         `json:"snip"`
	SnipKey     string         `json:"snipKey,omitempty"`
	SnipArgs    map[string]any `json:"snipArgs,omitempty"`
	Doing       string         `json:"doing,omitempty"` // live activity while working ("Đang tìm nguồn…")
	Pinned      bool           `json:"pin"`
	Kind        string         `json:"kind,omitempty"` // ""|group
	Members     []string       `json:"members,omitempty"`
	Updated     time.Time      `json:"t"`
}

type Step = ledger.DirectStep
type Plan struct {
	Steps []Step `json:"steps"`
}
type Approval = ledger.DirectApproval

type Message struct {
	Quote           *ledger.MessageQuote     `json:"quote,omitempty"`
	AnswerFor       string                   `json:"answerFor,omitempty"`
	FormFor         string                   `json:"formFor,omitempty"`
	FormValues      []string                 `json:"formValues,omitempty"`
	ID              string                   `json:"id"`
	TaskID          string                   `json:"taskId"`
	AgentID         string                   `json:"agentId"`
	Role            string                   `json:"role"` // user|agent|system
	Text            string                   `json:"text"`
	Recovery        string                   `json:"recovery,omitempty"`
	Interrupted     bool                     `json:"interrupted,omitempty"`
	Segment         bool                     `json:"segment,omitempty"`
	Completion      *ledger.DirectCompletion `json:"completion,omitempty"`
	BeforeRequestID string                   `json:"beforeRequestId,omitempty"`
	AfterInputID    string                   `json:"-"`
	Approval        *Approval                `json:"approval,omitempty"`
	Plan            *Plan                    `json:"plan,omitempty"`
	Choices         []string                 `json:"choices,omitempty"`
	Questions       []ledger.DirectQuestion  `json:"questions,omitempty"`
	Secret          *ledger.DirectSecret     `json:"secret,omitempty"`
	Tools           []ledger.DirectTool      `json:"tools,omitempty"`
	Sources         []ledger.DirectSource    `json:"sources,omitempty"`
	Queued          bool                     `json:"queued,omitempty"`
	Ref             string                   `json:"ref,omitempty"`  // name of a chat attached as context
	From            string                   `json:"from,omitempty"` // speaking bot, in group threads
	Origin          string                   `json:"origin,omitempty"`
	FromName        string                   `json:"fromName,omitempty"`
	PeerMessageID   string                   `json:"peerMessageId,omitempty"`
	Thinking        string                   `json:"thinking,omitempty"` // folded model reasoning (thinking toggle)
	Form            *form                    `json:"form,omitempty"`     // interactive form card
	Created         time.Time                `json:"t"`
}

type Brief struct {
	Text      string    `json:"text"`
	Items     []string  `json:"items"`
	Generated time.Time `json:"generated"`
	Source    string    `json:"source"` // ai|fallback
}

type Activity struct {
	ID      string    `json:"id"`
	AgentID string    `json:"agentId,omitempty"`
	Text    string    `json:"text"`
	T       time.Time `json:"t"`
}

func ts(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t.Local()
}

// view is one consistent read of the ledger.
type view struct {
	snap     ledger.Snapshot
	archived map[string]bool
	profiles map[string]ledger.BotProfile
	byBot    map[string][]ledger.Work
	byConv   map[string][]ledger.Work // group (ledger channel) threads
	allBot   map[string][]ledger.Work // a bot's own thread plus its group turns
	channels map[string]ledger.Channel
}

func newView(snap ledger.Snapshot) *view {
	v := &view{snap: snap, archived: map[string]bool{}, profiles: map[string]ledger.BotProfile{}, byBot: map[string][]ledger.Work{}, byConv: map[string][]ledger.Work{}, allBot: map[string][]ledger.Work{}, channels: map[string]ledger.Channel{}}
	for _, c := range snap.Channels {
		v.channels[c.ID] = c
	}
	for _, st := range snap.BotStates {
		if st.Archived {
			v.archived[st.BotID] = true
		}
	}
	for _, p := range snap.BotProfiles {
		v.profiles[p.BotID] = p
	}
	for _, w := range snap.Work {
		if w.Collaboration != nil {
			continue
		}
		if w.ConversationID == "conversation-"+w.BotID {
			v.byBot[w.BotID] = append(v.byBot[w.BotID], w)
			v.allBot[w.BotID] = append(v.allBot[w.BotID], w)
		} else if _, ok := v.channels[w.ConversationID]; ok {
			v.byConv[w.ConversationID] = append(v.byConv[w.ConversationID], w)
			v.allBot[w.BotID] = append(v.allBot[w.BotID], w)
		}
	}
	for _, m := range []map[string][]ledger.Work{v.byBot, v.byConv, v.allBot} {
		for _, l := range m {
			sort.SliceStable(l, func(i, j int) bool { return l[i].CreatedSequence < l[j].CreatedSequence })
		}
	}
	return v
}

func (v *view) bot(id string) (ledger.Bot, bool) {
	for _, b := range v.snap.Bots {
		if b.ID == id && !v.archived[id] {
			return b, true
		}
	}
	return ledger.Bot{}, false
}

func (v *view) desc(b ledger.Bot) string {
	if p, ok := v.profiles[b.ID]; ok && strings.TrimSpace(p.Instructions) != "" {
		return p.Instructions
	}
	return b.Description
}

func displayName(b ledger.Bot) string { return b.Name }

func active(w ledger.Work) bool {
	return w.Status == "waiting_host" || w.Status == "starting" || w.Status == "running" || w.Status == "cancelling"
}

func taskStatus(w ledger.Work) TaskStatus {
	switch {
	case w.Direct != nil && w.Direct.Approval != nil && w.Direct.Approval.State == "pending":
		return TaskWaitingApproval
	case w.Direct != nil && (len(w.Direct.Choices) > 0 || len(w.Direct.Questions) > 0) && w.Status == "completed":
		return TaskWaitingInput
	case w.Status == "waiting_host":
		return TaskQueued
	case w.Status == "running" || w.Status == "starting" || w.Status == "cancelling":
		return TaskRunning
	case w.Status == "completed" && w.Direct != nil && w.Direct.Completion != nil && w.Direct.Completion.Status == "incomplete":
		return TaskIncomplete
	case w.Status == "completed":
		return TaskSucceeded
	case w.Status == "cancelled":
		return TaskCancelled
	case w.Status == "paused" || w.Status == "suspended":
		return TaskPaused
	}
	return TaskFailed
}

func (v *view) agent(b ledger.Bot, prefs botPrefs) *Agent {
	a := &Agent{ID: b.ID, Name: displayName(b), Desc: v.desc(b), Tag: prefs.Tag, Pinned: prefs.Pinned, Status: "idle", Snip: "Sẵn sàng nhận việc.", SnipKey: "agent.ready", Task: ""}
	av := defaultAvatar(b.ID)
	if prefs.Avatar != nil {
		av = *prefs.Avatar
	}
	f := faceOf(av)
	a.Face = &f
	l := v.byBot[b.ID]
	if len(l) == 0 {
		return a
	}
	last := l[len(l)-1]
	a.Updated = ts(last.CreatedAt)
	if last.Direct != nil && last.Direct.FinishedAt != "" {
		a.Updated = ts(last.Direct.FinishedAt)
	}
	a.Task = string(taskStatus(last))
	for _, w := range l {
		if w.Direct != nil && w.Direct.Approval != nil && w.Direct.Approval.State == "pending" {
			a.Status, a.Snip = "waiting", "Cần bạn duyệt: "+w.Direct.Approval.Title
			a.SnipKey, a.SnipArgs = "agent.approval", map[string]any{"title": w.Direct.Approval.Title}
		}
	}
	for _, w := range l {
		if w.Direct != nil && w.Direct.Approval != nil && w.Direct.Approval.Kind == "computer" && w.Direct.Approval.Delivery != nil && w.Direct.Approval.Delivery.Status == "sending" {
			// an approved command is still running: the bot is busy, not done
			a.Status, a.Snip = "working", "Đang chạy lệnh trên máy tính…"
			a.SnipKey, a.SnipArgs = "agent.command", nil
		}
		if active(w) {
			a.Status, a.Snip = "working", "Đang làm việc"
			a.SnipKey, a.SnipArgs = "agent.working", nil
			if w.Status == "waiting_host" {
				a.Snip = "Đang chờ lượt"
				a.SnipKey = "agent.queued"
			}
			if w.Direct != nil {
				for i, st := range w.Direct.Plan {
					if st.Status == "running" {
						a.Snip = "Bước " + itoa(i+1) + "/" + itoa(len(w.Direct.Plan)) + ": " + st.Title
						a.SnipKey, a.SnipArgs = "agent.step", map[string]any{"current": i + 1, "total": len(w.Direct.Plan), "title": st.Title}
					}
				}
				a.Doing = doingOf(w.Direct)
				a.ActiveModel = w.Direct.Model
			}
			return a
		}
	}
	if a.Status == "waiting" {
		return a
	}
	switch taskStatus(last) {
	case TaskIncomplete:
		a.Status, a.Snip = "waiting", "Còn phần chưa hoàn tất"
		a.SnipKey, a.SnipArgs = "chat.completion.incomplete", nil
	case TaskWaitingInput:
		a.Status, a.Snip = "waiting", "Cần bạn chọn để tiếp tục"
		a.SnipKey, a.SnipArgs = "agent.choice", nil
	case TaskSucceeded:
		a.Status, a.Snip = "done", clean(plainMD(firstLine(visibleText(last.Reply))), 140)
		a.SnipKey, a.SnipArgs = "", nil
		if a.Snip == "" {
			a.Snip = "Đã xong"
			a.SnipKey = "agent.done"
		}
	case TaskCancelled:
		a.Status, a.Snip = "idle", "Đã dừng theo yêu cầu của bạn."
		a.SnipKey, a.SnipArgs = "agent.stopped", nil
	case TaskFailed:
		a.Status, a.Snip = "error", failureText(last.Failure)
		a.SnipKey, a.SnipArgs = "agent.failed", nil
		if last.Failure == "ai_not_configured" {
			a.Snip = "Cần thiết lập AI để trả lời."
			a.SnipKey = "agent.setup"
		}
	}
	return a
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func failureText(code string) string {
	switch {
	case code == "core_effect_unknown":
		return "Aurora bị ngắt sau khi gửi một thao tác và chưa nhận được kết quả đã lưu. Thao tác không được tự chạy lại. Hãy kiểm tra kết quả trên máy tính của bot trước khi tiếp tục."
	case code == "core_workspace_changed":
		return "Thư mục làm việc đã thay đổi trong lúc bot đang chạy. Aurora đã dừng lượt này để tránh thao tác nhầm thư mục."
	case code == "ai_not_configured":
		return "Tin nhắn đã được lưu nhưng chưa được xử lý vì chưa có kết nối AI. Hãy thiết lập AI, rồi gửi lại tin nhắn khi bạn muốn tiếp tục."
	case code == "core_configuration_invalid":
		return "Aurora chưa khởi động được AI do cấu hình ứng dụng. Hãy cập nhật hoặc khởi động lại Aurora rồi thử lại."
	case code == "core_auth_required":
		return "Kết nối AI chưa được xác thực. Mở AI của bot để kiểm tra API key hoặc đăng nhập lại. Tin nhắn đã được giữ."
	case code == "core_home_unavailable" || code == "core_runtime_unavailable":
		return "Aurora chưa mở được bộ chạy AI trên máy này. Hãy kiểm tra cài đặt Neko Core rồi thử lại."
	case code == "core_failed":
		return "AI chưa trả lời được. Bạn có thể thử lại hoặc kiểm tra kết nối trong thiết lập AI."
	case code == "core_model_unavailable":
		return "Model đang chọn không khả dụng với kết nối này. Mở AI của bot, tải lại danh sách và chọn model khác. Tin nhắn đã được giữ."
	case code == "core_support_required":
		return "Model này cần cập nhật thành phần ChatGPT trên máy chạy Aurora. Đăng nhập của bạn vẫn được giữ."
	case code == "core_catalog_unavailable":
		return "Chưa tải được danh sách model từ nhà cung cấp. Kiểm tra mạng rồi tải lại danh sách trong AI của bot."
	case code == "core_rate_limited":
		return "Nhà cung cấp đang giới hạn lượt dùng hoặc tài khoản đã hết hạn mức. Bạn có thể thử lại sau hoặc kiểm tra gói đang dùng."
	case code == "core_access_denied":
		return "Kết nối này chưa được phép dùng model đã chọn. Hãy kiểm tra quyền của API key hoặc gói tài khoản."
	case code == "core_connection_failed":
		return "Aurora chưa kết nối được với nhà cung cấp AI. Kiểm tra mạng rồi thử lại; thiết lập của bạn vẫn được giữ."
	case code == "interrupted_by_restart":
		return "Tác vụ bị gián đoạn khi máy chủ khởi động lại. Nhắn lại để tiếp tục."
	case code == "stopped_by_owner":
		return "Đã dừng theo yêu cầu của bạn."
	case code == "":
		return "Tác vụ chưa hoàn tất."
	}
	return "Mình chưa xử lý được: " + code
}

// messages projects the bot's tasks into chat bubbles.
func (v *view) messages(botID string, limit int) []*Message {
	out := []*Message{}
	l := v.byBot[botID]
	running := false
	for _, w := range l {
		if w.Status == "running" || w.Status == "starting" {
			running = true
		}
	}
	// A message the owner sent mid-turn is shown where it was taken in:
	// right before the answer it changed, not after it.
	by := v.steeredBy()
	text := map[string]ledger.Work{}
	for _, w := range l {
		text[w.RequestID] = w
	}
	for _, w := range l {
		ms := workMessages(w, running)
		if by[w.RequestID] != "" && len(ms) > 0 && ms[0].Role == "user" {
			ms = ms[1:]
		}
		if w.Direct != nil && w.Direct.Engine == "core" && len(w.Direct.Steered) > 0 {
			ordered := []*Message{}
			next := 0
			insertThrough := func(id string) {
				if id == "" {
					return
				}
				for next < len(w.Direct.Steered) {
					q := w.Direct.Steered[next]
					next++
					if qw, ok := text[q]; ok {
						ordered = append(ordered, &Message{ID: q + ":u", TaskID: q, AgentID: qw.BotID, Role: "user", Text: qw.Text, Created: ts(qw.CreatedAt), AnswerFor: questionAnswered(qw.ClientNonce), FormFor: formAnswered(qw.ClientNonce)})
					}
					if q == id {
						break
					}
				}
			}
			for _, m := range ms {
				if m.Segment && m.AfterInputID != "" {
					// Already inserted anchors must not consume later queued inputs.
					for _, q := range w.Direct.Steered[next:] {
						if q == m.AfterInputID {
							insertThrough(q)
							break
						}
					}
				}
				if m.ID == w.RequestID {
					insertThrough(w.Direct.Steered[len(w.Direct.Steered)-1])
				}
				ordered = append(ordered, m)
			}
			insertThrough(w.Direct.Steered[len(w.Direct.Steered)-1])
			ms = ordered
		} else if w.Direct != nil && len(w.Direct.Steered) > 0 {
			var ins []*Message
			for _, q := range w.Direct.Steered {
				if qw, ok := text[q]; ok {
					for _, m := range ms {
						if m.Interrupted && m.BeforeRequestID == q {
							ins = append(ins, m)
						}
					}
					ins = append(ins, &Message{ID: q + ":u", TaskID: q, AgentID: qw.BotID, Role: "user", Text: qw.Text, Created: ts(qw.CreatedAt), AnswerFor: questionAnswered(qw.ClientNonce), FormFor: formAnswered(qw.ClientNonce)})
				}
			}
			kept := ms[:0]
			for _, m := range ms {
				if !m.Interrupted || by[m.BeforeRequestID] != w.RequestID {
					kept = append(kept, m)
				}
			}
			ms = kept
			pos := len(ms)
			for i, m := range ms {
				if m.ID == w.RequestID {
					pos = i
				}
			}
			ms = append(ms[:pos:pos], append(ins, ms[pos:]...)...)
		}
		out = append(out, ms...)
	}
	// Supply the same ordering anchor to live SSE clients as the journal projection.
	nextUser := ""
	for i := len(out) - 1; i >= 0; i-- {
		m := out[i]
		if m.Role == "user" {
			nextUser = m.TaskID
		} else if !m.Interrupted {
			m.BeforeRequestID = nextUser
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func workMessages(w ledger.Work, botRunning bool) []*Message {
	at := ts(w.CreatedAt)
	d := w.Direct
	out := []*Message{}
	kind := ""
	if d != nil {
		kind = d.Kind
	}
	if kind == "" && strings.HasPrefix(w.ClientNonce, "computer-") {
		kind = "computer" // hidden follow-up/continuation, also while it waits in the queue (live run 11 showed it as the owner's bubble)
	}
	if kind == "" && strings.HasPrefix(w.ClientNonce, handoffPrefix) {
		kind = "handoff"
	}
	if kind == "" {
		out = append(out, &Message{ID: w.RequestID + ":u", TaskID: w.RequestID, AgentID: w.BotID, Role: "user", Text: stripRef(w.Text), Ref: refName(w.Text), Queued: w.Status == "waiting_host" && botRunning, Created: at, AnswerFor: questionAnswered(w.ClientNonce), FormFor: formAnswered(w.ClientNonce)})
	}
	if d == nil {
		if w.Status == "cancelled" {
			out = append(out, &Message{ID: w.RequestID, TaskID: w.RequestID, AgentID: w.BotID, Role: "system", Text: failureText("stopped_by_owner"), Created: at})
		} else if w.Status == "completed" && w.Reply != "" {
			out = append(out, &Message{ID: w.RequestID, TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: w.Reply, Created: at})
		}
		return out
	}
	// The acknowledgement is not its own bubble: one reply per turn, with the
	// live status line ("Đang tìm nguồn…") covering the wait (field report
	// 2026-10-23: a preamble bubble then the full answer read as two replies).
	if len(d.Plan) > 0 {
		out = append(out, &Message{ID: w.RequestID + ":plan", TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: d.PlanIntro, Plan: &Plan{Steps: append([]Step(nil), d.Plan...)}, Created: at})
	}
	for i, st := range d.Plan {
		if st.Note != "" {
			// milestone update: one short message per finished step
			out = append(out, &Message{ID: fmt.Sprintf("%s:step%d", w.RequestID, i+1), TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: fmt.Sprintf("**Xong bước %d/%d · %s**\n\n%s", i+1, len(d.Plan), st.Title, st.Note), Sources: append([]ledger.DirectSource(nil), d.Sources...), Created: at})
		}
	}
	for i, p := range d.Partials {
		out = append(out, &Message{ID: fmt.Sprintf("%s:partial%d", w.RequestID, i+1), TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: p.Text, Interrupted: true, BeforeRequestID: p.BeforeRequestID, Sources: append([]ledger.DirectSource(nil), d.Sources...), Created: ts(p.At)})
	}
	reply := visibleText(w.Reply)
	for _, segment := range d.Segments {
		out = append(out, &Message{ID: w.RequestID + ":segment:" + segment.ID, TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: segment.Text, Segment: true, AfterInputID: segment.AfterInputID, Sources: append([]ledger.DirectSource(nil), d.Sources...), Created: ts(segment.At)})
	}
	if reply != "" || d.Approval != nil || len(d.Choices) > 0 || len(d.Questions) > 0 || d.Secret != nil || len(d.Tools) > 0 {
		m := &Message{ID: w.RequestID, TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: reply, Choices: append([]string(nil), d.Choices...), Questions: ledger.CopyDirectQuestions(d.Questions), Created: at}
		if d.Approval != nil {
			a := *d.Approval
			m.Approval = &a
			if a.Kind == "computer" && strings.HasPrefix(m.Text, computerAskText) {
				// once decided, the bubble says what happened, not "please approve"
				switch a.State {
				case "approved", "done":
					m.Text = "Đã chạy lệnh trên máy tính của mình."
				case "declined", "rejected", "cancelled":
					m.Text = "Bạn đã từ chối lệnh này, mình không chạy."
				}
			}
		}
		if d.Secret != nil {
			x := *d.Secret
			m.Secret = &x
		}
		m.Tools = append([]ledger.DirectTool(nil), d.Tools...)
		m.Completion = d.Completion
		if m.Completion != nil && m.Completion.Status == "waiting" && d.Approval != nil && d.Approval.State != "pending" {
			m.Completion = nil
		}
		m.Sources = append([]ledger.DirectSource(nil), d.Sources...)
		out = append(out, m)
	}
	if d.Approval != nil && d.Approval.Outcome != "" && d.Approval.Kind != "computer" { // computer output renders on its card
		out = append(out, &Message{ID: w.RequestID + ":outcome", TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: d.Approval.Outcome, Created: ts(d.Approval.DecidedAt)})
	}
	if w.Status == "failed" || w.Status == "cancelled" {
		m := &Message{ID: w.RequestID + ":end", TaskID: w.RequestID, AgentID: w.BotID, Role: "system", Text: failureText(w.Failure), Created: at}
		if w.Failure == "ai_not_configured" {
			m.Recovery = "configure_ai"
		} else if strings.HasPrefix(w.Failure, "core_") && w.Failure != "core_effect_unknown" && w.Failure != "core_workspace_changed" {
			m.Recovery = "check_ai"
		}
		out = append(out, m)
	}
	return out
}

func visibleText(s string) string { return strings.TrimSpace(directiveRe.ReplaceAllString(s, "")) }

func (v *view) pending() []*Message {
	out := []*Message{}
	for _, b := range v.snap.Bots {
		if v.archived[b.ID] {
			continue
		}
		for _, w := range v.byBot[b.ID] {
			if w.Direct != nil && w.Direct.Approval != nil && w.Direct.Approval.State == "pending" {
				for _, m := range workMessages(w, false) {
					if m.Approval != nil {
						out = append(out, m)
					}
				}
			}
		}
	}
	for id := range v.channels {
		for _, m := range v.groupMessages(id, 0) {
			if m.Approval != nil && m.Approval.State == "pending" {
				out = append(out, m)
			}
		}
	}
	return out
}

func (v *view) brief() *Brief {
	var best *ledger.Work
	for i := range v.snap.Work {
		w := &v.snap.Work[i]
		if w.Direct != nil && w.Direct.Kind == "brief" && w.Status == "completed" && (best == nil || w.CreatedSequence > best.CreatedSequence) {
			best = w
		}
	}
	if best == nil {
		return nil
	}
	src := "ai"
	if best.Direct.Model == "" {
		src = "fallback"
	}
	return &Brief{Text: visibleText(best.Reply), Items: append([]string{}, best.Direct.Items...), Generated: ts(best.Direct.FinishedAt), Source: src}
}

// doingVerb is the live, human activity shown under a working bot's name.
var doingVerb = map[string]string{"gold_price": "Đang tra giá vàng…", "web_search": "Đang tìm nguồn…", "web_fetch": "Đang đọc trang…"}

func doingOf(r *ledger.DirectRun) string {
	for i := len(r.Tools) - 1; i >= 0; i-- {
		if r.Tools[i].Status == "running" {
			if v, ok := doingVerb[r.Tools[i].Name]; ok {
				return v
			}
			return "Đang dùng công cụ…"
		}
	}
	for i, st := range r.Plan {
		if st.Status == "running" {
			return fmt.Sprintf("Bước %d/%d · %s", i+1, len(r.Plan), clean(st.Title, 60))
		}
	}
	if len(r.Tools) > 0 {
		return "Đang viết câu trả lời…"
	}
	return "Đang suy nghĩ…"
}
