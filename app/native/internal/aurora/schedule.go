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
	"unicode/utf8"

	"nekoaurora.local/native/internal/ledger"
)

const maxSchedules = 12

type schedule struct {
	ID        string `json:"id"`
	Bot       string `json:"bot"`
	Text      string `json:"text"`
	At        string `json:"at"`             // HH:MM, Vietnam time
	Repeat    string `json:"repeat"`         // daily | weekdays | once
	Date      string `json:"date,omitempty"` // once: YYYY-MM-DD
	Last      string `json:"last,omitempty"` // last occurrence key run
	Paused    bool   `json:"paused"`
	Revision  uint64 `json:"revision"`
	StartDate string `json:"startDate,omitempty"` // earliest day after an owner edit
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

func (st *schedStore) saveLocked() error {
	b, _ := json.Marshal(st.L)
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

var hhmm = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

// normalize validates a schedule from a directive; ok=false drops it.
func (x schedule) normalize(now time.Time) (schedule, bool) {
	x.StartDate = ""
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
	if x.Paused {
		return ""
	}
	t := now.In(saigon)
	day := t.Format("2006-01-02")
	if x.StartDate != "" && day < x.StartDate {
		return ""
	}
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
	x.Revision = 1
	st.L = append(st.L, x)
	if st.saveLocked() != nil {
		st.L = st.L[:len(st.L)-1]
		return false
	}
	return true
}

func (st *schedStore) list(bot string) []schedule {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := []schedule{}
	for _, x := range st.L {
		if bot == "" || x.Bot == bot {
			x.Revision = max(1, x.Revision)
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
	defer st.mu.Unlock()
	changed := false
	for i := range st.L {
		if k := st.L[i].due(now); k != "" {
			x := st.L[i]
			text := "[Lịch hẹn " + x.At + "] " + x.Text
			if _, _, err := s.L.Accept(ledger.Input{BotID: x.Bot, ClientNonce: "sched-" + x.ID + "-" + k, Text: text}); err != nil {
				continue
			}
			st.L[i].Last = k
			changed = true
		}
	}
	keep := st.L[:0]
	for _, x := range st.L {
		if !(x.Repeat == "once" && x.Last != "") {
			keep = append(keep, x)
		}
	}
	st.L = keep
	if changed {
		st.saveLocked()
		s.poke()
	}
}

func (s *Service) controlSchedule(id string, revision uint64, paused *bool) error {
	if s.L.ReadOnly() {
		return ledger.ErrConflict
	}
	st := s.sched
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, x := range st.L {
		if x.ID != id {
			continue
		}
		if revision != max(1, x.Revision) {
			return ledger.ErrConflict
		}
		old := append([]schedule(nil), st.L...)
		if paused == nil {
			st.L = append(st.L[:i], st.L[i+1:]...)
		} else {
			st.L[i].Paused = *paused
			st.L[i].Revision = max(1, x.Revision) + 1
		}
		if err := st.saveLocked(); err != nil {
			st.L = old
			return err
		}
		if paused == nil || *paused {
			for _, w := range s.L.Snapshot().Work {
				if w.BotID == x.Bot && strings.HasPrefix(w.ClientNonce, "sched-"+id+"-") && w.Status == "waiting_host" {
					if _, err := s.L.CancelWaiting(w.RequestID); err != nil && err != ledger.ErrConflict {
						return err
					}
				}
			}
		}
		return nil
	}
	return ErrNotFound
}

type scheduleEdit struct {
	WorkspaceID string `json:"workspaceId"`
	Revision    uint64 `json:"revision"`
	Text        string `json:"text"`
	At          string `json:"at"`
	Repeat      string `json:"repeat"`
	Date        string `json:"date"`
}

func (s *Service) editSchedule(id string, in scheduleEdit) (schedule, error) {
	if s.L.ReadOnly() {
		return schedule{}, ledger.ErrConflict
	}
	now := s.Now().In(saigon)
	day := now.Format("2006-01-02")
	m := hhmm.FindStringSubmatch(strings.TrimSpace(in.At))
	if m == nil || strings.TrimSpace(in.Text) == "" || utf8.RuneCountInString(in.Text) > 1000 {
		return schedule{}, ErrInvalid
	}
	if len(m[1]) == 1 {
		m[1] = "0" + m[1]
	}
	in.At = m[1] + ":" + m[2]
	switch in.Repeat {
	case "daily", "weekdays":
		if in.Date != "" {
			return schedule{}, ErrInvalid
		}
	case "once":
		at, err := time.ParseInLocation("2006-01-02 15:04", in.Date+" "+in.At, saigon)
		if err != nil || at.Format("2006-01-02") != in.Date || !at.After(now) {
			return schedule{}, ErrInvalid
		}
	default:
		return schedule{}, ErrInvalid
	}
	st := s.sched
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, old := range st.L {
		if old.ID != id {
			continue
		}
		if in.Revision != max(1, old.Revision) {
			return schedule{}, ledger.ErrConflict
		}
		x := old
		x.Text, x.At, x.Repeat, x.Date = clean(in.Text, 1000), in.At, in.Repeat, in.Date
		x.Revision, x.StartDate = max(1, old.Revision)+1, ""
		key := day
		if in.Repeat == "once" {
			key = in.Date
		}
		admitted := old.Last == key
		for _, work := range s.L.Snapshot().Work {
			if work.BotID == old.Bot && work.ClientNonce == "sched-"+id+"-"+key {
				admitted = true
				break
			}
		}
		if in.Repeat == "once" {
			if admitted {
				return schedule{}, ledger.ErrConflict
			}
			x.Last = ""
		} else {
			x.StartDate = day
			if admitted || in.At <= now.Format("15:04") {
				x.StartDate = now.AddDate(0, 0, 1).Format("2006-01-02")
			}
		}
		st.L[i] = x
		if err := st.saveLocked(); err != nil {
			st.L[i] = old
			return schedule{}, err
		}
		return x, nil
	}
	return schedule{}, ErrNotFound
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
	s.mux.HandleFunc("POST /app/api/schedules/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		var in scheduleEdit
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID {
			fail(w, ledger.ErrConflict)
			return
		}
		x, err := s.editSchedule(r.PathValue("id"), in)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"schedule": x})
	})
	s.mux.HandleFunc("GET /app/api/schedules", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"schedules": s.sched.list(r.URL.Query().Get("bot"))})
	})
	s.mux.HandleFunc("POST /app/api/schedules/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Revision uint64 `json:"revision"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if err := s.controlSchedule(r.PathValue("id"), in.Revision, nil); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	s.mux.HandleFunc("POST /app/api/schedules/{id}/state", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Revision uint64 `json:"revision"`
			Paused   *bool  `json:"paused"`
		}
		if decodeStrict(r, &in) != nil || in.Paused == nil {
			fail(w, ErrInvalid)
			return
		}
		if err := s.controlSchedule(r.PathValue("id"), in.Revision, in.Paused); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
