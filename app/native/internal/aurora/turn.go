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
- Trả lời bằng tiếng Việt, ngắn gọn, cụ thể, như một đồng nghiệp đáng tin. Chỉ dùng chữ Latin tiếng Việt: tuyệt đối không chèn chữ Hán/Nhật/Hàn. Markdown gọn (tiêu đề ngắn, gạch đầu dòng, bảng) chỉ khi giúp dễ đọc.
- Bạn KHÔNG tự gửi, đăng, thanh toán hay liên hệ ai bên ngoài. Không bao giờ nói là đã gửi.
- Cuối câu trả lời, bạn có thể thêm các dòng chỉ thị (mỗi loại tối đa một dòng, đúng JSON):
PLAN: ["bước 1","bước 2"]   ← chỉ khi việc cần 2-4 bước rõ ràng; khi đó phần trả lời chỉ cần 1 câu nêu cách làm.
CHOICES: ["lựa chọn A","lựa chọn B"]   ← khi cần Darren chọn nhanh (2-4 lựa chọn ngắn).
APPROVAL: {"title":"<việc cần duyệt>","kind":"email|send|post|pay|other","to":"<email nếu là email>","subject":"<tiêu đề nếu là email>","body":"<toàn bộ nội dung đã soạn>"}   ← bắt buộc cho mọi hành động ra bên ngoài.
SECRET: {"name":"TEN_IN_HOA","label":"<tên hiển thị>","desc":"<vì sao cần, lấy ở đâu>"}   ← khi cần Darren cung cấp mật khẩu/khoá API. Darren nhập vào thẻ ẩn, giá trị vào kho bí mật, bạn không bao giờ thấy. Không bao giờ xin Darren dán bí mật vào chat. Mật khẩu ứng dụng Gmail dùng tên GMAIL_APP_PASSWORD.
TOOL: {"name":"web_search","q":"<truy vấn>"} | {"name":"web_fetch","url":"https://..."} | {"name":"gold_price"}   ← công cụ đọc dữ liệu thật trên máy chủ. Khi cần số liệu/tin tức hiện tại mà chưa có trong KẾT QUẢ CÔNG CỤ, chỉ in đúng một dòng TOOL rồi dừng; hệ thống sẽ chạy và gọi lại bạn.
- Số liệu (giá, tỷ giá, ngày, phần trăm) chỉ được lấy từ KẾT QUẢ CÔNG CỤ, ghi nguồn dạng [1], [2]. Tuyệt đối không bịa hay ước đoán số; thiếu dữ liệu thì nói rõ thiếu. Khi đã có KẾT QUẢ CÔNG CỤ thì trả lời thẳng, không dùng PLAN.
- Với email: kind là "email", dùng đúng địa chỉ Darren đưa; không lặp lại toàn văn thư ở phần trả lời.`

var gluedDirective = regexp.MustCompile(`([^\s\n])[ \t]*((?:PLAN|CHOICES|APPROVAL|SECRET|TOOL):\s*[\[{])`)

var directiveRe = regexp.MustCompile(`(?m)^\s*(PLAN|CHOICES|APPROVAL|SECRET|TOOL):\s*(.+?)\s*$`)

type directives struct {
	Text     string
	Plan     []string
	Choices  []string
	Approval *Approval
	Secret   *ledger.DirectSecret
	Tool     *toolReq
}

type toolReq struct {
	Name string `json:"name"`
	Q    string `json:"q,omitempty"`
	URL  string `json:"url,omitempty"`
}

// ParseDirectives strips trailing PLAN/CHOICES/APPROVAL lines from model text.
func ParseDirectives(text string) directives {
	var d directives
	// Models sometimes glue a directive onto the previous sentence
	// ("…tại Việt Nam.TOOL: {…}"); put it back on its own line.
	text = gluedDirective.ReplaceAllString(text, "$1\n$2")
	// APPROVAL bodies may span lines; handle it first with the dedicated parser.
	text, d.Approval = SplitApproval(text)
	for _, m := range directiveRe.FindAllStringSubmatch(text, -1) {
		if m[1] == "TOOL" {
			var t toolReq
			if json.Unmarshal([]byte(m[2]), &t) == nil && (t.Name == "gold_price" || t.Name == "web_search" && t.Q != "" || t.Name == "web_fetch" && t.URL != "") && d.Tool == nil {
				d.Tool = &t
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
	for {
		select {
		case <-s.ctx.Done():
			return
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

// recoverInterrupted closes direct runs a crash left open.
func (s *Service) recoverInterrupted() {
	for _, w := range s.L.Snapshot().Work {
		if w.Direct == nil {
			continue
		}
		switch w.Status {
		case "running":
			_, _ = s.L.DirectFinish(w.RequestID, w.Epoch, "failed", "interrupted_by_restart", nil)
		case "cancelling":
			_, _ = s.L.DirectStopped(w.RequestID)
		}
	}
}

func (s *Service) history(botID, current string) []ChatMsg {
	v := newView(s.L.Snapshot())
	b, _ := v.bot(botID)
	msgs := []ChatMsg{{Role: "system", Content: fmt.Sprintf(systemPrompt, b.Name, v.desc(b))}}
	l := v.byBot[botID]
	if cw, err := s.L.Get(current); err == nil && isGroupID(cw.ConversationID) {
		return s.groupHistory(v, b, cw, msgs)
	}
	if len(l) > 30 {
		l = l[len(l)-30:]
	}
	for _, w := range l {
		if w.Direct == nil || w.Direct.Kind == "" {
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
		if w.Direct != nil && w.Direct.Approval != nil {
			c += fmt.Sprintf("\n[Đề xuất cần duyệt: %s — trạng thái: %s]", w.Direct.Approval.Title, w.Direct.Approval.State)
		}
		if w.Status == "failed" || w.Status == "cancelled" {
			c += "\n[" + failureText(w.Failure) + "]"
		}
		if c = strings.TrimSpace(c); c != "" {
			msgs = append(msgs, ChatMsg{"assistant", c})
		}
	}
	return msgs
}

func (s *Service) agui(e AGUIEvent) { s.publish("agui", e) }

// call streams one model call; deltas go to SSE as TEXT_MESSAGE_CONTENT.
func (s *Service) call(ctx context.Context, w ledger.Work, msgs []ChatMsg) (directives, error) {
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
	return ParseDirectives(full), err
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
	run := ledger.DirectRun{Runtime: s.AI.Runtime(), Model: s.AI.ModelName()}
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
	ctx, cancel := context.WithTimeout(s.ctx, 6*time.Minute)
	defer cancel()
	go s.watchCancel(ctx, cancel, w.RequestID)
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
	s.endDraft(w)
	s.agui(AGUIEvent{Type: RUN_FINISHED, ThreadID: w.BotID, RunID: w.RequestID})
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

const actNudge = "Bạn vừa nói sẽ làm nhưng chưa làm gì. Làm ngay trong lượt này: việc nhiều bước thì viết PLAN, cần số liệu thì in đúng một dòng TOOL, còn lại thì trả lời đầy đủ luôn. Không hứa thêm."

var promiseRe = regexp.MustCompile(`(?i)(^|\s)(sẽ|để (em|mình|tôi) (tra|làm|tìm|xem)|bắt đầu|đang tra|chờ (em|mình|tôi))`)

// promiseOnly reports a short reply that only announces work it never does
// (no directive, no question): dogfood v16 long-task failure.
func promiseOnly(d directives) bool {
	t := strings.TrimSpace(d.Text)
	return d.Tool == nil && d.Approval == nil && d.Secret == nil && len(d.Plan) == 0 && len(d.Choices) == 0 &&
		t != "" && len([]rune(t)) < 220 && !strings.Contains(t, "?") && promiseRe.MatchString(t)
}

func (s *Service) runTurn(ctx context.Context, w ledger.Work) (*Approval, []string, error) {
	tc := &toolCtx{}
	// Reply flow: short acknowledgement first, then the tool status line, then
	// the result as its own message (progress only at milestones).
	if auto := s.autoTools(w); len(auto) > 0 {
		s.ack(w, ackFor(auto))
		for _, c := range auto {
			s.runTool(ctx, w, tc, c.name, c.arg)
		}
	}
	d, err := s.call(ctx, w, tc.inject(s.history(w.BotID, w.RequestID)))
	if err == nil && promiseOnly(d) {
		// "Em sẽ làm từng bước…" and then nothing: ask once for the action itself.
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		if cur, _ := s.L.Get(w.RequestID); cur.Direct != nil && cur.Direct.Ack == "" {
			s.ack(w, firstLine(d.Text))
		}
		d, err = s.call(ctx, w, append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: actNudge}))
	}
	for round := 0; err == nil && d.Tool != nil && round < maxToolRounds && s.Tools != nil; round++ {
		arg := d.Tool.Q + d.Tool.URL
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		if cur, _ := s.L.Get(w.RequestID); cur.Direct != nil && cur.Direct.Ack == "" {
			s.ack(w, ackText(d.Text, threadOf(w) != w.BotID, []toolCall{{d.Tool.Name, arg}}))
		}
		s.runTool(ctx, w, tc, d.Tool.Name, arg)
		d, err = s.call(ctx, w, tc.inject(s.history(w.BotID, w.RequestID)))
	}
	if err == nil && d.Tool != nil && len(tc.blocks) > 0 {
		// Out of lookups but holding real results: write the answer from them
		// instead of ending on a bare "no data" note (dogfood v16 compare task).
		s.publish("draft_end", map[string]string{"id": w.RequestID, "agentId": threadOf(w)})
		msgs := append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{Role: "system", Content: finalNudge})
		d, err = s.call(ctx, w, msgs)
	}
	if err != nil {
		return nil, nil, err
	}
	s.fixScript(ctx, w, &d)
	if d.Tool != nil { // tools exhausted or disabled: never leave a bare TOOL line
		d.Text = strings.TrimSpace(d.Text + "\n(Mình chưa lấy thêm được dữ liệu, nên không đưa số liệu chưa kiểm chứng.)")
	}
	if d.Secret != nil {
		sec := *d.Secret
		s.update(w.RequestID, func(r *ledger.DirectRun) { r.Secret = &sec })
		s.record(w.BotID, w.RequestID, "secret", "Xin bí mật "+sec.Name)
	}
	if len(d.Plan) < 2 || d.Approval != nil {
		return d.Approval, d.Choices, s.appendText(w.RequestID, d.Text)
	}
	steps := make([]Step, len(d.Plan))
	for i, t := range d.Plan {
		steps[i] = Step{Title: t, Status: "pending"}
	}
	s.update(w.RequestID, func(r *ledger.DirectRun) { r.PlanIntro, r.Plan = clean(d.Text, 600), steps })
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
	for i, title := range d.Plan {
		if ctx.Err() != nil {
			setStep(i, "cancelled")
			return nil, nil, ctx.Err()
		}
		setStep(i, "running")
		s.agui(AGUIEvent{Type: STEP_STARTED, ThreadID: w.BotID, RunID: w.RequestID, StepName: title})
		msgs := append(tc.inject(s.history(w.BotID, w.RequestID)), ChatMsg{"user", fmt.Sprintf("[Hệ thống] Thực hiện bước %d/%d: %s. Chỉ làm bước này, trả lời ngắn. Không viết PLAN. Đừng tự tạo bước chờ Darren duyệt: việc duyệt do hệ thống lo. Nếu thật sự thiếu thông tin chỉ Darren biết, hỏi 1 câu và dùng CHOICES, kế hoạch sẽ tạm dừng. Chỉ dùng APPROVAL nếu bước này cần hành động ra bên ngoài.", i+1, len(d.Plan), title)})
		sd, err := s.call(ctx, w, msgs)
		if err != nil {
			st := "failed"
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				st = "cancelled"
			}
			setStep(i, st)
			return nil, nil, err
		}
		if err := s.appendText(w.RequestID, sd.Text); err != nil {
			return nil, nil, err
		}
		s.agui(AGUIEvent{Type: STEP_FINISHED, ThreadID: w.BotID, RunID: w.RequestID, StepName: title})
		if sd.Approval != nil || len(sd.Choices) > 0 {
			setStep(i, "waiting")
			s.endDraft(w)
			return sd.Approval, sd.Choices, nil
		}
		setStep(i, "done")
		s.endDraft(w)
	}
	return nil, nil, nil
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
