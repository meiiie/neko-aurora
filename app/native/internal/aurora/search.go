package aurora

// Chat search across every bot and group thread (idea from DS Bot's chat
// index; here the ledger is already in memory, so a fold-and-scan is enough
// for an account's size). Matching ignores case and Vietnamese diacritics,
// so "tom tat" finds "tóm tắt".

import (
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"nekoaurora.local/native/internal/ledger"
)

type searchHit struct {
	AgentID string `json:"agentId"`
	Name    string `json:"name"`
	Role    string `json:"role"` // user | agent
	Snippet string `json:"snippet"`
	At      string `json:"at"`
}

func foldText(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'đ', 'Đ':
			return 'd'
		}
		return foldVietnamese(r)
	}, strings.ToLower(s))
}

// snippet cuts ~120 characters around the first match (by rune, so the
// folded and original strings line up: folding keeps one rune per rune).
func snippet(text string, fq string) (string, bool) {
	f := foldText(text)
	i := strings.Index(f, fq)
	if i < 0 {
		return "", false
	}
	r := []rune(text)
	at := utf8.RuneCountInString(f[:i])
	from, to := max(0, at-50), min(len(r), at+utf8.RuneCountInString(fq)+70)
	s := strings.Join(strings.Fields(string(r[from:to])), " ")
	if from > 0 {
		s = "…" + s
	}
	if to < len(r) {
		s += "…"
	}
	return s, true
}

func (v *view) search(q string, limit int) []searchHit {
	fq := foldText(strings.TrimSpace(q))
	if utf8.RuneCountInString(fq) < 2 {
		return []searchHit{}
	}
	hits := []searchHit{}
	seen := map[string]bool{} // a group message is one work item per member
	add := func(thread, name string, w ledger.Work, bot bool) {
		if strings.HasPrefix(w.ClientNonce, "computer-") {
			return // hidden continuation turns
		}
		if base := thread + "|" + groupBase(w.ClientNonce); !bot || !seen[base] {
			seen[base] = true
			if s, ok := snippet(w.Text, fq); ok {
				hits = append(hits, searchHit{thread, name, "user", s, w.CreatedAt})
			}
		}
		if s, ok := snippet(visibleText(w.Reply), fq); ok {
			who := name
			if b, ok := v.bot(w.BotID); ok && isGroupID(thread) {
				who = name + " · " + displayName(b)
			}
			hits = append(hits, searchHit{thread, who, "agent", s, w.CreatedAt})
		}
	}
	for _, b := range v.snap.Bots {
		if v.archived[b.ID] {
			continue
		}
		for _, w := range v.byBot[b.ID] {
			add(b.ID, displayName(b), w, false)
		}
	}
	for id, c := range v.channels {
		for _, w := range v.byConv[id] {
			add(id, c.Name, w, true)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].At > hits[j].At })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func (s *Service) searchRoutes() {
	s.mux.HandleFunc("GET /app/api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if len(q) > 200 {
			q = q[:200]
		}
		writeJSON(w, 200, map[string]any{"hits": s.view().search(q, 40)})
	})
}
