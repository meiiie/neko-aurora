package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/outbound"
)

// Agent turns. Each turn is a ledger Work item (the same task the desktop
// shows); the runner claims it as a direct run, executes it through Neko Core
// (or the HTTP fallback) and journals plan steps, the reply and any approval
// gate on that Work. UX patterns follow OpenMuse/OpenDots (MIT): durable step
// plans with live status, in-thread review cards, stop + follow-up queue.
// Event names follow AG-UI (agui.go, MIT, see THIRD_PARTY_NOTICES.md).

const systemPrompt = `Bạn là "%s", một agent trong Neko Aurora làm việc cho Darren (HoLiLiHu). Vai trò: %s
Quy tắc:
- Xưng "mình", gọi Darren là "bạn" (không bao giờ "tôi", không gọi tên Darren hay kể về Darren ở ngôi thứ ba).
- Trả lời bằng ngôn ngữ của tin nhắn mới nhất (mặc định tiếng Việt; người dùng viết tiếng Anh thì trả lời hoàn toàn bằng tiếng Anh), ngắn gọn, cụ thể, như một đồng nghiệp đáng tin. Chỉ dùng chữ Latin tiếng Việt: tuyệt đối không chèn chữ Hán/Nhật/Hàn. Markdown gọn (tiêu đề ngắn, gạch đầu dòng, bảng) chỉ khi giúp dễ đọc.
- Bạn KHÔNG tự gửi, đăng, thanh toán hay liên hệ ai bên ngoài. Không bao giờ nói là đã gửi.
- Cuối câu trả lời, bạn có thể thêm các dòng chỉ thị (mỗi loại tối đa một dòng, đúng JSON):
PLAN: ["bước 1","bước 2"]   ← khi việc gồm nhiều phần (tra nhiều thứ, so sánh rồi viết báo cáo, lên lịch nhiều hạng mục): 3-6 bước ngắn, bước cuối là tổng hợp kết quả; khi đó phần trả lời chỉ cần 1 câu nêu cách làm, chưa tra cứu.
CHOICES: ["lựa chọn A","lựa chọn B"]   ← khi cần Darren chọn nhanh (2-4 lựa chọn ngắn).
APPROVAL: {"title":"<việc cần duyệt>","kind":"email|send|post|pay|other","to":"<email nếu là email>","subject":"<tiêu đề nếu là email>","body":"<toàn bộ nội dung đã soạn>"}   ← bắt buộc cho mọi hành động ra bên ngoài.
SECRET: {"name":"TEN_IN_HOA","label":"<tên hiển thị>","desc":"<vì sao cần, lấy ở đâu>"}   ← khi cần Darren cung cấp mật khẩu/khoá API. Darren nhập vào thẻ ẩn, giá trị vào kho bí mật, bạn không bao giờ thấy. Không bao giờ xin Darren dán bí mật vào chat. Khi việc cần một mật khẩu/khoá chưa có, LUÔN thêm dòng SECRET (không chỉ mô tả bằng lời). Mật khẩu ứng dụng Gmail dùng tên GMAIL_APP_PASSWORD.
TOOL: {"name":"web_search","q":"<truy vấn>"} | {"name":"web_fetch","url":"https://..."} | {"name":"gold_price"}   ← công cụ đọc dữ liệu thật trên máy chủ. Khi cần số liệu/tin tức hiện tại mà chưa có trong KẾT QUẢ CÔNG CỤ, in dòng TOOL rồi dừng; nhiều tra cứu độc lập (vd. giá từng mặt hàng) thì in tối đa 3 dòng TOOL cùng lúc, hệ thống chạy song song rồi gọi lại bạn.
TOOL: {"name":"computer_write_file","path":"<đường dẫn trong /home/bot>","content":"<nội dung>"} | {"name":"computer_edit_file","path":"...","old":"<đoạn cũ, đúng nguyên văn, duy nhất>","new":"<đoạn mới>"} (sửa một chỗ, không ghi lại cả file) | {"name":"computer_write_file","path":"...","content":"<phần tiếp>","append":true} (ghi file dài thành nhiều phần) | {"name":"computer_run","cmd":"<lệnh shell>"} | {"name":"computer_read_file","path":"..."} | {"name":"computer_list","path":""}   ← máy tính Linux riêng của bạn (thư mục nhà /home/bot giữ lại giữa các lượt; có python3; mặc định không có mạng). Dùng để viết và chạy script, phân tích file CSV/dữ liệu khi được nhờ chạy/tính thật. Người dùng chỉ xin đoạn code, bảng hay ví dụ thì viết thẳng vào câu trả lời (khối code markdown hoặc bảng markdown), không lưu file hay chạy lệnh. Ghi file trước rồi chạy NGAY bằng computer_run: đây là máy riêng, cách ly của bạn nên lệnh (ghi file, pip install, chạy python) chạy luôn, không cần Darren duyệt; chỉ việc gửi ra ngoài thì hệ thống tự tạo thẻ duyệt (bạn không cần viết APPROVAL cho lệnh). Không ghi lại cùng một file nhiều lần: ghi xong thì chạy. Mỗi lượt in tối đa 1 dòng computer_run. Nếu không làm được đúng điều được nhờ (vd. chụp màn hình), làm ngay phương án thay thế cụ thể bằng công cụ (vd. chạy lệnh rồi đưa kết quả thật) thay vì chỉ hỏi lại. Đầu ra của lệnh là sự thật: không tính nhẩm để phản bác nó; nghi ngờ thì chạy lệnh kiểm tra.
MEMORY: {"text":"<một sở thích/bối cảnh lâu dài Darren vừa nói rõ hoặc xác nhận>"}   ← chỉ khi Darren nói rõ muốn bạn nhớ hoặc nêu sở thích lâu dài; không bao giờ ghi bí mật, mật khẩu.
- Số liệu (giá, tỷ giá, ngày, phần trăm) chỉ được lấy từ KẾT QUẢ CÔNG CỤ, ghi nguồn dạng [1], [2]. Tuyệt đối không bịa hay ước đoán số; thiếu dữ liệu thì nói rõ thiếu. Khi đã có KẾT QUẢ CÔNG CỤ thì trả lời thẳng, không dùng PLAN.
- Không bao giờ nhắc tên kỹ thuật của công cụ hay chỉ thị (web_search, web_fetch, gold_price, TOOL…) trong câu trả lời; nói bằng lời thường như "tìm trên web", "đọc trang".
- Với email: kind là "email", dùng đúng địa chỉ Darren đưa; không lặp lại toàn văn thư ở phần trả lời.`

var gluedDirective = regexp.MustCompile(`([^\s\n])[ \t]*((?:PLAN|CHOICES|APPROVAL|SECRET|TOOL|MEMORY):\s*[\[{])`)

var directiveRe = regexp.MustCompile(`(?m)^\s*(PLAN|CHOICES|APPROVAL|SECRET|TOOL|MEMORY):\s*(.+?)\s*$`)

type directives struct {
	Text     string
	Plan     []string
	Choices  []string
	Approval *Approval
	Secret   *ledger.DirectSecret
	Tools    []toolReq // independent lookups, run in parallel (max 3)
	Memory   string
	Cut      bool // the reply hit the output cap or the stream broke
}

type toolReq struct {
	Name string `json:"name"`
	Q    string `json:"q,omitempty"`
	URL  string `json:"url,omitempty"`
	// computer tools
	Cmd     string `json:"cmd,omitempty"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
	Append  bool   `json:"append,omitempty"` // computer_write_file: add to the end (chunked writes)
	Old     string `json:"old,omitempty"`    // computer_edit_file: exact text to replace (once)
	New     string `json:"new,omitempty"`
}

func validTool(t toolReq) bool {
	switch t.Name {
	case "gold_price", "computer_list":
		return true
	case "web_search":
		return t.Q != ""
	case "web_fetch":
		return t.URL != ""
	case "computer_run":
		return strings.TrimSpace(t.Cmd) != ""
	case "computer_read_file":
		return t.Path != ""
	case "computer_edit_file":
		return t.Path != "" && t.Old != ""
	case "computer_write_file":
		return t.Path != "" && len(t.Content) <= 1<<20
	}
	return false
}

// ParseDirectives strips trailing PLAN/CHOICES/APPROVAL lines from model text.
func ParseDirectives(text string) directives {
	var d directives
	// Models sometimes glue a directive onto the previous sentence
	// ("…tại Việt Nam.TOOL: {…}"); put it back on its own line.
	text = normalizeToolLabels(text)
	text = gluedDirective.ReplaceAllString(text, "$1\n$2")
	// APPROVAL bodies may span lines; handle it first with the dedicated parser.
	text, d.Approval = SplitApproval(text)
	for _, m := range directiveRe.FindAllStringSubmatch(text, -1) {
		if m[1] == "TOOL" {
			var t toolReq
			if json.Unmarshal([]byte(m[2]), &t) == nil && validTool(t) && len(d.Tools) < 3 {
				d.Tools = append(d.Tools, t)
			}
			continue
		}
		if m[1] == "MEMORY" {
			var x struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(m[2]), &x) == nil {
				d.Memory = clean(x.Text, 300)
			}
			continue
		}
		if m[1] == "SECRET" {
			var x ledger.DirectSecret
			if json.Unmarshal([]byte(m[2]), &x) == nil {
				x.Name, x.Label, x.Desc = strings.ToUpper(strings.TrimSpace(x.Name)), clean(x.Label, 80), clean(x.Desc, 300)
				if x.Label == "" {
					x.Label = x.Name
				}
				if ledger.ValidDirectSecret(&x) {
					d.Secret = &x
				}
			}
			continue
		}
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
		if to, ok := outbound.ValidEmail(a.To); ok {
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

// eligible reports whether the direct runner may execute this work: plain
// owner-to-bot intake with no attested-runtime, account, project, plugin or
// collaboration requirements (those stay with the host/executor pipeline).
// Default plugin pins are fine: a direct run grants no tools at all.
func eligible(w ledger.Work) bool {
	return w.Status == "waiting_host" && w.DesiredState == "running" && w.Direct == nil && w.Core == nil && w.CoreProfilePin == nil && w.CoreProfileSelection == nil && w.Collaboration == nil && w.Account == nil && w.Resources == nil && w.RequesterPin == nil && (w.ConversationID == "conversation-"+w.BotID || isGroupID(w.ConversationID))
}

// runLoop dispatches queued work one turn per bot, oldest first.
func (s *Service) runLoop() {
	t := time.NewTicker(150 * time.Millisecond)
	defer t.Stop()
	sweep := time.NewTicker(heartbeat)
	defer sweep.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-sweep.C:
			s.sweepLeases()
			continue
		case <-t.C:
		case <-s.wake:
		}
		snap := s.L.Snapshot()
		v := newView(snap)
		for botID, l := range v.allBot {
			if v.archived[botID] {
				continue
			}
			busy := false
			for _, w := range l {
				if w.Status == "running" || w.Status == "cancelling" {
					busy = true
				}
			}
			s.mu.Lock()
			if s.running[botID] || len(s.running) >= 4 {
				busy = true
			}
			s.mu.Unlock()
			if busy {
				continue
			}
			for _, w := range l {
				if !eligible(w) {
					continue
				}
				s.mu.Lock()
				s.running[botID] = true
				s.mu.Unlock()
				go func(w ledger.Work) {
					defer func() { s.mu.Lock(); delete(s.running, w.BotID); s.mu.Unlock(); s.poke() }()
					s.runWork(w)
				}(w)
				break
			}
		}
	}
}

func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// recoverInterrupted requeues direct runs a crash or restart left open
// (bounded by ledger.MaxDirectResumes, then failed) and closes stops.
func (s *Service) recoverInterrupted() {
	for _, w := range s.L.Snapshot().Work {
		if w.Direct == nil {
			continue
		}
		switch w.Status {
		case "running":
			s.resumeOrFail(w)
		case "cancelling":
			_, _ = s.L.DirectStopped(w.RequestID)
		}
	}
}

func (s *Service) resumeOrFail(w ledger.Work) {
	if _, err := s.L.DirectResume(w.RequestID); err == nil {
		log.Printf("aurora resumed %s after interruption (attempt %d)", w.RequestID, w.DirectResumes+1)
		s.record(w.BotID, w.RequestID, "step", "Tiếp tục việc bị gián đoạn")
		s.poke()
		return
	}
	_, _ = s.L.DirectFinish(w.RequestID, w.Epoch, "failed", "interrupted_by_restart", nil)
}

// Lease/heartbeat (OpenMuse worker.ts pattern, MIT): a running turn renews
// its lease; the sweeper requeues any run whose lease lapsed while no
// goroutine in this process owns its bot (a hung or lost run).
var (
	leaseFor  = 90 * time.Second
	heartbeat = 30 * time.Second
)

func (s *Service) leaseUntil() string { return s.Now().Add(leaseFor).UTC().Format(time.RFC3339) }

func (s *Service) holdLease(ctx context.Context, id string) {
	t := time.NewTicker(heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if cur, err := s.L.Get(id); err == nil && cur.Status == "running" {
				_, _ = s.L.DirectUpdate(id, cur.Epoch, func(r *ledger.DirectRun) { r.LeaseUntil = s.leaseUntil() })
			}
		}
	}
}

func (s *Service) sweepLeases() {
	now := s.Now()
	for _, w := range s.L.Snapshot().Work {
		if w.Status != "running" || w.Direct == nil || w.Direct.LeaseUntil == "" {
			continue
		}
		until, err := time.Parse(time.RFC3339, w.Direct.LeaseUntil)
		if err != nil || now.Before(until) {
			continue
		}
		s.mu.Lock()
		owned := s.running[w.BotID]
		s.mu.Unlock()
		if !owned {
			s.resumeOrFail(w)
		}
	}
}

func (s *Service) history(botID, current string) []ChatMsg {
	v := newView(s.L.Snapshot())
	b, _ := v.bot(botID)
	msgs := []ChatMsg{{Role: "system", Content: fmt.Sprintf(systemPrompt, b.Name, v.desc(b)) + memoryPrompt(s.Mem.List(botID)) + s.computerNote(botID) + clockNote(time.Now())}}
	if cw, err := s.L.Get(current); err == nil && isEnglish(cw.Text) {
		// the owner wrote English: the Vietnamese-first prompt must not win
		msgs[0].Content += "\nLANGUAGE: the user's latest message is in English. Reply entirely in natural English (keep [n] citations and directive lines as specified)."
	}
	l := v.byBot[botID]
	if cw, err := s.L.Get(current); err == nil && isGroupID(cw.ConversationID) {
		return s.groupHistory(v, b, cw, msgs)
	}
	if len(l) > maxHistoryTurns {
		l = l[len(l)-maxHistoryTurns:]
	}
	for _, w := range l {
		if w.Direct == nil || w.Direct.Kind == "" || w.Direct.Kind == "computer" {
			msgs = append(msgs, ChatMsg{"user", w.Text})
		}
		if w.RequestID == current {
			break
		}
		c := ""
		if w.Direct != nil {
			c = w.Direct.PlanIntro
			for i, st := range w.Direct.Plan {
				c += fmt.Sprintf("\n[Bước %d: %s — %s]", i+1, st.Title, st.Status)
			}
		}
		if r := visibleText(w.Reply); r != "" {
			c += "\n" + r
		}
		if w.Direct != nil && w.Direct.Approval != nil && w.Direct.Approval.Kind == "computer" {
			// never show the model its own approval boilerplate: it learns to
			// type it instead of printing a TOOL line (live 2026-10-23)
			c = strings.Replace(c, computerAskText, "", 1)
			c += fmt.Sprintf("\n(đã gọi công cụ computer_run, trạng thái duyệt: %s)", w.Direct.Approval.State)
		} else if w.Direct != nil && w.Direct.Approval != nil {
			c += fmt.Sprintf("\n[Đề xuất cần duyệt: %s — trạng thái: %s]", w.Direct.Approval.Title, w.Direct.Approval.State)
		}
		if w.Status == "failed" || w.Status == "cancelled" {
			c += "\n[" + failureText(w.Failure) + "]"
		}
		if c = strings.TrimSpace(c); c != "" {
			msgs = append(msgs, ChatMsg{"assistant", c})
		}
	}
	return compactHistory(msgs)
}

func (s *Service) agui(e AGUIEvent) { s.publish("agui", e) }

// call streams one model call; deltas go to SSE as TEXT_MESSAGE_CONTENT.
func (s *Service) call(ctx context.Context, w ledger.Work, msgs []ChatMsg) (directives, error) {
	msgs = s.personalize(msgs)
	s.agui(AGUIEvent{Type: TEXT_MESSAGE_START, ThreadID: w.BotID, RunID: w.RequestID, MessageID: w.RequestID})
	// Deltas are coalesced (~60ms) and each event also carries the full text so far,
	// so a burst that overflows a slow client's buffer can never drop characters.
	var sb strings.Builder
	var pend string
	var last time.Time
	flush := func() {
		if pend == "" {
			return
		}
		s.publish("delta", map[string]any{"type": TEXT_MESSAGE_CONTENT, "agentId": threadOf(w), "from": w.BotID, "id": w.RequestID, "messageId": w.RequestID, "delta": pend, "text": sb.String()})
		pend, last = "", time.Now()
	}
	full, err := s.AI.Stream(ctx, msgs, func(d string) {
		sb.WriteString(d)
		pend += d
		if time.Since(last) >= 60*time.Millisecond {
			flush()
		}
	})
	flush()
	cut := strings.Contains(full, truncMark)
	d := ParseDirectives(strings.ReplaceAll(full, truncMark, ""))
	d.Cut = cut
	return d, err
}

func (s *Service) endDraft(w ledger.Work) {
	s.sync()
	s.agui(AGUIEvent{Type: TEXT_MESSAGE_END, ThreadID: w.BotID, RunID: w.RequestID, MessageID: w.RequestID})
	s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
}

func (s *Service) runWork(w ledger.Work) {
	if strings.HasPrefix(w.ClientNonce, "brief-") {
		s.runBrief(w)
		return
	}
	run := ledger.DirectRun{Runtime: s.AI.Runtime(), Model: s.AI.ModelName(), LeaseUntil: s.leaseUntil()}
	if strings.HasPrefix(w.ClientNonce, "computer-") {
		run.Kind = "computer" // hidden follow-up after an approved command
	}
	if !s.AI.Configured() {
		run.Model = ""
	}
	w, err := s.L.DirectStart(w.RequestID, w.Epoch, run)
	if err != nil {
		return
	}
	name := s.botName(w.BotID)
	s.record(w.BotID, w.RequestID, "message", "Bạn giao cho "+name+": "+w.Text)
	s.agui(AGUIEvent{Type: RUN_STARTED, ThreadID: w.BotID, RunID: w.RequestID})
	ctx, cancel := context.WithTimeout(s.ctx, turnBudget+turnSlack)
	defer cancel()
	go s.watchCancel(ctx, cancel, w.RequestID)
	go s.holdLease(ctx, w.RequestID)
	appr, choices, err := s.runTurn(ctx, w)
	if err != nil {
		cur, gerr := s.L.Get(w.RequestID)
		if gerr == nil && cur.Status == "cancelling" {
			_, _ = s.L.DirectStopped(w.RequestID)
			s.endDraft(w)
			s.agui(AGUIEvent{Type: RUN_FINISHED, ThreadID: w.BotID, RunID: w.RequestID})
			return
		}
		code := clean(err.Error(), 150)
		if errors.Is(err, ErrNoAI) {
			code = "ai_not_configured"
		}
		if s.ctx.Err() != nil {
			// Root cause of "ai_unreachable after restart" (live 2026-10-23):
			// SIGTERM cancels the in-flight model call and the dying process
			// recorded that as a failure. Leave the run open; the next process
			// resumes it from its checkpoint.
			log.Printf("aurora: shutting down, %s left for resume", w.RequestID)
			return
		}
		if gerr == nil && cur.Status == "running" && (code == "ai_unreachable" || code == "ai_stream_broken") && cur.DirectResumes < ledger.MaxDirectResumes {
			// transient provider/network loss: requeue (from the checkpoint)
			// after a pause instead of failing the owner's task
			s.endDraft(w)
			go func() {
				select {
				case <-s.ctx.Done():
				case <-time.After(requeueDelay):
					if c, err := s.L.Get(w.RequestID); err == nil && c.Status == "running" {
						s.resumeOrFail(c)
					}
				}
			}()
			return
		}
		if gerr == nil && cur.Status == "running" {
			_, _ = s.L.DirectFinish(w.RequestID, cur.Epoch, "failed", code, nil)
		}
		s.record(w.BotID, w.RequestID, "error", name+": "+failureText(code))
		s.endDraft(w)
		s.agui(AGUIEvent{Type: RUN_ERROR, ThreadID: w.BotID, RunID: w.RequestID, Message: code})
		return
	}
	cur, gerr := s.L.Get(w.RequestID)
	if gerr != nil || cur.Status != "running" {
		if gerr == nil && cur.Status == "cancelling" {
			_, _ = s.L.DirectStopped(w.RequestID)
		}
		s.endDraft(w)
		return
	}
	_, _ = s.L.DirectFinish(w.RequestID, cur.Epoch, "completed", "", func(d *ledger.DirectRun) {
		if appr != nil {
			d.Approval = appr
		}
		d.Choices = choices
	})
	if appr != nil && appr.Kind == "computer" {
		s.supersedeCommands(w.BotID, w.RequestID)
	}
	s.endDraft(w)
	s.agui(AGUIEvent{Type: RUN_FINISHED, ThreadID: w.BotID, RunID: w.RequestID})
	if s.absorbed(w.RequestID) {
		return // folded into an earlier run: no activity line or push of its own
	}
	if done, _ := s.L.Get(w.RequestID); done.Direct != nil && len(done.Direct.Plan) > 0 {
		s.record(w.BotID, w.RequestID, "plan", name+" chạy kế hoạch "+itoa(len(done.Direct.Plan))+" bước")
	} else {
		s.record(w.BotID, w.RequestID, "reply", name+" đã xong")
	}
	if appr != nil {
		s.record(w.BotID, w.RequestID, "approval", name+" xin duyệt “"+appr.Title+"”")
	}
	switch {
	case appr != nil:
		s.notify(name+" cần bạn duyệt", appr.Title, "/app/#"+threadOf(w))
	case len(choices) > 0:
		s.notify(name+" cần bạn chọn", "Chọn một phương án để tiếp tục", "/app/#"+threadOf(w))
	default:
		s.notify(name, "Đã xong việc bạn giao.", "/app/#"+threadOf(w))
	}
}

// watchCancel turns a ledger cancel (desktop or mobile Stop) into ctx cancel.
func (s *Service) watchCancel(ctx context.Context, cancel context.CancelFunc, id string) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if cur, err := s.L.Get(id); err != nil || cur.DesiredState != "running" {
				cancel()
				return
			}
		}
	}
}

func (s *Service) appendText(id, text string) error {
	cur, err := s.L.Get(id)
	if err != nil {
		return err
	}
	if text = strings.TrimSpace(text); text == "" {
		return nil
	}
	text = scrubInternal(text)
	if s.PC != nil {
		var bad []string
		if text, bad = s.verifyClaims(cur.BotID, text, computerTask(cur)); len(bad) > 0 && computerTask(cur) && !strings.Contains(text, unverifiedFooter) {
			text += "\n\n" + unverifiedFooter
		}
	}
	if cur.Reply != "" {
		text = "\n\n" + text
	}
	if len(cur.Reply)+len(text) > 60000 {
		text = text[:max(0, 60000-len(cur.Reply))]
		text = strings.ToValidUTF8(text, "")
	}
	return s.L.AppendText(id, cur.Epoch, s.V.Redact(text))
}

func (s *Service) update(id string, fn func(*ledger.DirectRun)) {
	if cur, err := s.L.Get(id); err == nil && cur.Status == "running" {
		_, _ = s.L.DirectUpdate(id, cur.Epoch, fn)
	}
	s.sync()
}

// maxToolRounds bounds model-requested lookups per turn; multi-part research
// (search each item, then compare) needs a few more than a single fact.
const maxToolRounds = 5

const finalNudge = "Đã hết lượt tra cứu cho lượt này. Viết NGAY câu trả lời cuối cùng chỉ từ KẾT QUẢ CÔNG CỤ ở trên, trích [n], nói rõ phần nào còn thiếu dữ liệu. Không viết dòng TOOL."

const actNudge = "Bạn vừa nói sẽ làm (hoặc chép dòng “(đã gọi công cụ …)” — dòng đó KHÔNG gọi công cụ) nhưng chưa làm gì. Làm ngay trong lượt này: việc nhiều bước thì viết PLAN, cần số liệu thì in đúng một dòng TOOL, còn lại thì trả lời đầy đủ luôn. Không hứa thêm."

const planNudge = "Yêu cầu này gồm nhiều phần. Trả lời đúng 1 câu nêu cách làm, rồi một dòng PLAN 3-6 bước ngắn (bước cuối là tổng hợp/viết kết quả). Chưa tra cứu, chưa trả lời chi tiết. Nếu thật ra chỉ là một câu hỏi đơn giản thì trả lời luôn."

var multiPartRe = regexp.MustCompile(`(?i)(rồi|sau đó|tiếp theo|cuối cùng|so sánh|báo cáo|lịch trình|kế hoạch|từng bước|nhiều bước)`)

// multiPart flags a long request with several parts ("nghiên cứu … so sánh …
// rồi viết báo cáo") that should run as a step plan.
func multiPart(t string) bool {
	return len([]rune(t)) >= 120 && len(multiPartRe.FindAllString(t, -1)) >= 2
}

var promiseRe = regexp.MustCompile(`(?i)(^|\s)(sẽ|để (em|mình|tôi) (tra|làm|tìm|xem)|bắt đầu|đang tra|chờ (em|mình|tôi)|(kiểm tra|xem) lại|trước khi (báo|trả lời|gửi)|I'll|I will|let me)`)

// promiseOnly reports a short reply that only announces work it never does
// (no directive, no question): dogfood v16 long-task failure.
func promiseOnly(d directives) bool {
	if len(d.Tools) == 0 && toolMimicRe.MatchString(d.Text) {
		// live run 3: the model copied the history's "(đã gọi công cụ
		// computer_write_file, trạng thái: pending)" line instead of a TOOL line
		return true
	}
	t := strings.TrimSpace(d.Text)
	return len(d.Tools) == 0 && d.Approval == nil && d.Secret == nil && len(d.Plan) == 0 && len(d.Choices) == 0 &&
		t != "" && len([]rune(t)) < 220 && !strings.Contains(t, "?") && promiseRe.MatchString(t)
}

func (s *Service) runTurn(ctx context.Context, w ledger.Work) (*Approval, []string, error) {
	start := time.Now()
	tc := &toolCtx{}
	defer s.saveResearch(w.BotID, tc)
	if s.absorbed(w.RequestID) {
		// Already folded into the previous run (shown above its answer):
		// finish silently instead of a second, redundant reply.
		return nil, nil, nil
	}
	// Reply flow: short acknowledgement first, then the tool status line, then
	// the result as its own message (progress only at milestones).
	if auto := s.autoTools(w); len(auto) > 0 {
		s.ack(w, ackFor(auto))
		for _, c := range auto {
			s.runTool(ctx, w, tc, c.name, c.arg)
		}
	}
	var d directives
	var err error
	followUp := strings.HasPrefix(w.ClientNonce, "computer-") // tool output must never trigger planning
	var resumed []ledger.DirectStep
	if !followUp {
		s.newRequest(w.BotID, w.Text, computerTask(w))
	}
	if cp := w.ResumeFrom; cp != nil && !followUp && len(cp.Plan) >= 2 {
		// Restarted mid-plan: keep the finished steps (and their sources)
		// and continue from the next one instead of starting over.
		resumed = cp.Plan
		tc.sources = append(tc.sources, cp.Sources...)
		titles := make([]string, len(cp.Plan))
		for i, st := range cp.Plan {
			titles[i] = st.Title
		}
		d = directives{Text: cp.PlanIntro, Plan: titles}
	} else if steps := userSteps(w.Text); !followUp && len(steps) >= 3 && threadOf(w) == w.BotID {
		// The owner already listed the steps: run them as a server-driven plan
		// (one milestone update per step, then a final result) without asking
		// the model to re-plan.
		d = directives{Text: fmt.Sprintf("Mình sẽ làm %d bước và báo sau mỗi bước.", len(steps)), Plan: steps}
	} else if !followUp && multiPart(w.Text) && threadOf(w) == w.BotID {
		// Long multi-part request without numbered steps: ask the model to
		// propose the plan first, so it runs with milestones.
		d, err = s.ask(ctx, w, tc, func() []ChatMsg {
			return append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: planNudge})
		})
	} else {
		d, err = s.ask(ctx, w, tc, func() []ChatMsg { return tc.inject(s.history(w.BotID, w.RequestID)) })
	}
	if err == nil && promiseOnly(d) {
		// "Em sẽ làm từng bước…" and then nothing: ask once for the action itself.
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		if cur, _ := s.L.Get(w.RequestID); cur.Direct != nil && cur.Direct.Ack == "" {
			s.ack(w, firstLine(d.Text))
		}
		d, err = s.ask(ctx, w, tc, func() []ChatMsg {
			return append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: actNudge})
		})
	}
	rounds := maxToolRounds
	if followUp {
		rounds = 8 // build loop: write file, list, run, verify
	}
	for attempt := 0; ; attempt++ {
		for round := 0; err == nil && len(d.Tools) > 0 && round < rounds && s.Tools != nil; round++ {
			s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
			calls := callsOf(d.Tools)
			if cur, _ := s.L.Get(w.RequestID); cur.Direct != nil && cur.Direct.Ack == "" {
				s.ack(w, ackText(d.Text, threadOf(w) != w.BotID, calls))
			}
			s.runTools(ctx, w, tc, calls)
			if tc.pendingCmd != "" {
				// a command needs the owner: end the turn on its approval card
				d = directives{Text: computerAskText, Approval: computerApproval(tc.pendingCmd)}
				break
			}
			if cp, ok := s.checkpoint(w, start); ok {
				d = cp // out of time: continue as a fresh turn instead of a deadline error
				break
			}
			d, err = s.ask(ctx, w, tc, func() []ChatMsg { return tc.inject(s.history(w.BotID, w.RequestID)) })
		}
		// A reply that narrates tool work ("[đã ghi render_video.py]") with no
		// tool call behind it: strip the claim and ask for directives only.
		if err != nil || attempt >= 3 {
			break
		}
		if d.Approval != nil && d.Approval.Kind == "computer" || d.Text == checkpointText {
			break
		}
		nudge := ""
		if d.Cut && len(d.Tools) == 0 {
			nudge = cutNudge // a file body cut by the output cap: the TOOL line was lost
		} else if s.narratedOnly(w, d) {
			d.Text, nudge = stripNarration(d.Text), directivesOnly
		} else if n := s.continueNudge(w, d); n != "" {
			// the task is not finished (open checklist items, or a reply that
			// announces the next work and stops): keep working in this turn
			nudge = n
		}
		if nudge == "" {
			break
		}
		if cp, ok := s.checkpoint(w, start); ok {
			d = cp
			break
		}
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		d, err = s.ask(ctx, w, tc, func() []ChatMsg {
			return append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: nudge})
		})
	}
	if err == nil && len(d.Tools) == 0 && d.Approval == nil {
		d.Text = stripNarration(d.Text) // never show a claimed action that did not happen
		if cp, ok := s.unfinished(w, d); ok {
			d = cp
		}
	}
	if err == nil && len(d.Tools) > 0 && len(tc.blocks) > 0 {
		// Out of lookups but holding real results: write the answer from them
		// instead of ending on a bare "no data" note (dogfood v16 compare task).
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		msgs := append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: finalNudge})
		d, err = s.call(ctx, w, msgs)
	}
	if err == nil && len(d.Plan) == 0 && s.steer(w, tc) {
		// The owner changed course while this answer was being written:
		// discard the draft and answer once more with the new instruction.
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		d, err = s.call(ctx, w, tc.inject(s.history(w.BotID, w.RequestID)))
		if err == nil && len(d.Tools) > 0 && s.Tools != nil {
			s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
			s.runTools(ctx, w, tc, callsOf(d.Tools))
			d, err = s.call(ctx, w, append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: finalNudge}))
		}
	}
	if err != nil {
		return nil, nil, err
	}
	s.fixScript(ctx, w, &d)
	s.remember(w, d.Memory)
	s.reviewMemory(w, d.Memory)
	if len(d.Tools) > 0 { // tools exhausted or disabled: never leave a bare TOOL line
		d.Text = strings.TrimSpace(d.Text + "\n(Mình chưa lấy thêm được dữ liệu, nên không đưa số liệu chưa kiểm chứng.)")
	}
	if d.Secret != nil {
		sec := *d.Secret
		s.update(w.RequestID, func(r *ledger.DirectRun) { r.Secret = &sec })
		s.record(w.BotID, w.RequestID, "secret", "Xin bí mật "+sec.Name)
	}
	if len(d.Plan) < 2 || d.Approval != nil {
		if d.Approval == nil {
			s.verifyFinal(ctx, w, tc, &d)
		}
		return d.Approval, d.Choices, s.appendText(w.RequestID, normalizeCites(d.Text, len(tc.sources)))
	}
	steps := make([]Step, len(d.Plan))
	for i, t := range d.Plan {
		steps[i] = Step{Title: t, Status: "pending"}
		if i < len(resumed) && resumed[i].Status == "done" {
			steps[i] = resumed[i]
		}
	}
	s.update(w.RequestID, func(r *ledger.DirectRun) {
		r.PlanIntro, r.Plan = clean(planIntro(d.Text), 600), steps
		if len(resumed) > 0 {
			r.Sources = append([]ledger.DirectSource(nil), tc.sources...)
		}
	})
	s.endDraft(w)
	setStep := func(i int, st string) {
		s.update(w.RequestID, func(r *ledger.DirectRun) {
			r.Plan[i].Status = st
			if st == "cancelled" || st == "failed" || st == "waiting" {
				for j := i + 1; j < len(r.Plan); j++ {
					r.Plan[j].Status = "cancelled"
				}
			}
		})
	}
	tc.lookupOnly = !wantsComputer.MatchString(w.Text)
	for i, title := range d.Plan {
		if steps[i].Status == "done" { // finished before the restart
			tc.note(fmt.Sprintf("KẾT QUẢ BƯỚC %d (%s): %s", i+1, title, steps[i].Note))
			continue
		}
		if ctx.Err() != nil {
			setStep(i, "cancelled")
			return nil, nil, ctx.Err()
		}
		setStep(i, "running")
		s.agui(AGUIEvent{Type: STEP_STARTED, ThreadID: w.BotID, RunID: w.RequestID, StepName: title})
		prompt := fmt.Sprintf("[Hệ thống] Thực hiện bước %d/%d: %s. Chỉ làm bước này. Cần số liệu hiện tại thì in đúng một dòng TOOL (tra cứu thông tin luôn dùng công cụ web/giá; máy tính riêng không có mạng, chỉ dùng khi bước yêu cầu chạy/tính trên file). Trả lời ngắn gọn (tối đa 5 gạch đầu dòng; bước yêu cầu bảng hay code thì đưa đủ bảng markdown hoặc khối code markdown) chỉ nêu kết quả của bước, có [n] nếu dùng nguồn; không viết “Kết quả bước…” hay “Bước tiếp theo…”. Không hỏi lựa chọn khi bước đã làm xong. Không viết PLAN. Đừng tự tạo bước chờ Darren duyệt. Nếu thật sự thiếu thông tin chỉ Darren biết, hỏi 1 câu và dùng CHOICES. Chỉ dùng APPROVAL nếu bước này cần hành động ra bên ngoài.", i+1, len(d.Plan), title)
		sd, err := s.stepCall(ctx, w, tc, prompt)
		if err != nil {
			st := "failed"
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				st = "cancelled"
			}
			setStep(i, st)
			return nil, nil, err
		}
		s.fixScript(ctx, w, &sd)
		s.agui(AGUIEvent{Type: STEP_FINISHED, ThreadID: w.BotID, RunID: w.RequestID, StepName: title})
		if len(sd.Choices) > 0 && sd.Approval == nil && (len([]rune(sd.Text)) > 300 || !strings.Contains(sd.Text, "?")) {
			sd.Choices = nil // a finished step offering options is not a real question
		}
		if sd.Approval != nil || len(sd.Choices) > 0 {
			setStep(i, "waiting")
			s.endDraft(w)
			return sd.Approval, sd.Choices, s.appendText(w.RequestID, normalizeCites(sd.Text, len(tc.sources)))
		}
		note := normalizeCites(clean(scrubInternal(stepNote(sd.Text)), 1500), len(tc.sources))
		tc.note(fmt.Sprintf("KẾT QUẢ BƯỚC %d (%s): %s", i+1, title, note))
		s.update(w.RequestID, func(r *ledger.DirectRun) {
			r.Plan[i].Status, r.Plan[i].Note = "done", note
		})
		s.endDraft(w)
		if i < len(d.Plan)-1 {
			s.notify(fmt.Sprintf("%s · bước %d/%d xong", s.botName(w.BotID), i+1, len(d.Plan)), clean(firstLine(note), 140), "/app/#"+threadOf(w))
		}
	}
	// Final result: one complete answer built from every step's outcome.
	fd, err := s.stepCall(ctx, w, tc, "[Hệ thống] Mọi bước đã xong. Viết KẾT QUẢ CUỐI CÙNG hoàn chỉnh cho yêu cầu ban đầu, dựa trên KẾT QUẢ BƯỚC và KẾT QUẢ CÔNG CỤ, trích [n]. Không nhắc lại tiến độ, không PLAN, không TOOL.")
	if err != nil {
		return nil, nil, err
	}
	s.fixScript(ctx, w, &fd)
	if fd.Approval == nil {
		s.verifyFinal(ctx, w, tc, &fd)
	}
	if err := s.appendText(w.RequestID, normalizeCites(fd.Text, len(tc.sources))); err != nil {
		return nil, nil, err
	}
	s.endDraft(w)
	return fd.Approval, fd.Choices, nil
}

// verifyFinal: a final reply of a computer task that claims files which do
// not exist is asked once more (with the real file list), then rewritten.
func (s *Service) verifyFinal(ctx context.Context, w ledger.Work, tc *toolCtx, d *directives) {
	if s.PC == nil || !computerTask(w) {
		return
	}
	_, bad := s.verifyClaims(w.BotID, d.Text, true)
	if len(bad) == 0 {
		return
	}
	s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
	nd, err := s.ask(ctx, w, tc, func() []ChatMsg {
		return append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: verifyNudge(bad, s.computerContext(w.BotID))})
	})
	if err != nil {
		return
	}
	for r := 0; len(nd.Tools) > 0 && r < 4 && s.Tools != nil; r++ { // it may go back to work (file writes, a command card)
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		s.runTools(ctx, w, tc, callsOf(nd.Tools))
		if tc.pendingCmd != "" {
			*d = directives{Text: computerAskText, Approval: computerApproval(tc.pendingCmd)}
			return
		}
		if nd, err = s.ask(ctx, w, tc, func() []ChatMsg { return tc.inject(s.history(w.BotID, w.RequestID)) }); err != nil {
			return
		}
	}
	nd.Tools = nil
	*d = nd
}

// stepCall runs one model call with its own small tool loop and always ends
// in text (never a bare TOOL line).
func (s *Service) stepCall(ctx context.Context, w ledger.Work, tc *toolCtx, prompt string) (directives, error) {
	mk := func(extra ...ChatMsg) []ChatMsg {
		return append(append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "user", Content: prompt}), extra...)
	}
	d, err := s.ask(ctx, w, tc, func() []ChatMsg { return mk() })
	for r := 0; err == nil && len(d.Tools) > 0 && r < 4 && s.Tools != nil; r++ {
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		s.runTools(ctx, w, tc, callsOf(d.Tools))
		if tc.pendingCmd != "" {
			return directives{Text: "Bước này cần chạy lệnh trên máy tính của mình, bạn duyệt giúp nhé.", Approval: computerApproval(tc.pendingCmd)}, nil
		}
		d, err = s.ask(ctx, w, tc, func() []ChatMsg { return mk() })
	}
	if err == nil && s.narratedOnly(w, d) {
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		d, err = s.ask(ctx, w, tc, func() []ChatMsg { return mk(ChatMsg{Role: "system", Content: directivesOnly}) })
		for r := 0; err == nil && len(d.Tools) > 0 && r < 4 && s.Tools != nil; r++ {
			s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
			s.runTools(ctx, w, tc, callsOf(d.Tools))
			if tc.pendingCmd != "" {
				return directives{Text: "Bước này cần chạy lệnh trên máy tính của mình, bạn duyệt giúp nhé.", Approval: computerApproval(tc.pendingCmd)}, nil
			}
			d, err = s.ask(ctx, w, tc, func() []ChatMsg { return mk() })
		}
	}
	if err == nil && len(d.Tools) > 0 {
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		d, err = s.call(ctx, w, mk(ChatMsg{Role: "system", Content: finalNudge}))
	}
	if len(d.Tools) == 0 && d.Approval == nil {
		d.Text = stripNarration(d.Text)
	}
	s.remember(w, d.Memory)
	s.reviewMemory(w, d.Memory)
	d.Tools, d.Plan, d.Memory = nil, nil, ""
	return d, err
}

var (
	stepMarkRe = regexp.MustCompile(`(?m)(?:^|\s)(?:\((\d{1,2})\)|(\d{1,2})[.)])\s+`)
	citeRe     = regexp.MustCompile(`\[(\d{1,2})(?:\]|[^\[\]\n]{1,60}\])`)
)

// userSteps extracts an explicit numbered task list ("(1) … (2) …" or
// "1. …\n2. …") from the owner's message; numbers must run 1,2,3….
func userSteps(text string) []string {
	if requirementsRe.MatchString(text) {
		// a numbered list of requirements describes the deliverable; it is not
		// the work plan (live pptx task: "1. File .pptx thật…" became step 1)
		return nil
	}
	idx := stepMarkRe.FindAllStringSubmatchIndex(text, -1)
	var out []string
	for k, m := range idx {
		var num string
		if m[2] >= 0 {
			num = text[m[2]:m[3]]
		} else {
			num = text[m[4]:m[5]]
		}
		if num != fmt.Sprint(len(out)+1) {
			return nil
		}
		end := len(text)
		if k+1 < len(idx) {
			end = idx[k+1][0]
		}
		t := strings.TrimRight(strings.TrimSpace(text[m[1]:end]), ",;.")
		if t == "" {
			return nil
		}
		out = append(out, clean(t, 120))
	}
	if len(out) > 8 {
		return nil
	}
	return out
}

var (
	noteHeadRe = regexp.MustCompile(`(?im)^\s*(\*\*)?(kết quả bước|bước \d+(/\d+)? (xong|đã xong|hoàn thành)|đã xong bước)[^\n]*$`)
	noteNextRe = regexp.MustCompile(`(?i)\s*(đã xong bước \d+[.!]?\s*)?bước tiếp theo[^\n]*$`)
)

// stepNote trims progress chatter ("Kết quả bước 1/4:", "Bước tiếp theo mình
// sẽ…") from a step reply; the milestone header already says which step.
func stepNote(t string) string {
	t = noteHeadRe.ReplaceAllStringFunc(t, func(m string) string {
		if i := strings.Index(m, ":"); i >= 0 && strings.TrimSpace(m[i+1:]) != "" {
			return strings.TrimSpace(m[i+1:])
		}
		if i := strings.Index(m, ". "); i >= 0 {
			return strings.TrimSpace(m[i+2:])
		}
		return ""
	})
	t = noteNextRe.ReplaceAllString(strings.TrimSpace(t), "")
	return strings.TrimSpace(t)
}

var internalRe = regexp.MustCompile("(?i)(định dạng |công cụ |lệnh )?`?\\b(web_fetch|web_search|gold_price)\\b`?( tool)?")

var internalName = map[string]string{"web_fetch": "đọc trang", "web_search": "tìm trên web", "gold_price": "tra giá vàng"}

// scrubInternal replaces internal tool identifiers that leak into replies
// ("Định dạng web_fetch bị chặn") with plain Vietnamese.
// toolMimicRe is the history's tool-call note copied into a reply.
var toolMimicRe = regexp.MustCompile(`(?m)^[ \t]*\(đã gọi công cụ [a-z_]+, trạng thái[^)\n]*\)[ \t]*$`)

func scrubInternal(t string) string {
	t = strings.TrimSpace(toolMimicRe.ReplaceAllString(t, ""))
	return internalRe.ReplaceAllStringFunc(t, func(m string) string {
		k := strings.ToLower(internalRe.FindStringSubmatch(m)[2])
		return "việc " + internalName[k]
	})
}

// normalizeCites repairs footnotes: "[12 nguồn thứ 2]" → "[12]", and drops
// markers that point past the sources actually gathered this turn.
func normalizeCites(text string, n int) string {
	return citeRe.ReplaceAllStringFunc(text, func(m string) string {
		k := citeRe.FindStringSubmatch(m)[1]
		var v int
		fmt.Sscan(k, &v)
		if v < 1 || v > n {
			return ""
		}
		return "[" + k + "]"
	})
}

// hasHan reports CJK ideographs/kana/hangul: GLM occasionally drifts into
// Chinese mid-sentence ("báo mình找个 khung"), which must never reach a draft.
func hasHan(t string) bool {
	for _, r := range t {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func stripHan(t string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return -1
		}
		return r
	}, t)
}

// fixScript repairs script drift in the reply and approval draft with one quick
// rewrite call (unless Darren himself wrote CJK); falls back to stripping.
func (s *Service) fixScript(ctx context.Context, w ledger.Work, d *directives) {
	h := s.history(w.BotID, w.RequestID)
	if len(h) > 0 && hasHan(h[len(h)-1].Content) {
		return
	}
	en := hasStrayEnglish(h[len(h)-1].Content)
	fix := func(t string) string {
		if !hasHan(t) && (en || !hasStrayEnglish(t)) {
			return t
		}
		out, err := s.AI.Stream(ctx, []ChatMsg{{"system", "Bạn là biên tập viên tiếng Việt. Viết lại nguyên văn đoạn sau, chỉ thay các cụm chữ Hán/Nhật/Hàn và các từ nối tiếng Anh lạc vào câu (with, the, and, however…) bằng tiếng Việt tự nhiên đúng nghĩa. Giữ nguyên tên riêng, thuật ngữ, mã, URL, số liệu, xuống dòng và [n]. Chỉ trả về đoạn đã sửa."}, {"user", t}}, func(string) {})
		if out = strings.TrimSpace(out); err != nil || out == "" || hasHan(out) || len(out) < len(t)/2 {
			out = stripHan(t)
		}
		log.Printf("aurora repaired script drift (%d→%d chars)", len(t), len(out))
		return out
	}
	d.Text = fix(d.Text)
	if d.Approval != nil {
		d.Approval.Body, d.Approval.Subject, d.Approval.Title = fix(d.Approval.Body), fix(d.Approval.Subject), fix(d.Approval.Title)
	}
}

// ackText keeps the model's pre-tool sentence only when it reads like a clean
// one-line acknowledgement; group turns, lists, "Name: …" prefixes and long or
// multi-line text fall back to the server template.
func ackText(t string, group bool, calls []toolCall) string {
	t = strings.TrimSpace(t)
	if group || t == "" || len([]rune(t)) > 160 || strings.ContainsAny(t, "\n[|") || strings.HasPrefix(t, "-") || strings.HasPrefix(t, "•") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "#") {
		return ackFor(calls)
	}
	if i := strings.Index(t, ":"); i > 0 && i < 30 && strings.Count(t[:i], " ") < 4 {
		return ackFor(calls)
	}
	return t
}

// strayEnglish are English function words that never appear in normal
// Vietnamese prose (unlike "in", "to", "on"); one of them in a Vietnamese
// reply ("-2,75% so with 5 phiên") means the model drifted mid-sentence.
var strayEnglish = regexp.MustCompile(`(?i)(^|[\s(,])(with|the|and|however|which|because|although|should|would|into|than|about|from)(\s|[,.)]|$)`)
var codeOrURL = regexp.MustCompile("`[^`]*`|https?://\\S+|\"[^\"]*\"")

func hasStrayEnglish(t string) bool {
	return strayEnglish.MatchString(codeOrURL.ReplaceAllString(t, " "))
}

func callsOf(ts []toolReq) []toolCall {
	out := make([]toolCall, 0, len(ts))
	for _, t := range ts {
		if isComputerTool(t.Name) {
			out = append(out, toolCall{t.Name, computerArg(t)})
			continue
		}
		out = append(out, toolCall{t.Name, t.Q + t.URL})
	}
	return out
}

func (s *Service) remember(w ledger.Work, text string) {
	if text != "" && s.Mem.Remember(w.BotID, text) {
		s.record(w.BotID, w.RequestID, "step", "Đã ghi nhớ: "+clean(text, 120))
	}
}

const computerAskText = "Mình cần bạn duyệt lệnh này trước khi chạy trên máy tính của mình."

var enWords = regexp.MustCompile(`(?i)\b(the|is|are|what|how|when|why|which|and|of|to|in|for|please|can|you|it|my|me|with)\b`)

// isEnglish: no Vietnamese letters and several common English words.
func isEnglish(t string) bool {
	if strings.ContainsAny(strings.ToLower(t), "ăâđêôơưàáạảãằắặẳẵầấậẩẫèéẹẻẽềếệểễìíịỉĩòóọỏõồốộổỗờớợởỡùúụủũừứựửữỳýỵỷỹ") {
		return false
	}
	return len(enWords.FindAllString(t, 4)) >= 3
}

// wantsComputer: the owner asked for something run or computed on files.
var wantsComputer = regexp.MustCompile(`(?i)máy tính|chạy|lệnh|script|python|csv|file|tệp|\brun\b|command|terminal|shell`)

// requeueDelay is the pause before a run that lost the model is retried.
var requeueDelay = 15 * time.Second

var requirementsRe = regexp.MustCompile(`(?i)(yêu cầu|tiêu chí|requirements?)\s*:`)
