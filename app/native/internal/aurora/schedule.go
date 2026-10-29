package aurora

// Schedules (idea from DS Bot): the owner asks a bot to do something at a
// time ("mỗi sáng 8h tóm tắt tin vàng"), the bot answers with a SCHEDULE
// line, and at that time the request runs as an ordinary turn of that bot
// (push notification included). Legacy schedules use Vietnam time. One occurrence is
// one ledger nonce, so a restart never runs it twice.

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"nekoaurora.local/native/internal/ledger"
)

const maxSchedules = 12

const defaultScheduleTimezone = "Asia/Ho_Chi_Minh"

type schedule struct {
	ID        string `json:"id"`
	Timezone  string `json:"timezone,omitempty"`
	Bot       string `json:"bot"`
	Text      string `json:"text"`
	At        string `json:"at"`             // HH:MM in Timezone
	Repeat    string `json:"repeat"`         // daily | weekdays | once
	Date      string `json:"date,omitempty"` // once: YYYY-MM-DD
	Last      string `json:"last,omitempty"` // last occurrence key run
	Paused    bool   `json:"paused"`
	Revision  uint64 `json:"revision"`
	StartDate string `json:"startDate,omitempty"` // earliest day after an owner edit
	Blocked   string `json:"blocked,omitempty"`
}

func (x schedule) zoneName() string {
	if x.Timezone == "" {
		return defaultScheduleTimezone
	}
	return x.Timezone
}

func scheduleLocation(zone string) (*time.Location, error) {
	if zone == "" {
		return saigon, nil
	}
	if len(zone) > 64 || zone != strings.TrimSpace(zone) || zone == "Local" || strings.HasPrefix(zone, "/") || strings.Contains(zone, "..") || strings.Contains(zone, "\\") || (zone != "UTC" && !strings.Contains(zone, "/")) {
		return nil, ErrInvalid
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, ErrInvalid
	}
	return loc, nil
}

type schedStore struct {
	mu      sync.Mutex
	path    string
	L       []schedule
	loadErr error
}

var hhmm = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

// normalize validates a schedule from a directive; ok=false drops it.
func (x schedule) normalize(now time.Time) (schedule, bool) {
	x.Timezone = strings.TrimSpace(x.Timezone)
	loc, err := scheduleLocation(x.Timezone)
	if err != nil {
		return x, false
	}
	x.Timezone = x.zoneName()
	x.StartDate = ""
	x.Blocked = ""
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
		t := now.In(loc)
		today := t.Format("2006-01-02")
		if _, err := time.ParseInLocation("2006-01-02", x.Date, loc); err != nil || x.Date < today {
			x.Date = today
		}
		if x.Date == today && t.Format("15:04") >= x.At {
			// a time already past today means the next one (never fire at once)
			x.Date = t.AddDate(0, 0, 1).Format("2006-01-02")
		}
	}
	if x.Repeat == "once" {
		at, err := time.ParseInLocation("2006-01-02 15:04", x.Date+" "+x.At, loc)
		if err != nil || at.Format("2006-01-02 15:04") != x.Date+" "+x.At {
			return x, false
		}
	}
	return x, true
}

// due: the occurrence key to run now, or "".
func (x schedule) due(now time.Time) string {
	loc, err := scheduleLocation(x.Timezone)
	if err != nil {
		return ""
	}
	if x.Paused {
		return ""
	}
	t := now.In(loc)
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
	if st.loadErr != nil || x.ID == "" || len(st.L) >= maxSchedules {
		return false
	}
	for _, y := range st.L {
		if y.ID == x.ID {
			return false
		}
		if y.Bot == x.Bot && y.At == x.At && y.zoneName() == x.zoneName() && y.Repeat == x.Repeat && strings.EqualFold(y.Text, x.Text) {
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
	if st.loadErr != nil {
		return false
	}
	for i, x := range st.L {
		if x.ID == id {
			before := append([]schedule(nil), st.L...)
			st.L = append(st.L[:i], st.L[i+1:]...)
			if st.saveLocked() != nil {
				st.L = before
				return false
			}
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
	if st.loadErr != nil || s.L.ReadOnly() {
		return
	}
	before := append([]schedule(nil), st.L...)
	changed := false
	for i := range st.L {
		if _, err := scheduleLocation(st.L[i].Timezone); err != nil {
			if st.L[i].Blocked != "invalid_timezone" {
				st.L[i].Blocked = "invalid_timezone"
				changed = true
			}
			continue
		}
		if k := st.L[i].due(now); k != "" {
			x := st.L[i]
			text := "[Lịch hẹn " + x.At + " " + x.zoneName() + "] " + x.Text
			nonce := "sched-" + x.ID + "-" + k
			admitted := false
			for _, work := range s.L.Snapshot().Work {
				if work.ClientNonce == nonce && (work.BotID == x.Bot || work.SourceBotID == x.Bot) {
					admitted = true
					break
				}
			}
			var err error
			if !admitted {
				err = s.admitRoutine(x.Bot, nonce, text)
				if err == nil {
					_, _, err = s.L.Accept(ledger.Input{BotID: x.Bot, ClientNonce: nonce, Text: text})
				}
			}
			if err != nil {
				blocked := "admission_unavailable"
				if err.Error() == "quota_exceeded" {
					blocked = "quota_exceeded"
				}
				if st.L[i].Blocked != blocked {
					st.L[i].Blocked = blocked
					changed = true
				}
				continue
			}
			st.L[i].Blocked = ""
			st.L[i].Last = k
			changed = true
		}
	}
	keep := st.L[:0]
	for _, x := range st.L {
		if !(x.Repeat == "once" && x.Last != "") {
			keep = append(keep, x)
		} else {
			changed = true
		}
	}
	st.L = keep
	if changed {
		if st.saveLocked() != nil {
			st.L = before
		}
		// Admitted work is already durable in the ledger even if saving routine progress failed.
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
	if st.loadErr != nil {
		return errScheduleStorage
	}
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
	Timezone    *string `json:"timezone"`
	WorkspaceID string  `json:"workspaceId"`
	Revision    uint64  `json:"revision"`
	Text        string  `json:"text"`
	At          string  `json:"at"`
	Repeat      string  `json:"repeat"`
	Date        string  `json:"date"`
}

func (s *Service) editSchedule(id string, in scheduleEdit) (schedule, error) {
	if s.L.ReadOnly() {
		return schedule{}, ledger.ErrConflict
	}
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
	default:
		return schedule{}, ErrInvalid
	}
	st := s.sched
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.loadErr != nil {
		return schedule{}, errScheduleStorage
	}
	for i, old := range st.L {
		if old.ID != id {
			continue
		}
		if in.Revision != max(1, old.Revision) {
			return schedule{}, ledger.ErrConflict
		}
		zone := old.Timezone
		if in.Timezone != nil {
			zone = strings.TrimSpace(*in.Timezone)
			if zone == "" {
				return schedule{}, ErrInvalid
			}
		}
		loc, err := scheduleLocation(zone)
		if err != nil {
			return schedule{}, ErrInvalid
		}
		now := s.Now().In(loc)
		day := now.Format("2006-01-02")
		if in.Repeat == "once" {
			at, err := time.ParseInLocation("2006-01-02 15:04", in.Date+" "+in.At, loc)
			if err != nil || at.Format("2006-01-02 15:04") != in.Date+" "+in.At || !at.After(now) {
				return schedule{}, ErrInvalid
			}
		}
		x := old
		x.Timezone = zone
		x.Text, x.At, x.Repeat, x.Date = clean(in.Text, 1000), in.At, in.Repeat, in.Date
		x.Revision, x.StartDate = max(1, old.Revision)+1, ""
		x.Blocked = ""
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
			s.record(w.BotID, w.RequestID, "step", "Đã đặt lịch "+n.At+" "+n.zoneName()+" "+when+": "+clean(n.Text, 80))
		}
	}
}

func (s *Service) scheduleRoutes() {
	s.scheduleTestRoutes()
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
		if err := s.sched.storageError(); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"schedules": s.sched.list(r.URL.Query().Get("bot"))})
	})
	s.mux.HandleFunc("POST /app/api/schedules/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID string `json:"workspaceId"`
			Revision    uint64 `json:"revision"`
		}
		if decodeStrict(r, &in) != nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID {
			fail(w, ledger.ErrConflict)
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
			WorkspaceID string `json:"workspaceId"`
			Revision    uint64 `json:"revision"`
			Paused      *bool  `json:"paused"`
		}
		if decodeStrict(r, &in) != nil || in.Paused == nil {
			fail(w, ErrInvalid)
			return
		}
		if in.WorkspaceID != s.workspaceID {
			fail(w, ledger.ErrConflict)
			return
		}
		if err := s.controlSchedule(r.PathValue("id"), in.Revision, in.Paused); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
