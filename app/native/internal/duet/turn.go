package duet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Agent turns. Patterns studied (not copied) from CopilotKit OpenMuse/OpenDots
// (MIT): durable step plans with live status, tool-rendered cards in the
// thread, human-in-the-loop review before effects, stop + visible follow-up
// queue. Implementation here is original Go.

const systemPrompt = `Bạn là "%s", một agent trong Neko Duet làm việc cho Darren (HoLiLiHu). Vai trò: %s
Quy tắc:
- Trả lời bằng tiếng Việt, ngắn gọn, cụ thể, như một đồng nghiệp đáng tin. Không dùng markdown nặng.
- Bạn KHÔNG tự gửi, đăng, thanh toán hay liên hệ ai bên ngoài. Không bao giờ nói là đã gửi.
- Cuối câu trả lời, bạn có thể thêm các dòng chỉ thị (mỗi loại tối đa một dòng, đúng JSON):
PLAN: ["bước 1","bước 2"]   ← chỉ khi việc cần 2-4 bước rõ ràng; khi đó phần trả lời chỉ cần 1 câu nêu cách làm.
CHOICES: ["lựa chọn A","lựa chọn B"]   ← khi cần Darren chọn nhanh (2-4 lựa chọn ngắn).
APPROVAL: {"title":"<việc cần duyệt>","kind":"email|send|post|pay|other","to":"<email nếu là email>","subject":"<tiêu đề nếu là email>","body":"<toàn bộ nội dung đã soạn>"}   ← bắt buộc cho mọi hành động ra bên ngoài.
- Với email: kind là "email", dùng đúng địa chỉ Darren đưa; không lặp lại toàn văn thư ở phần trả lời.`

var directiveRe = regexp.MustCompile(`(?m)^\s*(PLAN|CHOICES|APPROVAL):\s*(.+?)\s*$`)

type directives struct {
	Text     string
	Plan     []string
	Choices  []string
	Approval *Approval
}

// ParseDirectives strips trailing PLAN/CHOICES/APPROVAL lines from model text.
func ParseDirectives(text string) directives {
	var d directives
	// APPROVAL bodies may span lines; handle it first with the dedicated parser.
	text, d.Approval = SplitApproval(text)
	for _, m := range directiveRe.FindAllStringSubmatch(text, -1) {
		var list []string
		if json.Unmarshal([]byte(m[2]), &list) != nil {
			continue
		}
		clean2 := []string{}
		for _, v := range list {
			if v = clean(v, 120); v != "" && len(clean2) < 4 {
				clean2 = append(clean2, v)
			}
		}
		switch m[1] {
		case "PLAN":
			if len(clean2) >= 2 {
				d.Plan = clean2
			}
		case "CHOICES":
			if len(clean2) >= 2 {
				d.Choices = clean2
			}
		}
	}
	d.Text = strings.TrimSpace(directiveRe.ReplaceAllString(text, ""))
	return d
}

var approvalRe = regexp.MustCompile(`(?s)\n?\s*APPROVAL:\s*(\{.*\})\s*$`)

func SplitApproval(text string) (string, *Approval) {
	m := approvalRe.FindStringSubmatchIndex(text)
	if m == nil {
		return strings.TrimSpace(text), nil
	}
	var a Approval
	if json.Unmarshal([]byte(text[m[2]:m[3]]), &a) != nil || strings.TrimSpace(a.Body) == "" {
		return strings.TrimSpace(text), nil
	}
	a.Title, a.Body, a.Kind = clean(a.Title, 80), clean(a.Body, 4000), clean(a.Kind, 12)
	a.Subject = clean(a.Subject, 160)
	if a.Kind == "email" {
		if to, ok := ValidEmail(a.To); ok {
			a.To = to
		} else {
			a.To = ""
		}
	} else {
		a.To, a.Subject = "", ""
	}
	if a.Title == "" {
		a.Title = "Hành động cần bạn duyệt"
	}
	a.State = "pending"
	return strings.TrimSpace(text[:m[0]]), &a
}

func (s *Service) kick(agentID string) {
	s.mu.Lock()
	w := s.work[agentID]
	if w == nil {
		w = &worker{}
		s.work[agentID] = w
	}
	if w.running {
		w.again = true // follow-up queue: runs right after the current turn
		s.mu.Unlock()
		return
	}
	w.running = true
	s.mu.Unlock()
	go func() {
		for {
			ctx, cancel := context.WithTimeout(s.ctx, 6*time.Minute)
			s.mu.Lock()
			w.cancel = cancel
			s.mu.Unlock()
			s.runTurn(ctx, agentID)
			cancel()
			s.mu.Lock()
			w.cancel = nil
			if !w.again {
				w.running = false
				s.mu.Unlock()
				return
			}
			w.again = false
			s.mu.Unlock()
		}
	}()
}

func (s *Service) isRunning(agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.work[agentID]
	return w != nil && w.running
}

// Stop cancels the running turn and holds the follow-up queue.
func (s *Service) Stop(agentID string) bool {
	s.mu.Lock()
	w := s.work[agentID]
	if w == nil || !w.running || w.cancel == nil {
		s.mu.Unlock()
		return false
	}
	w.again = false
	w.cancel()
	s.mu.Unlock()
	return true
}

func (s *Service) history(ag *Agent) []ChatMsg {
	hist, _ := s.Store.Messages(ag.ID, 30)
	msgs := []ChatMsg{{Role: "system", Content: fmt.Sprintf(systemPrompt, ag.Name, ag.Desc)}}
	for _, m := range hist {
		switch m.Role {
		case "user":
			msgs = append(msgs, ChatMsg{"user", m.Text})
		case "agent":
			c := m.Text
			if m.Plan != nil {
				for i, st := range m.Plan.Steps {
					c += fmt.Sprintf("\n[Bước %d: %s — %s]", i+1, st.Title, st.Status)
				}
			}
			if m.Approval != nil {
				c += fmt.Sprintf("\n[Đề xuất cần duyệt: %s — trạng thái: %s]", m.Approval.Title, m.Approval.State)
			}
			if c != "" {
				msgs = append(msgs, ChatMsg{"assistant", c})
			}
		case "system":
			msgs = append(msgs, ChatMsg{"user", "[Hệ thống] " + m.Text})
		}
	}
	return msgs
}

// call streams one model call into a draft message id; returns parsed directives.
func (s *Service) call(ctx context.Context, agentID string, msgs []ChatMsg) (string, directives, error) {
	draft := newID("m_")
	full, err := s.AI.Stream(ctx, msgs, func(d string) {
		s.publish("delta", map[string]string{"agentId": agentID, "id": draft, "delta": d})
	})
	return draft, ParseDirectives(full), err
}

func (s *Service) fail(agentID, draft string, ag *Agent, err error) {
	text := "Mình chưa xử lý được: " + err.Error()
	st := "error"
	switch {
	case errors.Is(err, ErrNoAI):
		text = "Chưa cấu hình AI trên máy chủ (thiếu ZAI_API_KEY). Mình đã lưu yêu cầu, hãy nhắn lại sau khi cấu hình."
	case errors.Is(err, context.Canceled):
		text, st = "Đã dừng theo yêu cầu của bạn.", "idle"
	}
	m := &Message{ID: draft, AgentID: agentID, Role: "system", Text: text}
	if s.Store.AddMessage(m) == nil {
		s.publish("message", m)
	}
	s.status(agentID, st, text)
	s.logAct(agentID, ag.Name+": "+text)
}

func (s *Service) setStep(planID string, i int, status string) {
	if m, err := s.Store.UpdateMessage(planID, func(m *Message) error {
		if m.Plan == nil || i >= len(m.Plan.Steps) {
			return ErrInvalid
		}
		m.Plan.Steps[i].Status = status
		if status == "cancelled" || status == "failed" {
			for j := i + 1; j < len(m.Plan.Steps); j++ {
				m.Plan.Steps[j].Status = "cancelled"
			}
		}
		return nil
	}); err == nil {
		s.publish("message", m)
	}
}

func (s *Service) finish(agentID string, ag *Agent, draft string, d directives) *Approval {
	m := &Message{ID: draft, AgentID: agentID, Role: "agent", Text: d.Text, Approval: d.Approval, Choices: d.Choices}
	if s.Store.AddMessage(m) != nil {
		return nil
	}
	s.publish("message", m)
	return d.Approval
}

func (s *Service) runTurn(ctx context.Context, agentID string) {
	ag, err := s.Store.Agent(agentID)
	if err != nil {
		return
	}
	for _, q := range s.Store.TakeQueued(agentID) {
		s.publish("message", q)
	}
	s.status(agentID, "working", "Đang làm việc")
	s.logAct(agentID, ag.Name+" bắt đầu xử lý yêu cầu")
	draft, d, err := s.call(ctx, agentID, s.history(ag))
	if err != nil {
		s.fail(agentID, draft, ag, err)
		return
	}
	var appr *Approval
	if len(d.Plan) >= 2 && d.Approval == nil {
		plan := &Message{ID: draft, AgentID: agentID, Role: "agent", Text: d.Text, Plan: &Plan{}}
		for _, t := range d.Plan {
			plan.Plan.Steps = append(plan.Plan.Steps, Step{Title: t, Status: "pending"})
		}
		if s.Store.AddMessage(plan) != nil {
			return
		}
		s.publish("message", plan)
		s.logAct(agentID, fmt.Sprintf("%s lập kế hoạch %d bước", ag.Name, len(d.Plan)))
		for i, title := range d.Plan {
			if ctx.Err() != nil {
				s.setStep(plan.ID, i, "cancelled")
				s.fail(agentID, newID("m_"), ag, context.Canceled)
				return
			}
			s.setStep(plan.ID, i, "running")
			s.status(agentID, "working", fmt.Sprintf("Bước %d/%d: %s", i+1, len(d.Plan), title))
			msgs := append(s.history(ag), ChatMsg{"user", fmt.Sprintf("[Hệ thống] Thực hiện bước %d/%d: %s. Chỉ làm bước này, trả lời ngắn. Không viết PLAN. Đừng tự tạo bước chờ Darren duyệt: việc duyệt do hệ thống lo. Nếu thật sự thiếu thông tin chỉ Darren biết, hỏi 1 câu và dùng CHOICES, kế hoạch sẽ tạm dừng. Chỉ dùng APPROVAL nếu bước này cần hành động ra bên ngoài.", i+1, len(d.Plan), title)})
			sd, sdir, err := s.call(ctx, agentID, msgs)
			if err != nil {
				st := "failed"
				if errors.Is(err, context.Canceled) {
					st = "cancelled"
				}
				s.setStep(plan.ID, i, st)
				s.fail(agentID, sd, ag, err)
				return
			}
			sdir.Plan = nil
			hasChoices := len(sdir.Choices) > 0
			if a := s.finish(agentID, ag, sd, sdir); a != nil || hasChoices {
				s.setStep(plan.ID, i, "waiting")
				for j := i + 1; j < len(d.Plan); j++ {
					s.setStep(plan.ID, j, "cancelled")
				}
				if a == nil {
					s.status(agentID, "waiting", "Cần bạn chọn để tiếp tục")
					s.notify(ag.Name+" cần bạn chọn", title, "/duet/#"+agentID)
					return
				}
				appr = a
				break
			}
			s.setStep(plan.ID, i, "done")
		}
	} else {
		appr = s.finish(agentID, ag, draft, d)
	}
	if appr != nil {
		s.status(agentID, "waiting", "Cần bạn duyệt: "+appr.Title)
		s.logAct(agentID, ag.Name+" soạn xong, chờ bạn duyệt “"+appr.Title+"”")
		s.notify(ag.Name+" cần bạn duyệt", appr.Title, "/duet/#"+agentID)
		return
	}
	s.status(agentID, "done", "Đã xong")
	s.logAct(agentID, ag.Name+" đã xong")
	s.notify(ag.Name, "Đã xong việc bạn giao.", "/duet/#"+agentID)
}
