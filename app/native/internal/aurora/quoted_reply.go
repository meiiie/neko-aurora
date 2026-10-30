package aurora

import "nekoaurora.local/native/internal/ledger"

func (s *Service) quoteOf(v *view, thread, id string) *ledger.MessageQuote {
	if id == "" {
		return nil
	}
	ms := v.messages(thread, 0)
	if isGroupID(thread) {
		ms = v.groupMessages(thread, 0)
	}
	for _, m := range s.ownerDecorateMessages(s.networkDecorateMessages(ms)) {
		if m.ID != id || m.Text == "" || m.Role != "user" && m.Role != "agent" {
			continue
		}
		name := "Bạn"
		if m.Role == "agent" {
			name = s.botName(first(m.From, m.AgentID))
		}
		return &ledger.MessageQuote{ID: m.ID, TaskID: m.TaskID, Role: m.Role, Author: clean(name, 80), Text: clean(m.Text, 2000)}
	}
	return nil
}

func (s *Service) ownerDecorateMessages(ms []*Message) []*Message {
	for _, m := range ms {
		if m.Role != "user" || m.TaskID == "" {
			continue
		}
		if r, ok := s.L.OwnerMessageForWork(m.TaskID); ok && r.Input.ThreadID == m.AgentID {
			if r.Quote != nil {
				m.Text, m.Ref, m.Quote = r.Input.Text, refName(r.ReferenceText), r.Quote
			}
			if r.Input.Form != nil {
				m.FormFor = r.Input.Form.TaskID
				m.FormValues = append([]string(nil), r.Input.Form.Values...)
				m.Text = r.Input.Text
			}
		}
	}
	return ms
}
