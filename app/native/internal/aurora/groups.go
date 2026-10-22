package aurora

import (
	"sort"
	"strings"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

// Group chats are ledger channels. One owner message becomes one ledger work
// item per member bot, all in the channel's conversation (the same shape the
// desktop uses for group sends), so every bot answers in the same thread and
// each one sees what the others said.

func isGroupID(id string) bool { return strings.HasPrefix(id, "channel-") }

// thread returns the work items behind a bot or group thread.
func (v *view) thread(id string) []ledger.Work {
	if isGroupID(id) {
		return v.byConv[id]
	}
	return v.byBot[id]
}

func groupBase(nonce string) string {
	if i := strings.LastIndex(nonce, "_g"); i > 0 {
		return nonce[:i]
	}
	return nonce
}

func (v *view) groupMessages(id string, limit int) []*Message {
	out := []*Message{}
	seen := map[string]bool{}
	running := map[string]bool{}
	for _, w := range v.byConv[id] {
		if w.Status == "running" || w.Status == "starting" {
			running[w.BotID] = true
		}
	}
	for _, w := range v.byConv[id] {
		for _, m := range workMessages(w, running[w.BotID]) {
			if m.Role == "user" {
				base := groupBase(w.ClientNonce)
				if seen[base] {
					continue
				}
				seen[base] = true
				m.ID = base + ":u"
			} else {
				m.From = w.BotID
			}
			m.AgentID = id
			out = append(out, m)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func (v *view) group(c ledger.Channel, pinned bool) *Agent {
	members := []string{}
	names := []string{}
	for _, b := range c.BotIDs {
		if bb, ok := v.bot(b); ok && !v.archived[b] {
			members = append(members, b)
			names = append(names, displayName(bb))
		}
	}
	a := &Agent{ID: c.ID, Name: c.Name, Desc: strings.Join(names, ", "), Kind: "group", Members: members, Status: "idle", Snip: "Nhóm với " + strings.Join(names, ", "), Pinned: pinned}
	l := v.byConv[c.ID]
	if len(l) == 0 {
		return a
	}
	last := l[len(l)-1]
	a.Updated = ts(last.CreatedAt)
	for _, w := range l {
		if active(w) {
			a.Status, a.Snip = "working", "Đang trả lời"
		}
		if w.Direct != nil && w.Direct.Approval != nil && w.Direct.Approval.State == "pending" && a.Status != "working" {
			a.Status, a.Snip = "waiting", "Cần bạn duyệt: "+w.Direct.Approval.Title
		}
		if w.Direct != nil && w.Direct.FinishedAt != "" {
			if t := ts(w.Direct.FinishedAt); t.After(a.Updated) {
				a.Updated = t
			}
		}
	}
	if a.Status == "idle" {
		for i := len(l) - 1; i >= 0; i-- {
			if r := firstLine(visibleText(l[i].Reply)); r != "" {
				b, _ := v.bot(l[i].BotID)
				a.Status, a.Snip = "done", clean(displayName(b)+": "+r, 140)
				break
			}
		}
	}
	return a
}

func (v *view) groups() []ledger.Channel {
	out := []ledger.Channel{}
	for _, c := range v.channels {
		if c.HumanACL == nil {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// createGroup saves a ledger channel with the chosen bots.
func (s *Service) createGroup(name string, bots []string) (ledger.Channel, error) {
	v := s.view()
	uniq := map[string]bool{}
	ids := []string{}
	for _, b := range bots {
		if _, ok := v.bot(b); ok && !v.archived[b] && !uniq[b] {
			uniq[b] = true
			ids = append(ids, b)
		}
	}
	if len(ids) < 2 || len(ids) > 8 {
		return ledger.Channel{}, ErrInvalid
	}
	if name = clean(name, 40); name == "" {
		names := []string{}
		for _, b := range ids {
			bb, _ := v.bot(b)
			names = append(names, displayName(bb))
		}
		name = clean(strings.Join(names, ", "), 40)
	}
	c, _, err := s.L.SaveChannel(ledger.ChannelInput{Name: name, BotIDs: ids, ClientNonce: newID("grp-")})
	return c, err
}

// sendGroup fans one owner message out to every member bot.
func (s *Service) sendGroup(id, text, nonce string) (*Message, error) {
	v := s.view()
	c, ok := v.channels[id]
	if !ok {
		return nil, ErrNotFound
	}
	s.supersede(id)
	rev := c.Revision
	var first ledger.Work
	n := 0
	for i, b := range c.BotIDs {
		if v.archived[b] {
			continue
		}
		w, _, err := s.L.AcceptProduct(ledger.Input{BotID: b, ClientNonce: nonce + "_g" + itoa(i+1), Text: text}, ledger.SubmissionScope{ChannelID: id, ExpectedChannelRevision: &rev})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			first = w
		}
		n++
	}
	if n == 0 {
		return nil, ErrInvalid
	}
	return &Message{ID: nonce + ":u", TaskID: first.RequestID, AgentID: id, Role: "user", Text: text, Created: time.Now()}, nil
}

func threadOf(w ledger.Work) string {
	if isGroupID(w.ConversationID) {
		return w.ConversationID
	}
	return w.BotID
}

// groupHistory gives a bot the shared group thread: the owner's messages
// once, its own replies as its turns, and the other bots' replies labelled.
func (s *Service) groupHistory(v *view, b ledger.Bot, cur ledger.Work, msgs []ChatMsg) []ChatMsg {
	c := v.channels[cur.ConversationID]
	names := []string{}
	for _, id := range c.BotIDs {
		if id != b.ID {
			if o, ok := v.bot(id); ok {
				names = append(names, displayName(o))
			}
		}
	}
	msgs[0].Content += "\nBạn đang ở nhóm “" + c.Name + "” cùng " + strings.Join(names, ", ") + ". Trả lời phần việc của bạn, ngắn gọn, không lặp lại điều người khác đã nói."
	seen := map[string]bool{}
	l := v.byConv[c.ID]
	if len(l) > 60 {
		l = l[len(l)-60:]
	}
	for _, w := range l {
		if base := groupBase(w.ClientNonce); !seen[base] {
			seen[base] = true
			msgs = append(msgs, ChatMsg{"user", w.Text})
		}
		if w.RequestID == cur.RequestID {
			continue
		}
		r := visibleText(w.Reply)
		if r == "" {
			continue
		}
		if w.BotID == b.ID {
			msgs = append(msgs, ChatMsg{"assistant", r})
		} else {
			o, _ := v.bot(w.BotID)
			msgs = append(msgs, ChatMsg{"user", "[" + displayName(o) + " trong nhóm]: " + r})
		}
	}
	// the current turn's owner message must be last
	for i := len(msgs) - 1; i > 0; i-- {
		if msgs[i].Role == "user" && msgs[i].Content == cur.Text {
			m := msgs[i]
			msgs = append(append(msgs[:i:i], msgs[i+1:]...), m)
			break
		}
	}
	return msgs
}
