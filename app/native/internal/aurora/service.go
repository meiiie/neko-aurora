package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/outbound"
	"nekoaurora.local/native/internal/vault"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Service is the mobile (/app/) API over the control-plane ledger. It owns
// no bot, message or task model: agents are ledger bots, messages are ledger
// work items, plans and approvals are journaled on that work.
type Service struct {
	L     *ledger.Store
	AI    Model
	Push  *Pusher
	Out   *outbound.Router
	Prefs *Prefs
	V     *vault.Vault
	mux   *http.ServeMux
	mu    sync.Mutex
	subs  map[chan Event]struct{}
	ctx   context.Context
	wake  chan struct{}

	running map[string]bool // botID → turn in flight
	seen    map[string]string
	seenSeq uint64
	syncMu  sync.Mutex
	briefM  sync.Mutex
	// Now is overridable in tests.
	Now func() time.Time
}

type Options struct {
	// RunTurns lets this service execute queued owner→bot work. Disable it
	// when the attested host/executor runtime is configured instead.
	RunTurns bool
}

func New(ctx context.Context, store *ledger.Store, stateDir string, opt Options) (*Service, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, err
	}
	prefs := &Prefs{L: store}
	if !store.ReadOnly() {
		if err := importPrefsFile(prefs, stateDir); err != nil {
			log.Printf("aurora prefs import failed: %v", err)
		}
	}
	p, err := LoadPusher(stateDir)
	if err != nil {
		return nil, err
	}
	var model Model = AIFromEnv()
	if Env("AI_BASE_URL") == "" {
		if c := CoreFromEnv(stateDir); c != nil {
			model = c
		}
	}
	v, err := vault.Open(stateDir)
	if err != nil {
		return nil, err
	}
	log.SetOutput(v.Writer(os.Stderr))
	s := &Service{L: store, AI: model, Push: p, V: v, Prefs: prefs, subs: map[chan Event]struct{}{}, ctx: ctx, wake: make(chan struct{}, 1), running: map[string]bool{}, seen: map[string]string{}, Now: time.Now}
	s.Out = outbound.NewRouter(outbound.Switch{Resolve: s.mailer})
	s.routes()
	s.connectorRoutes()
	s.sync()
	go s.watch()
	if opt.RunTurns && !store.ReadOnly() {
		s.recoverInterrupted()
		go s.runLoop()
		if Env("BRIEF_LOOP") != "off" {
			go s.briefLoop()
		}
	}
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

// watch turns ledger changes (from this API, the desktop or the runner)
// into agent/message events by diffing projected views.
func (s *Service) watch() {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.sync()
		}
	}
}

func (s *Service) sync() {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	snap := s.L.Snapshot()
	if snap.Sequence == s.seenSeq {
		return
	}
	first := len(s.seen) == 0
	s.seenSeq = snap.Sequence
	v := newView(snap)
	next := map[string]string{}
	for _, b := range snap.Bots {
		if v.archived[b.ID] {
			next["a:"+b.ID] = "archived"
			if old, ok := s.seen["a:"+b.ID]; ok && old != "archived" && !first {
				s.publish("archived", map[string]string{"id": b.ID})
			}
			continue
		}
		a := v.agent(b, s.Prefs.Bot(b.ID))
		j, _ := json.Marshal(a)
		next["a:"+b.ID] = string(j)
		if s.seen["a:"+b.ID] != string(j) && !first {
			s.publish("agent", a)
		}
		for _, m := range v.messages(b.ID, 0) {
			j, _ := json.Marshal(m)
			next["m:"+m.ID] = string(j)
			if s.seen["m:"+m.ID] != string(j) && !first {
				s.publish("message", m)
			}
		}
	}
	for _, c := range v.groups() {
		a := v.group(c, s.Prefs.GroupPinned(c.ID))
		j, _ := json.Marshal(a)
		next["a:"+c.ID] = string(j)
		if s.seen["a:"+c.ID] != string(j) && !first {
			s.publish("agent", a)
		}
		for _, m := range v.groupMessages(c.ID, 0) {
			j, _ := json.Marshal(m)
			next["m:"+m.ID] = string(j)
			if s.seen["m:"+m.ID] != string(j) && !first {
				s.publish("message", m)
			}
		}
	}
	if br := v.brief(); br != nil {
		j, _ := json.Marshal(br)
		next["brief"] = string(j)
		if s.seen["brief"] != string(j) && !first {
			s.publish("brief", br)
		}
	}
	for _, a := range s.activity(5) {
		k := "act:" + a.ID
		next[k] = "1"
		if s.seen[k] == "" && !first {
			s.publish("activity", a)
		}
	}
	s.seen = next
}

func (s *Service) resync() {
	s.syncMu.Lock()
	s.seenSeq = ^uint64(0)
	s.syncMu.Unlock()
	s.sync()
}

func (s *Service) view() *view { return newView(s.L.Snapshot()) }

func (s *Service) botName(id string) string {
	if b, ok := s.view().bot(id); ok {
		return b.Name
	}
	return "Neko"
}

func (s *Service) notify(title, body, url string) {
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body, "url": url})
	s.publish("notify", json.RawMessage(payload))
	for _, sub := range s.Prefs.PushSubs() {
		go func(sub PushSub) {
			gone, err := s.Push.Send(sub, payload)
			if gone {
				s.Prefs.DropPush(sub.Endpoint)
			} else if err != nil {
				log.Printf("aurora push failed: %v", err)
			}
		}(sub)
	}
}

// ---------- morning brief (a ledger task on the brief bot) ----------

func (s *Service) briefLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		now := s.Now()
		due := time.Date(now.Year(), now.Month(), now.Day(), 5, 30, 0, 0, now.Location())
		b := s.view().brief()
		if b == nil || (now.After(due) && b.Generated.Before(due)) {
			_, _ = s.RequestBrief()
		}
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) briefBot(v *view) (ledger.Bot, bool) {
	var first *ledger.Bot
	for i, b := range v.snap.Bots {
		if v.archived[b.ID] {
			continue
		}
		if strings.Contains(strings.ToLower(b.Name), "brief") || strings.Contains(strings.ToLower(b.Name), "bản tin") {
			return b, true
		}
		if first == nil {
			first = &v.snap.Bots[i]
		}
	}
	if first == nil {
		return ledger.Bot{}, false
	}
	return *first, true
}

// RequestBrief queues a brief task; the runner fills it.
func (s *Service) RequestBrief() (ledger.Work, error) {
	s.briefM.Lock()
	defer s.briefM.Unlock()
	v := s.view()
	for _, w := range v.snap.Work {
		if strings.HasPrefix(w.ClientNonce, "brief-") && active(w) {
			return w, nil
		}
	}
	b, ok := s.briefBot(v)
	if !ok {
		return ledger.Work{}, ErrNotFound
	}
	w, _, err := s.L.Accept(ledger.Input{BotID: b.ID, ClientNonce: newID("brief-"), Text: "[Bản tin sáng] Tóm tắt trạng thái các agent và việc cần Darren."})
	s.poke()
	return w, err
}

func (s *Service) runBrief(w ledger.Work) {
	v := s.view()
	var sb strings.Builder
	n, pend := 0, v.pending()
	for _, b := range v.snap.Bots {
		if v.archived[b.ID] {
			continue
		}
		n++
		a := v.agent(b, s.Prefs.Bot(b.ID))
		fmt.Fprintf(&sb, "- %s [%s]: %s\n", a.Name, a.Status, a.Snip)
	}
	sb.WriteString("Đang chờ duyệt:\n")
	items := []string{}
	for _, p := range pend {
		fmt.Fprintf(&sb, "- %s: %s\n", s.botName(p.AgentID), p.Approval.Title)
		items = append(items, clean(s.botName(p.AgentID)+": "+p.Approval.Title, 160))
	}
	sb.WriteString("Hoạt động gần đây:\n")
	for _, a := range s.activity(15) {
		fmt.Fprintf(&sb, "- %s\n", a.Text)
	}
	text := fmt.Sprintf("Chào Darren. %d agent đang hoạt động, %d việc cần bạn.", n, len(pend))
	run := ledger.DirectRun{Runtime: s.AI.Runtime(), Kind: "brief"}
	if s.AI.Configured() {
		c, cancel := context.WithTimeout(s.ctx, 90*time.Second)
		out, err := s.AI.Stream(c, []ChatMsg{
			{"system", `Bạn viết bản tin sáng cho Darren từ trạng thái các agent. Trả về DUY NHẤT JSON: {"text":"1-2 câu chào và tóm tắt","items":["tối đa 5 dòng ngắn, ưu tiên việc cần Darren duyệt"]}. Tiếng Việt. Không bịa dữ kiện ngoài dữ liệu.`},
			{"user", sb.String()},
		}, func(string) {})
		cancel()
		if err == nil {
			var j struct {
				Text  string   `json:"text"`
				Items []string `json:"items"`
			}
			if i := strings.Index(out, "{"); i >= 0 {
				out = out[i:]
			}
			if k := strings.LastIndex(out, "}"); k >= 0 {
				out = out[:k+1]
			}
			if json.Unmarshal([]byte(out), &j) == nil && j.Text != "" {
				text, run.Model = clean(j.Text, 400), s.AI.ModelName()
				items = []string{}
				for _, it := range j.Items {
					if it = clean(it, 160); it != "" && len(items) < 5 {
						items = append(items, it)
					}
				}
			}
		} else {
			log.Printf("aurora brief ai failed: %v", err)
		}
	}
	if len(items) > 5 {
		items = items[:5]
	}
	w, err := s.L.DirectStart(w.RequestID, w.Epoch, run)
	if err != nil {
		return
	}
	_ = s.L.AppendText(w.RequestID, w.Epoch, text)
	_, _ = s.L.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *ledger.DirectRun) { d.Items = items })
	s.record(w.BotID, w.RequestID, "reply", "Đã tạo bản tin sáng")
	s.sync()
	s.notify("Bản tin sáng", text, "/app/")
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
	case errors.Is(err, ErrNotFound), errors.Is(err, ledger.ErrNotFound):
		code = 404
	case errors.Is(err, ErrInvalid), errors.Is(err, ledger.ErrInvalid):
		code = 400
	case errors.Is(err, ErrConflict), errors.Is(err, ledger.ErrConflict):
		code = 409
	case errors.Is(err, ledger.ErrReadOnly):
		code = 423
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

// alias derives a ledger alias (^[a-z][a-z0-9_-]{1,31}$) from a display name.
func alias(name string, taken map[string]bool) string {
	repl := strings.NewReplacer("đ", "d", "Đ", "d")
	name = repl.Replace(name)
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		r = foldVietnamese(r)
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		base = "agent-" + base
	}
	if len(base) > 26 {
		base = strings.Trim(base[:26], "-")
	}
	if len(base) < 2 {
		base += "-x"
	}
	out := base
	for i := 2; taken[out]; i++ {
		out = base + "-" + strconv.Itoa(i)
	}
	return out
}

func foldVietnamese(r rune) rune {
	const from = "àáạảãâầấậẩẫăằắặẳẵèéẹẻẽêềếệểễìíịỉĩòóọỏõôồốộổỗơờớợởỡùúụủũưừứựửữỳýỵỷỹ"
	const to = "aaaaaaaaaaaaaaaaaeeeeeeeeeeeiiiiiooooooooooooooooouuuuuuuuuuuyyyyy"
	i := 0
	for _, f := range from {
		if f == r {
			return rune(to[i])
		}
		i++
	}
	return r
}

// MaxBots mirrors the ledger's cap on bots (archived ones still count).
const MaxBots = 32

var ErrBotLimit = fmt.Errorf("%w: bot_limit", ErrConflict)

func (s *Service) createBot(name, desc, nonce string) (ledger.Bot, error) {
	snap := s.L.Snapshot()
	if len(snap.Bots) >= MaxBots {
		return ledger.Bot{}, ErrBotLimit
	}
	taken, names := map[string]bool{}, map[string]bool{}
	for _, b := range snap.Bots {
		taken[b.Alias] = true
		names[strings.ToLower(b.Name)] = true
	}
	n := name
	for i := 2; names[strings.ToLower(n)]; i++ {
		n = fmt.Sprintf("%s %d", name, i)
	}
	if len([]rune(desc)) > 240 {
		desc = string([]rune(desc)[:240])
	}
	b, _, err := s.L.CreateBot(ledger.BotInput{Name: n, Alias: alias(name, taken), Description: desc, ClientNonce: nonce})
	return b, err
}

func (s *Service) routes() {
	m := http.NewServeMux()
	m.HandleFunc("GET /app/api/state", func(w http.ResponseWriter, r *http.Request) {
		v := s.view()
		agents := []*Agent{}
		for _, b := range v.snap.Bots {
			if !v.archived[b.ID] {
				agents = append(agents, v.agent(b, s.Prefs.Bot(b.ID)))
			}
		}
		for _, c := range v.groups() {
			agents = append(agents, v.group(c, s.Prefs.GroupPinned(c.ID)))
		}
		writeJSON(w, 200, map[string]any{
			"agents": agents, "pending": v.pending(), "brief": v.brief(),
			"ai":             map[string]any{"configured": s.AI.Configured(), "model": s.AI.ModelName(), "provider": "z.ai", "runtime": s.AI.Runtime()},
			"vapidPublicKey": s.Push.Pub, "pushSubscriptions": len(s.Prefs.PushSubs()),
			"outbound": s.outboundInfo(), "secrets": s.secretNames(),
		})
	})
	m.HandleFunc("POST /app/api/agents", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name  string `json:"name"`
			Desc  string `json:"desc"`
			Face  Face   `json:"av"`
			Nonce string `json:"clientNonce"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		name, desc := clean(in.Name, 28), clean(in.Desc, 240)
		av, okAv := avatarOf(in.Face)
		if name == "" || !okAv {
			fail(w, ErrInvalid)
			return
		}
		nonce := in.Nonce
		if len(nonce) < 8 {
			nonce = newID("aurora-")
		}
		b, err := s.createBot(name, desc, nonce)
		if err != nil {
			fail(w, err)
			return
		}
		_ = s.Prefs.SetBot(b.ID, func(p *botPrefs) { p.Avatar = &av })
		s.resync()
		writeJSON(w, 201, s.view().agent(b, s.Prefs.Bot(b.ID)))
	})
	m.HandleFunc("POST /app/api/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Name     *string `json:"name"`
			Desc     *string `json:"desc"`
			Face     *Face   `json:"av"`
			Pinned   *bool   `json:"pin"`
			Archived *bool   `json:"archived"`
		}
		if err := decode(r, &p); err != nil {
			fail(w, err)
			return
		}
		id := r.PathValue("id")
		v := s.view()
		b, ok := v.bot(id)
		if !ok {
			fail(w, ErrNotFound)
			return
		}
		var newAv *Avatar
		if p.Face != nil {
			a, ok := avatarOf(*p.Face)
			if !ok {
				fail(w, ErrInvalid)
				return
			}
			newAv = &a
		}
		if p.Name != nil {
			n := clean(*p.Name, 28)
			if n == "" {
				fail(w, ErrInvalid)
				return
			}
			if n != b.Name {
				if _, _, err := s.L.RenameBot(id, ledger.BotNameInput{Name: n, ExpectedDisplayRevision: b.DisplayRevision, ClientNonce: newID("rn-")}); err != nil {
					fail(w, err)
					return
				}
			}
		}
		if p.Desc != nil {
			prof := v.profiles[id]
			if _, _, err := s.L.SetBotProfile(id, ledger.BotProfileInput{Instructions: clean(*p.Desc, 4000), IncludeHistory: prof.IncludeHistory || prof.Revision == 0, ExpectedRevision: prof.Revision, ClientNonce: newID("pf-")}); err != nil {
				fail(w, err)
				return
			}
		}
		if newAv != nil || p.Pinned != nil {
			_ = s.Prefs.SetBot(id, func(x *botPrefs) {
				if newAv != nil {
					x.Avatar = newAv
				}
				if p.Pinned != nil {
					x.Pinned = *p.Pinned
				}
			})
			s.resync()
		}
		if p.Archived != nil && *p.Archived {
			st := ledger.BotState{}
			for _, x := range v.snap.BotStates {
				if x.BotID == id {
					st = x
				}
			}
			if _, _, err := s.L.ControlBot(ledger.BotControlInput{BotID: id, Action: "archive", ExpectedRevision: st.Revision, ClientNonce: newID("ar-")}); err != nil {
				fail(w, err)
				return
			}
			s.sync()
			writeJSON(w, 200, map[string]any{"id": id, "archived": true})
			return
		}
		s.sync()
		v = s.view()
		b, _ = v.bot(id)
		writeJSON(w, 200, v.agent(b, s.Prefs.Bot(id)))
	})
	m.HandleFunc("GET /app/api/agents/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		v := s.view()
		id := r.PathValue("id")
		if _, ok := v.channels[id]; ok {
			writeJSON(w, 200, v.groupMessages(id, 200))
			return
		}
		if _, ok := v.bot(id); !ok {
			fail(w, ErrNotFound)
			return
		}
		writeJSON(w, 200, v.messages(id, 200))
	})
	m.HandleFunc("POST /app/api/agents/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Text  string `json:"text"`
			Nonce string `json:"clientNonce"`
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
		nonce := in.Nonce
		if len(nonce) < 8 || len(nonce) > 100 {
			nonce = newID("aurora-")
		}
		if isGroupID(id) {
			msg, err := s.sendGroup(id, text, nonce)
			if err != nil {
				fail(w, err)
				return
			}
			s.sync()
			s.poke()
			writeJSON(w, 202, msg)
			return
		}
		if _, ok := s.view().bot(id); !ok {
			fail(w, ErrNotFound)
			return
		}
		// steering: a new instruction supersedes any stale draft awaiting approval
		s.supersede(id)
		work, _, err := s.L.Accept(ledger.Input{BotID: id, ClientNonce: nonce, Text: text})
		if err != nil {
			fail(w, err)
			return
		}
		s.sync()
		s.poke()
		for _, msg := range workMessages(work, false) {
			if msg.Role == "user" {
				msg.Queued = s.isRunning(id)
				writeJSON(w, 202, msg)
				return
			}
		}
		writeJSON(w, 202, map[string]string{"taskId": work.RequestID})
	})
	m.HandleFunc("POST /app/api/approvals/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Decision string  `json:"decision"`
			Body     *string `json:"body"`
			Subject  *string `json:"subject"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		if in.Decision != "approved" && in.Decision != "declined" {
			fail(w, ErrInvalid)
			return
		}
		id := strings.TrimSuffix(r.PathValue("id"), ":plan")
		work, err := s.L.DirectDecide(id, func(a *ledger.DirectApproval, d *ledger.DirectRun) error {
			if a.State != "pending" {
				return ErrConflict
			}
			// edit-before-approve: the owner's edits replace the draft atomically
			if in.Body != nil {
				b := clean(*in.Body, 4000)
				if b == "" {
					return ErrInvalid
				}
				a.Body = b
			}
			if in.Subject != nil && a.Kind == "email" {
				a.Subject = clean(*in.Subject, 160)
			}
			a.State, a.DecidedAt = in.Decision, time.Now().UTC().Format(time.RFC3339Nano)
			switch {
			case in.Decision == "declined":
				a.Outcome = "Ok, mình giữ lại “" + a.Title + "” và không làm gì thêm."
			case !s.hasChannel(a.Kind) || a.To == "":
				a.Outcome = "Đã ghi nhận bạn duyệt “" + a.Title + "”. Kênh gửi ra ngoài chưa được kết nối nên mình chưa gửi gì; bản đã duyệt được lưu để gửi khi kênh sẵn sàng."
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, ledger.ErrConflict) {
				err = ErrConflict
			}
			fail(w, err)
			return
		}
		{
			ap := work.Direct.Approval
			verb := map[string]string{"approved": "Bạn duyệt", "declined": "Bạn hoãn"}[ap.State]
			s.record(work.BotID, work.RequestID, "decision", verb+" “"+ap.Title+"” của "+s.botName(work.BotID))
		}
		if ap := work.Direct.Approval; in.Decision == "approved" && s.hasChannel(ap.Kind) && ap.To != "" {
			work = s.deliver(work)
		}
		s.sync()
		for _, msg := range workMessages(work, false) {
			if msg.Approval != nil {
				writeJSON(w, 200, msg)
				return
			}
		}
		writeJSON(w, 200, map[string]string{"taskId": work.RequestID})
	})
	m.HandleFunc("POST /app/api/groups", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string   `json:"name"`
			Bots []string `json:"bots"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		c, err := s.createGroup(in.Name, in.Bots)
		if err != nil {
			fail(w, err)
			return
		}
		s.record(c.BotIDs[0], "", "agent", "Bạn tạo nhóm "+c.Name)
		s.resync()
		writeJSON(w, 201, s.view().group(c, false))
	})
	m.HandleFunc("POST /app/api/agents/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"stopped": s.Stop(r.PathValue("id"))})
	})
	m.HandleFunc("GET /app/api/outbox", func(w http.ResponseWriter, r *http.Request) {
		v := s.view()
		items := []*Message{}
		for _, wk := range v.snap.Work {
			if wk.Direct != nil && wk.Direct.Approval != nil && wk.Direct.Approval.Kind == "email" {
				for _, msg := range workMessages(wk, false) {
					if msg.Approval != nil {
						items = append(items, msg)
					}
				}
			}
		}
		mail, _ := s.Out.Adapter("email")
		writeJSON(w, 200, map[string]any{"email": map[string]any{"configured": mail.Configured(), "from": mail.Sender()}, "items": items})
	})
	m.HandleFunc("GET /app/api/activity", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if n <= 0 || n > 200 {
			n = 60
		}
		writeJSON(w, 200, s.activity(n))
	})
	m.HandleFunc("POST /app/api/brief", func(w http.ResponseWriter, r *http.Request) {
		work, err := s.RequestBrief()
		if err != nil {
			fail(w, err)
			return
		}
		deadline := time.Now().Add(100 * time.Second)
		for time.Now().Before(deadline) {
			cur, err := s.L.Get(work.RequestID)
			if err == nil && !active(cur) {
				break
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(150 * time.Millisecond):
			}
		}
		s.sync()
		writeJSON(w, 200, s.view().brief())
	})
	m.HandleFunc("POST /app/api/push/subscribe", func(w http.ResponseWriter, r *http.Request) {
		var p PushSub
		if err := decode(r, &p); err != nil {
			fail(w, err)
			return
		}
		if err := s.Prefs.AddPush(p); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("POST /app/api/push/test", func(w http.ResponseWriter, r *http.Request) {
		s.notify("Neko Aurora", "Thông báo thử: kênh thông báo đang hoạt động.", "/app/")
		writeJSON(w, 200, map[string]int{"subscriptions": len(s.Prefs.PushSubs())})
	})
	m.HandleFunc("GET /app/api/events", s.events)
	s.mux = m
}

func (s *Service) isRunning(botID string) bool {
	for _, w := range s.view().thread(botID) {
		if w.Status == "running" || w.Status == "starting" {
			return true
		}
	}
	return false
}

// Stop cancels this bot's running and queued tasks via the ledger's task
// control (the same Cancel the desktop uses).
func (s *Service) Stop(botID string) bool {
	stopped := false
	for _, w := range s.view().thread(botID) {
		if w.Status == "running" || w.Status == "waiting_host" || w.Status == "starting" {
			if _, err := s.L.Cancel(w.RequestID); err == nil {
				stopped = true
			}
		}
	}
	if stopped && !isGroupID(botID) {
		s.record(botID, "", "stop", "Bạn dừng "+s.botName(botID))
	}
	s.sync()
	return stopped
}

func (s *Service) supersede(botID string) {
	for _, w := range s.view().thread(botID) {
		if w.Direct != nil && w.Direct.Approval != nil && w.Direct.Approval.State == "pending" {
			_, _ = s.L.DirectDecide(w.RequestID, func(a *ledger.DirectApproval, d *ledger.DirectRun) error {
				a.State, a.DecidedAt = "superseded", time.Now().UTC().Format(time.RFC3339Nano)
				return nil
			})
			s.record(w.BotID, w.RequestID, "decision", "Thay thế bản nháp “"+w.Direct.Approval.Title+"” của "+s.botName(w.BotID))
		}
	}
}

// userMentioned reports whether the owner typed needle in this bot's thread.
func (s *Service) userMentioned(botID, needle string) bool {
	n := strings.ToLower(needle)
	for _, w := range s.view().allBot[botID] {
		if (w.Direct == nil || w.Direct.Kind == "") && strings.Contains(strings.ToLower(w.Text), n) {
			return true
		}
	}
	return false
}

func (s *Service) setDelivery(id string, d ledger.DirectDelivery, outcome string) ledger.Work {
	d.At = time.Now().UTC().Format(time.RFC3339Nano)
	w, err := s.L.DirectDecide(id, func(a *ledger.DirectApproval, _ *ledger.DirectRun) error {
		a.Delivery, a.Outcome = &d, outcome
		return nil
	})
	if err != nil {
		w, _ = s.L.Get(id)
	}
	return w
}

func (s *Service) hasChannel(kind string) bool { _, ok := s.Out.Adapter(kind); return ok }

// deliver hands an approved card to the outbound layer. It runs only after
// the owner's approval was journaled; the router refuses any recipient the
// owner did not type in this thread, so model output alone never picks one.
func (s *Service) deliver(w ledger.Work) ledger.Work {
	ap := *w.Direct.Approval
	req := outbound.Request{Kind: ap.Kind, To: ap.To, Subject: ap.Subject, Title: ap.Title, Body: ap.Body}
	a, err := s.Out.Check(req, func(n string) bool { return s.userMentioned(w.BotID, n) })
	switch {
	case errors.Is(err, outbound.ErrRecipientGuard):
		s.record(w.BotID, w.RequestID, "delivery", "Chặn gửi tới "+ap.To+": địa chỉ không do bạn nhập")
		return s.setDelivery(w.RequestID, ledger.DirectDelivery{Status: "failed", Detail: "recipient_not_from_owner"}, "Mình không gửi: địa chỉ "+ap.To+" không phải do bạn nhập trong hội thoại này.")
	case errors.Is(err, outbound.ErrNotConfigured):
		s.record(w.BotID, w.RequestID, "delivery", "“"+ap.Title+"” chờ trong hàng đợi: kênh "+ap.Kind+" chưa cấu hình")
		return s.setDelivery(w.RequestID, ledger.DirectDelivery{Status: "queued", Detail: ap.Kind + "_channel_not_configured"}, "Đã ghi nhận bạn duyệt. Kênh "+ap.Kind+" của máy chủ chưa được cấu hình (AURORA_SMTP_*), nên thư tới "+ap.To+" đang nằm trong hàng đợi, chưa gửi.")
	case err != nil:
		return s.setDelivery(w.RequestID, ledger.DirectDelivery{Status: "failed", Detail: clean(err.Error(), 160)}, "Mình không gửi được “"+ap.Title+"”: "+clean(err.Error(), 160))
	}
	out := s.setDelivery(w.RequestID, ledger.DirectDelivery{Status: "sending", From: a.Sender()}, "Đã ghi nhận bạn duyệt. Đang gửi tới "+ap.To+"…")
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
		defer cancel()
		err := a.Deliver(ctx, req)
		d := ledger.DirectDelivery{Status: "sent", From: a.Sender()}
		text := "Đã gửi tới " + ap.To + " từ " + a.Sender() + "."
		if err != nil {
			d = ledger.DirectDelivery{Status: "failed", From: a.Sender(), Detail: clean(err.Error(), 160)}
			text = "Gửi tới " + ap.To + " thất bại: " + d.Detail
		}
		s.setDelivery(w.RequestID, d, text)
		s.record(w.BotID, w.RequestID, "delivery", text)
		s.sync()
		s.notify(ap.Kind, text, "/app/#"+w.BotID)
	}()
	return out
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
