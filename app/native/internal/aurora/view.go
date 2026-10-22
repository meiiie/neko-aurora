package aurora

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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

// ---- views: the mobile JSON shapes, projected from ledger bots and work ----

type Agent struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Desc    string    `json:"desc"`
	Tag     string    `json:"tag,omitempty"`
	Face    *Face     `json:"av,omitempty"`
	Status  string    `json:"status"` // idle|working|waiting|done|error
	Task    string    `json:"task"`   // OpenMuse-style TaskStatus of the latest task
	Snip    string    `json:"snip"`
	Pinned  bool      `json:"pin"`
	Kind    string    `json:"kind,omitempty"` // ""|group
	Members []string  `json:"members,omitempty"`
	Updated time.Time `json:"t"`
}

type Step = ledger.DirectStep
type Plan struct {
	Steps []Step `json:"steps"`
}
type Approval = ledger.DirectApproval

type Message struct {
	ID       string                `json:"id"`
	TaskID   string                `json:"taskId"`
	AgentID  string                `json:"agentId"`
	Role     string                `json:"role"` // user|agent|system
	Text     string                `json:"text"`
	Approval *Approval             `json:"approval,omitempty"`
	Plan     *Plan                 `json:"plan,omitempty"`
	Choices  []string              `json:"choices,omitempty"`
	Secret   *ledger.DirectSecret  `json:"secret,omitempty"`
	Tools    []ledger.DirectTool   `json:"tools,omitempty"`
	Sources  []ledger.DirectSource `json:"sources,omitempty"`
	Queued   bool                  `json:"queued,omitempty"`
	From     string                `json:"from,omitempty"` // speaking bot, in group threads
	Created  time.Time             `json:"t"`
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
	case w.Direct != nil && len(w.Direct.Choices) > 0 && w.Status == "completed":
		return TaskWaitingInput
	case w.Status == "waiting_host":
		return TaskQueued
	case w.Status == "running" || w.Status == "starting" || w.Status == "cancelling":
		return TaskRunning
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
	a := &Agent{ID: b.ID, Name: displayName(b), Desc: v.desc(b), Tag: prefs.Tag, Pinned: prefs.Pinned, Status: "idle", Snip: "Sẵn sàng nhận việc.", Task: ""}
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
		}
	}
	for _, w := range l {
		if active(w) {
			a.Status, a.Snip = "working", "Đang làm việc"
			if w.Status == "waiting_host" {
				a.Snip = "Đang chờ lượt"
			}
			if w.Direct != nil {
				for i, st := range w.Direct.Plan {
					if st.Status == "running" {
						a.Snip = "Bước " + itoa(i+1) + "/" + itoa(len(w.Direct.Plan)) + ": " + st.Title
					}
				}
			}
			return a
		}
	}
	if a.Status == "waiting" {
		return a
	}
	switch taskStatus(last) {
	case TaskWaitingInput:
		a.Status, a.Snip = "waiting", "Cần bạn chọn để tiếp tục"
	case TaskSucceeded:
		a.Status, a.Snip = "done", clean(firstLine(visibleText(last.Reply)), 140)
		if a.Snip == "" {
			a.Snip = "Đã xong"
		}
	case TaskCancelled:
		a.Status, a.Snip = "idle", "Đã dừng theo yêu cầu của bạn."
	case TaskFailed:
		a.Status, a.Snip = "error", failureText(last.Failure)
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
	case code == "ai_not_configured":
		return "Chưa cấu hình AI trên máy chủ (thiếu ZAI_API_KEY). Yêu cầu đã được lưu, hãy nhắn lại sau khi cấu hình."
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
	for _, w := range l {
		out = append(out, workMessages(w, running)...)
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
	if kind == "" {
		out = append(out, &Message{ID: w.RequestID + ":u", TaskID: w.RequestID, AgentID: w.BotID, Role: "user", Text: w.Text, Queued: w.Status == "waiting_host" && botRunning, Created: at})
	}
	if d == nil {
		if w.Status == "cancelled" {
			out = append(out, &Message{ID: w.RequestID, TaskID: w.RequestID, AgentID: w.BotID, Role: "system", Text: failureText("stopped_by_owner"), Created: at})
		} else if w.Status == "completed" && w.Reply != "" {
			out = append(out, &Message{ID: w.RequestID, TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: w.Reply, Created: at})
		}
		return out
	}
	if d.Ack != "" {
		out = append(out, &Message{ID: w.RequestID + ":ack", TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: d.Ack, Created: at})
	}
	if len(d.Plan) > 0 {
		out = append(out, &Message{ID: w.RequestID + ":plan", TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: d.PlanIntro, Plan: &Plan{Steps: append([]Step(nil), d.Plan...)}, Created: at})
	}
	reply := visibleText(w.Reply)
	if reply != "" || d.Approval != nil || len(d.Choices) > 0 || d.Secret != nil || len(d.Tools) > 0 {
		m := &Message{ID: w.RequestID, TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: reply, Choices: append([]string(nil), d.Choices...), Created: at}
		if d.Approval != nil {
			a := *d.Approval
			m.Approval = &a
		}
		if d.Secret != nil {
			x := *d.Secret
			m.Secret = &x
		}
		m.Tools = append([]ledger.DirectTool(nil), d.Tools...)
		m.Sources = append([]ledger.DirectSource(nil), d.Sources...)
		out = append(out, m)
	}
	if d.Approval != nil && d.Approval.Outcome != "" {
		out = append(out, &Message{ID: w.RequestID + ":outcome", TaskID: w.RequestID, AgentID: w.BotID, Role: "agent", Text: d.Approval.Outcome, Created: ts(d.Approval.DecidedAt)})
	}
	if w.Status == "failed" || w.Status == "cancelled" {
		out = append(out, &Message{ID: w.RequestID + ":end", TaskID: w.RequestID, AgentID: w.BotID, Role: "system", Text: failureText(w.Failure), Created: at})
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
