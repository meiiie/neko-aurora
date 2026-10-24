package aurora

// Schedules (idea from DS Bot): the owner asks a bot to do something at a
// time ("mỗi sáng 8h tóm tắt tin vàng"), the bot answers with a SCHEDULE
// line, and at that time the request runs as an ordinary turn of that bot
// (push notification included). Times are Vietnam time. One occurrence is
// one ledger nonce, so a restart never runs it twice.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

const maxSchedules = 12

type schedule struct {
	ID     string `json:"id"`
	Bot    string `json:"bot"`
	Text   string `json:"text"`
	At     string `json:"at"`             // HH:MM, Vietnam time
	Repeat string `json:"repeat"`         // daily | weekdays | once
	Date   string `json:"date,omitempty"` // once: YYYY-MM-DD
	Last   string `json:"last,omitempty"` // last occurrence key run
}

type schedStore struct {
	mu   sync.Mutex
	path string
	L    []schedule
}

func openSchedules(dir string) *schedStore {
	st := &schedStore{path: filepath.Join(dir, "schedules.json")}
	if b, err := os.ReadFile(st.path); err == nil {
		_ = json.Unmarshal(b, &st.L)
	}
	return st
}

func (st *schedStore) saveLocked() {
	b, _ := json.Marshal(st.L)
	tmp := st.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, st.path)
	}
}

var hhmm = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

// normalize validates a schedule from a directive; ok=false drops it.
func (x schedule) normalize(now time.Time) (schedule, bool) {
	x.Text = clean(x.Text, 1000)
	m := hhmm.FindStringSubmatch(strings.TrimSpace(x.At))
	if x.Text == "" || m == nil {
		return x, false
	}
	if len(m[1]) == 1 {
		m[1] = "0" + m[1]
	}
	x.At = m[1] + ":" + m[2]
	switch x.Repeat {
	case "daily", "weekdays":
		x.Date = ""
	default:
		x.Repeat = "once"
		t := now.In(saigon)
		today := t.Format("2006-01-02")
		if _, err := time.ParseInLocation("2006-01-02", x.Date, saigon); err != nil || x.Date < today {
			x.Date = today
		}
		if x.Date == today && t.Format("15:04") >= x.At {
			// a time already past today means the next one (never fire at once)
			x.Date = t.AddDate(0, 0, 1).Format("2006-01-02")
		}
	}
	return x, true
}

// due: the occurrence key to run now, or "".
func (x schedule) due(now time.Time) string {
	t := now.In(saigon)
	day := t.Format("2006-01-02")
	if x.Repeat == "once" {
		switch {
		case x.Last != "" || day < x.Date:
			return ""
		case day > x.Date: // missed while down: run once, late
			return x.Date
		case t.Format("15:04") < x.At:
			return ""
		}
		return day
	}
	if t.Format("15:04") < x.At {
		return ""
	}
	switch x.Repeat {
	case "weekdays":
		if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
			return ""
		}
	}
	if x.Last == day {
		return ""
	}
	return day
}

func (st *schedStore) add(x schedule) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.L) >= maxSchedules {
		return false
	}
	for _, y := range st.L {
		if y.Bot == x.Bot && y.At == x.At && y.Repeat == x.Repeat && strings.EqualFold(y.Text, x.Text) {
			return false
		}
	}
	st.L = append(st.L, x)
	st.saveLocked()
	return true
}

func (st *schedStore) list(bot string) []schedule {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := []schedule{}
	for _, x := range st.L {
		if bot == "" || x.Bot == bot {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

func (st *schedStore) remove(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, x := range st.L {
		if x.ID == id {
			st.L = append(st.L[:i], st.L[i+1:]...)
			st.saveLocked()
			return true
		}
	}
	return false
}

// tick runs every due occurrence; once-schedules are removed after running.
func (s *Service) scheduleTick(now time.Time) {
	st := s.sched
	st.mu.Lock()
	type job struct {
		x   schedule
		key string
	}
	var jobs []job
	for i := range st.L {
		if k := st.L[i].due(now); k != "" {
			st.L[i].Last = k
			jobs = append(jobs, job{st.L[i], k})
		}
	}
	keep := st.L[:0]
	for _, x := range st.L {
		if !(x.Repeat == "once" && x.Last != "") {
			keep = append(keep, x)
		}
	}
	st.L = keep
	if len(jobs) > 0 {
		st.saveLocked()
	}
	st.mu.Unlock()
	for _, j := range jobs {
		text := "[Lịch hẹn " + j.x.At + "] " + j.x.Text
		if _, _, err := s.L.Accept(ledger.Input{BotID: j.x.Bot, ClientNonce: "sched-" + j.x.ID + "-" + j.key, Text: text}); err == nil {
			s.record(j.x.Bot, "", "step", "Chạy lịch hẹn "+j.x.At+": "+clean(j.x.Text, 80))
		}
	}
	if len(jobs) > 0 {
		s.poke()
	}
}

func (s *Service) scheduleLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-t.C:
			s.scheduleTick(now)
		}
	}
}

// addSchedules stores a turn's SCHEDULE lines.
func (s *Service) addSchedules(w ledger.Work, xs []schedule) {
	for _, x := range xs {
		x.Bot, x.ID = w.BotID, newID("s-")
		if n, ok := x.normalize(s.Now()); ok && s.sched.add(n) {
			when := map[string]string{"daily": "mỗi ngày", "weekdays": "các ngày trong tuần", "once": "ngày " + n.Date}[n.Repeat]
			s.record(w.BotID, w.RequestID, "step", "Đã đặt lịch "+n.At+" "+when+": "+clean(n.Text, 80))
		}
	}
}

func (s *Service) scheduleRoutes() {
	s.mux.HandleFunc("GET /app/api/schedules", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"schedules": s.sched.list(r.URL.Query().Get("bot"))})
	})
	s.mux.HandleFunc("POST /app/api/schedules/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		if !s.sched.remove(r.PathValue("id")) {
			fail(w, ErrNotFound)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
