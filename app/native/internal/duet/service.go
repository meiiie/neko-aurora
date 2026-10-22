package duet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Service struct {
	Store  *Store
	AI     *AI
	Push   *Pusher
	Mail   *Mailer
	mux    *http.ServeMux
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	work   map[string]*worker
	ctx    context.Context
	briefM sync.Mutex
	// Now is overridable in tests.
	Now func() time.Time
}

type worker struct{ running, again bool }

func New(ctx context.Context, stateDir string) (*Service, error) {
	st, err := OpenStore(stateDir)
	if err != nil {
		return nil, err
	}
	p, err := LoadPusher(stateDir)
	if err != nil {
		return nil, err
	}
	s := &Service{Store: st, AI: AIFromEnv(), Push: p, Mail: MailerFromEnv(), subs: map[chan Event]struct{}{}, work: map[string]*worker{}, ctx: ctx, Now: time.Now}
	s.routes()
	go s.briefLoop()
	return s, nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Service) publish(t string, d any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.subs {
		select {
		case c <- Event{t, d}:
		default: // slow client; it resyncs on reconnect
		}
	}
}

func (s *Service) logAct(agentID, text string) {
	s.publish("activity", s.Store.Log(agentID, text))
}

func (s *Service) status(id, st, snip string) {
	if a, err := s.Store.SetStatus(id, st, snip); err == nil {
		s.publish("agent", a)
	}
}

func (s *Service) notify(title, body, url string) {
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body, "url": url})
	s.publish("notify", json.RawMessage(payload))
	for _, sub := range s.Store.PushSubs() {
		go func(sub PushSub) {
			gone, err := s.Push.Send(sub, payload)
			if gone {
				s.Store.DropPush(sub.Endpoint)
			} else if err != nil {
				log.Printf("duet push failed: %v", err)
			}
		}(sub)
	}
}

// ---------- background agent tasks ----------

func (s *Service) kick(agentID string) {
	s.mu.Lock()
	w := s.work[agentID]
	if w == nil {
		w = &worker{}
		s.work[agentID] = w
	}
	if w.running {
		w.again = true // steer: new message picked up right after the current turn
		s.mu.Unlock()
		return
	}
	w.running = true
	s.mu.Unlock()
	go func() {
		for {
			s.runTurn(agentID)
			s.mu.Lock()
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

const systemPrompt = `Bạn là "%s", một agent trong Neko Duet làm việc cho Darren (HoLiLiHu). Vai trò: %s
Quy tắc:
- Trả lời bằng tiếng Việt, ngắn gọn, cụ thể, như một đồng nghiệp đáng tin. Không dùng markdown nặng.
- Bạn KHÔNG tự gửi, đăng, thanh toán hay liên hệ ai bên ngoài. Không bao giờ nói là đã gửi.
- Khi việc cần một hành động ra bên ngoài (gửi email/tin nhắn, đăng bài, thanh toán, mời họp), hãy soạn sẵn nội dung rồi kết thúc câu trả lời bằng đúng MỘT dòng cuối:
APPROVAL: {"title":"<việc cần duyệt>","kind":"email|send|post|pay|other","to":"<địa chỉ email nếu là email>","subject":"<tiêu đề nếu là email>","body":"<nội dung đã soạn>"}
- Với email: kind phải là "email", điền đúng địa chỉ người nhận Darren đưa, tiêu đề và toàn bộ nội dung thư trong body (không lặp lại thư ở phần trả lời phía trên; chỉ tóm tắt 1 câu).
- Nếu không cần duyệt thì không viết dòng APPROVAL.`

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

func (s *Service) runTurn(agentID string) {
	ag, err := s.Store.Agent(agentID)
	if err != nil {
		return
	}
	hist, _ := s.Store.Messages(agentID, 30)
	msgs := []ChatMsg{{Role: "system", Content: fmt.Sprintf(systemPrompt, ag.Name, ag.Desc)}}
	for _, m := range hist {
		switch {
		case m.Role == "user":
			msgs = append(msgs, ChatMsg{"user", m.Text})
		case m.Role == "agent":
			c := m.Text
			if m.Approval != nil {
				c += fmt.Sprintf("\n[Đề xuất cần duyệt: %s — trạng thái: %s]", m.Approval.Title, m.Approval.State)
			}
			msgs = append(msgs, ChatMsg{"assistant", c})
		case m.Role == "system":
			msgs = append(msgs, ChatMsg{"user", "[Hệ thống] " + m.Text})
		}
	}
	s.status(agentID, "working", "Đang làm việc")
	s.logAct(agentID, ag.Name+" bắt đầu xử lý yêu cầu")
	draft := newID("m_")
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
	defer cancel()
	full, err := s.AI.Stream(ctx, msgs, func(d string) {
		s.publish("delta", map[string]string{"agentId": agentID, "id": draft, "delta": d})
	})
	if err != nil {
		text := "Mình chưa xử lý được: " + err.Error()
		if errors.Is(err, ErrNoAI) {
			text = "Chưa cấu hình AI trên máy chủ (thiếu ZAI_API_KEY). Mình đã lưu yêu cầu, hãy nhắn lại sau khi cấu hình."
		}
		m := &Message{ID: draft, AgentID: agentID, Role: "system", Text: text}
		if s.Store.AddMessage(m) == nil {
			s.publish("message", m)
		}
		s.status(agentID, "error", text)
		s.logAct(agentID, ag.Name+": lỗi — "+err.Error())
		return
	}
	text, appr := SplitApproval(full)
	m := &Message{ID: draft, AgentID: agentID, Role: "agent", Text: text, Approval: appr}
	if err := s.Store.AddMessage(m); err != nil {
		return
	}
	s.publish("message", m)
	if appr != nil {
		s.status(agentID, "waiting", "Cần bạn duyệt: "+appr.Title)
		s.logAct(agentID, ag.Name+" soạn xong, chờ bạn duyệt “"+appr.Title+"”")
		s.notify(ag.Name+" cần bạn duyệt", appr.Title, "/duet/#"+agentID)
	} else {
		s.status(agentID, "done", text)
		s.logAct(agentID, ag.Name+" đã trả lời")
		s.notify(ag.Name, clean(text, 120), "/duet/#"+agentID)
	}
}

// ---------- morning brief ----------

func (s *Service) briefLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		now := s.Now()
		due := time.Date(now.Year(), now.Month(), now.Day(), 5, 30, 0, 0, now.Location())
		b := s.Store.Brief()
		if b == nil || (now.After(due) && b.Generated.Before(due)) {
			_, _ = s.GenerateBrief(s.ctx)
		}
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) GenerateBrief(ctx context.Context) (*Brief, error) {
	s.briefM.Lock()
	defer s.briefM.Unlock()
	agents := s.Store.Agents()
	pending := s.Store.Pending()
	names := map[string]string{}
	var sb strings.Builder
	for _, a := range agents {
		names[a.ID] = a.Name
		fmt.Fprintf(&sb, "- %s [%s]: %s\n", a.Name, a.Status, a.Snip)
	}
	sb.WriteString("Đang chờ duyệt:\n")
	items := []string{}
	for _, p := range pending {
		fmt.Fprintf(&sb, "- %s: %s\n", names[p.AgentID], p.Approval.Title)
		items = append(items, names[p.AgentID]+": "+p.Approval.Title)
	}
	sb.WriteString("Hoạt động gần đây:\n")
	for _, a := range s.Store.ActivityLog(15) {
		fmt.Fprintf(&sb, "- %s\n", a.Text)
	}
	b := &Brief{Generated: s.Now(), Items: items, Source: "fallback",
		Text: fmt.Sprintf("Chào Darren. %d agent đang hoạt động, %d việc cần bạn.", len(agents), len(pending))}
	if s.AI.Configured() {
		c, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		out, err := s.AI.Stream(c, []ChatMsg{
			{"system", `Bạn viết bản tin sáng cho Darren từ trạng thái các agent. Trả về DUY NHẤT JSON: {"text":"1-2 câu chào và tóm tắt","items":["tối đa 5 dòng ngắn, ưu tiên việc cần Darren duyệt"]}. Tiếng Việt. Không bịa dữ kiện ngoài dữ liệu.`},
			{"user", sb.String()},
		}, func(string) {})
		if err == nil {
			var j struct {
				Text  string   `json:"text"`
				Items []string `json:"items"`
			}
			out = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(out), "```json"), "```"))
			if i := strings.Index(out, "{"); i >= 0 {
				out = out[i:]
			}
			if json.Unmarshal([]byte(out), &j) == nil && j.Text != "" {
				b.Text, b.Source = clean(j.Text, 400), "ai"
				b.Items = []string{}
				for _, it := range j.Items {
					if it = clean(it, 160); it != "" && len(b.Items) < 5 {
						b.Items = append(b.Items, it)
					}
				}
			}
		} else {
			log.Printf("duet brief ai failed: %v", err)
		}
	}
	if err := s.Store.SetBrief(b); err != nil {
		return nil, err
	}
	s.publish("brief", b)
	s.logAct("", "Đã tạo bản tin sáng ("+b.Source+")")
	s.notify("Bản tin sáng", b.Text, "/duet/")
	return b, nil
}

// ---------- HTTP ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code := 500
	switch {
	case errors.Is(err, ErrNotFound):
		code = 404
	case errors.Is(err, ErrInvalid):
		code = 400
	case errors.Is(err, ErrConflict):
		code = 409
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<10)
	if json.NewDecoder(r.Body).Decode(v) != nil {
		return ErrInvalid
	}
	return nil
}

func (s *Service) routes() {
	m := http.NewServeMux()
	m.HandleFunc("GET /duet/api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"agents": s.Store.Agents(), "pending": s.Store.Pending(), "brief": s.Store.Brief(),
			"ai":             map[string]any{"configured": s.AI.Configured(), "model": s.AI.Model, "provider": "z.ai"},
			"vapidPublicKey": s.Push.Pub, "pushSubscriptions": len(s.Store.PushSubs()),
		})
	})
	m.HandleFunc("POST /duet/api/agents", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name   string `json:"name"`
			Desc   string `json:"desc"`
			Avatar Avatar `json:"av"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		a, err := s.Store.CreateAgent(in.Name, in.Desc, in.Avatar)
		if err != nil {
			fail(w, err)
			return
		}
		s.publish("agent", a)
		s.publish("activity", s.Store.ActivityLog(1)[0])
		writeJSON(w, 201, a)
	})
	m.HandleFunc("POST /duet/api/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		var p AgentPatch
		if err := decode(r, &p); err != nil {
			fail(w, err)
			return
		}
		a, err := s.Store.PatchAgent(r.PathValue("id"), p)
		if err != nil {
			fail(w, err)
			return
		}
		if a.Archived {
			s.publish("archived", map[string]string{"id": a.ID})
		} else {
			s.publish("agent", a)
		}
		writeJSON(w, 200, a)
	})
	m.HandleFunc("GET /duet/api/agents/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		ms, err := s.Store.Messages(r.PathValue("id"), 200)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, ms)
	})
	m.HandleFunc("POST /duet/api/agents/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text string `json:"text"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		id := r.PathValue("id")
		text := clean(in.Text, 4000)
		if text == "" {
			fail(w, ErrInvalid)
			return
		}
		msg := &Message{AgentID: id, Role: "user", Text: text}
		if err := s.Store.AddMessage(msg); err != nil {
			fail(w, err)
			return
		}
		s.publish("message", msg)
		a, _ := s.Store.Agent(id)
		s.logAct(id, "Bạn giao cho "+a.Name+": "+clean(text, 80))
		s.kick(id)
		writeJSON(w, 202, msg)
	})
	m.HandleFunc("POST /duet/api/approvals/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Decision string `json:"decision"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		msg, err := s.Store.Decide(r.PathValue("id"), in.Decision)
		if err != nil {
			fail(w, err)
			return
		}
		s.publish("message", msg)
		a, _ := s.Store.Agent(msg.AgentID)
		var follow string
		if in.Decision == "approved" && msg.Approval.Kind == "email" && msg.Approval.To != "" {
			follow = s.deliverEmail(msg)
			s.status(msg.AgentID, "done", "Đã duyệt: "+msg.Approval.Title)
			s.logAct(msg.AgentID, "Bạn duyệt email “"+msg.Approval.Title+"” cho "+a.Name)
		} else if in.Decision == "approved" {
			follow = "Đã ghi nhận bạn duyệt “" + msg.Approval.Title + "”. Kênh gửi ra ngoài chưa được kết nối nên mình chưa gửi gì; bản đã duyệt được lưu để gửi khi kênh sẵn sàng."
			s.status(msg.AgentID, "done", "Đã duyệt: "+msg.Approval.Title)
			s.logAct(msg.AgentID, "Bạn duyệt “"+msg.Approval.Title+"” cho "+a.Name)
		} else {
			follow = "Ok, mình giữ lại “" + msg.Approval.Title + "” và không làm gì thêm."
			s.status(msg.AgentID, "idle", "Đã hoãn: "+msg.Approval.Title)
			s.logAct(msg.AgentID, "Bạn hoãn “"+msg.Approval.Title+"” của "+a.Name)
		}
		f := &Message{AgentID: msg.AgentID, Role: "agent", Text: follow}
		if s.Store.AddMessage(f) == nil {
			s.publish("message", f)
		}
		writeJSON(w, 200, msg)
	})
	m.HandleFunc("GET /duet/api/outbox", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"email": map[string]any{"configured": s.Mail.Configured(), "from": s.Mail.From}, "items": s.Store.Outbox()})
	})
	m.HandleFunc("GET /duet/api/activity", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if n <= 0 || n > 200 {
			n = 60
		}
		writeJSON(w, 200, s.Store.ActivityLog(n))
	})
	m.HandleFunc("POST /duet/api/brief", func(w http.ResponseWriter, r *http.Request) {
		b, err := s.GenerateBrief(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, b)
	})
	m.HandleFunc("POST /duet/api/push/subscribe", func(w http.ResponseWriter, r *http.Request) {
		var p PushSub
		if err := decode(r, &p); err != nil {
			fail(w, err)
			return
		}
		if err := s.Store.AddPush(p); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("POST /duet/api/push/test", func(w http.ResponseWriter, r *http.Request) {
		s.notify("Neko Duet", "Thông báo thử: kênh thông báo đang hoạt động.", "/duet/")
		writeJSON(w, 200, map[string]int{"subscriptions": len(s.Store.PushSubs())})
	})
	m.HandleFunc("GET /duet/api/events", s.events)
	s.mux = m
}

func (s *Service) events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{}) // long-lived stream; server ReadTimeout must not cut it
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	c := make(chan Event, 256)
	s.mu.Lock()
	s.subs[c] = struct{}{}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.subs, c); s.mu.Unlock() }()
	fmt.Fprint(w, "retry: 2000\n: hello\n\n")
	_ = rc.Flush()
	hb := time.NewTicker(15 * time.Second)
	defer hb.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case <-hb.C:
			fmt.Fprint(w, ": hb\n\n")
		case e := <-c:
			b, _ := json.Marshal(e.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b)
		}
		if rc.Flush() != nil {
			return
		}
	}
}

// deliverEmail runs only after an owner approval was recorded. The recipient
// must be an address the owner typed in this thread, so model output alone
// can never choose who receives mail.
func (s *Service) deliverEmail(msg *Message) string {
	ap := msg.Approval
	if !s.Store.UserMentioned(msg.AgentID, ap.To) {
		m, _ := s.Store.SetDelivery(msg.ID, Delivery{Status: "failed", Detail: "recipient_not_from_owner"})
		if m != nil {
			s.publish("message", m)
		}
		return "Mình không gửi: địa chỉ " + ap.To + " không phải do bạn nhập trong hội thoại này."
	}
	if !s.Mail.Configured() {
		m, _ := s.Store.SetDelivery(msg.ID, Delivery{Status: "queued", Detail: "email_channel_not_configured"})
		if m != nil {
			s.publish("message", m)
		}
		return "Đã ghi nhận bạn duyệt. Kênh email của máy chủ chưa được cấu hình (DUET_SMTP_*), nên thư tới " + ap.To + " đang nằm trong hàng đợi, chưa gửi."
	}
	m, _ := s.Store.SetDelivery(msg.ID, Delivery{Status: "sending", From: s.Mail.From})
	if m != nil {
		s.publish("message", m)
	}
	go func() {
		subj := ap.Subject
		if subj == "" {
			subj = ap.Title
		}
		err := s.Mail.Send(ap.To, subj, ap.Body)
		d := Delivery{Status: "sent", From: s.Mail.From}
		text := "Đã gửi thư tới " + ap.To + " từ " + s.Mail.From + "."
		if err != nil {
			d = Delivery{Status: "failed", From: s.Mail.From, Detail: clean(err.Error(), 160)}
			text = "Gửi thư tới " + ap.To + " thất bại: " + d.Detail
		}
		if m, _ := s.Store.SetDelivery(msg.ID, d); m != nil {
			s.publish("message", m)
		}
		f := &Message{AgentID: msg.AgentID, Role: "agent", Text: text}
		if s.Store.AddMessage(f) == nil {
			s.publish("message", f)
		}
		s.logAct(msg.AgentID, text)
		s.notify("Email", text, "/duet/#"+msg.AgentID)
	}()
	return "Đã ghi nhận bạn duyệt. Đang gửi thư tới " + ap.To + "…"
}
