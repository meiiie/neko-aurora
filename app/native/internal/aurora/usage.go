package aurora

// Usage (idea from DS Bot's usage page): every model call is counted per
// day, per bot and per kind (chat turn, checkpoint, memory review, fact
// check…). The z.ai stream does not report tokens here, so tokens are
// estimated from characters (≈3.2 chars per token for Vietnamese/English
// mixed text) and labelled as estimates in the UI.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const charsPerToken = 3.2

type usageRow struct {
	Calls int `json:"calls"`
	In    int `json:"in"`  // characters sent
	Out   int `json:"out"` // characters received
}

type usageLog struct {
	mu    sync.Mutex
	path  string
	Days  map[string]map[string]map[string]*usageRow `json:"days"` // day → bot → kind
	dirty bool
}

func openUsage(dir string) *usageLog {
	u := &usageLog{path: filepath.Join(dir, "usage.json"), Days: map[string]map[string]map[string]*usageRow{}}
	if b, err := os.ReadFile(u.path); err == nil {
		_ = json.Unmarshal(b, u)
	}
	return u
}

func (u *usageLog) add(bot, kind string, in, out int) {
	if u == nil {
		return
	}
	if bot == "" {
		bot = "_system"
	}
	day := time.Now().In(saigon).Format("2006-01-02")
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.Days[day] == nil {
		u.Days[day] = map[string]map[string]*usageRow{}
	}
	if u.Days[day][bot] == nil {
		u.Days[day][bot] = map[string]*usageRow{}
	}
	r := u.Days[day][bot][kind]
	if r == nil {
		r = &usageRow{}
		u.Days[day][bot][kind] = r
	}
	r.Calls++
	r.In += in
	r.Out += out
	if len(u.Days) > 400 { // keep ~13 months
		keys := make([]string, 0, len(u.Days))
		for k := range u.Days {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		delete(u.Days, keys[0])
	}
	b, _ := json.Marshal(u)
	tmp := u.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, u.path)
	}
}

type usageSum struct {
	Key    string `json:"key"`
	Calls  int    `json:"calls"`
	Tokens int    `json:"tokens"` // estimated
}

// report sums the last `days` days by day, by bot and by kind.
func (u *usageLog) report(days int, now time.Time) map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	byDay, byBot, byKind := map[string]*usageSum{}, map[string]*usageSum{}, map[string]*usageSum{}
	total := usageSum{Key: "total"}
	from := now.In(saigon).AddDate(0, 0, -days+1).Format("2006-01-02")
	bump := func(m map[string]*usageSum, k string, r *usageRow) {
		if m[k] == nil {
			m[k] = &usageSum{Key: k}
		}
		m[k].Calls += r.Calls
		m[k].Tokens += int(float64(r.In+r.Out) / charsPerToken)
	}
	for day, bots := range u.Days {
		if day < from {
			continue
		}
		for bot, kinds := range bots {
			for kind, r := range kinds {
				bump(byDay, day, r)
				bump(byBot, bot, r)
				bump(byKind, kind, r)
				total.Calls += r.Calls
				total.Tokens += int(float64(r.In+r.Out) / charsPerToken)
			}
		}
	}
	list := func(m map[string]*usageSum, byKey bool) []usageSum {
		out := make([]usageSum, 0, len(m))
		for _, v := range m {
			out = append(out, *v)
		}
		sort.Slice(out, func(i, j int) bool {
			if byKey {
				return out[i].Key < out[j].Key
			}
			return out[i].Tokens > out[j].Tokens
		})
		return out
	}
	return map[string]any{"days": list(byDay, true), "bots": list(byBot, false), "kinds": list(byKind, false), "total": total, "estimated": true}
}

func msgChars(msgs []ChatMsg) int {
	n := 0
	for _, m := range msgs {
		n += len([]rune(m.Content))
	}
	return n
}

// stream is s.AI.Stream with usage accounting.
func (s *Service) stream(ctx context.Context, bot, kind string, msgs []ChatMsg, on func(string)) (string, error) {
	if s.V != nil { // DS Bot: mask every secret value before a request leaves
		masked := make([]ChatMsg, len(msgs))
		for i, m := range msgs {
			m.Content = s.V.Redact(m.Content)
			masked[i] = m
		}
		msgs = masked
	}
	out, err := s.AI.Stream(ctx, msgs, on)
	s.usage.add(bot, kind, msgChars(msgs), len([]rune(out)))
	return out, err
}

func (s *Service) usageRoutes() {
	s.mux.HandleFunc("GET /app/api/usage", func(w http.ResponseWriter, r *http.Request) {
		days := 7
		switch r.URL.Query().Get("range") {
		case "1":
			days = 1
		case "30":
			days = 30
		case "365":
			days = 365
		}
		rep := s.usage.report(days, time.Now())
		v := s.view()
		names := map[string]string{"_system": "Hệ thống"}
		for _, b := range v.snap.Bots {
			names[b.ID] = displayName(b)
		}
		rep["names"] = names
		rep["range"] = days
		writeJSON(w, 200, rep)
	})
}
