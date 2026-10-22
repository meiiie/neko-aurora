package duet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nekobot.local/native/internal/ledger"
)

// One-time import of the pre-unification Duet store (state/duet/duet.json)
// into the control-plane ledger. Each legacy agent becomes a ledger bot; each
// owner message becomes a ledger task whose reply, plan and approval are
// recorded as an "imported" direct run, preserving pending approvals as
// pending. Nonces are derived from legacy ids, so a crash mid-import resumes
// without duplicates. The legacy file is renamed, never deleted.

type legacyAgent struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Desc     string    `json:"desc"`
	Tag      string    `json:"tag"`
	Avatar   Avatar    `json:"av"`
	Pinned   bool      `json:"pin"`
	Archived bool      `json:"archived"`
	Created  time.Time `json:"created"`
}

type legacyMessage struct {
	ID       string                 `json:"id"`
	AgentID  string                 `json:"agentId"`
	Role     string                 `json:"role"`
	Text     string                 `json:"text"`
	Approval *ledger.DirectApproval `json:"approval"`
	Plan     *struct {
		Steps []ledger.DirectStep `json:"steps"`
	} `json:"plan"`
	Choices []string  `json:"choices"`
	Created time.Time `json:"t"`
}

type legacyData struct {
	Agents   []legacyAgent   `json:"agents"`
	Messages []legacyMessage `json:"messages"`
	Push     []PushSub       `json:"push"`
}

// legacy approval timestamps were time.Time; accept both shapes.
func (m *legacyMessage) UnmarshalJSON(b []byte) error {
	type plain legacyMessage
	var raw struct {
		plain
		Approval *struct {
			ledger.DirectApproval
			DecidedAt *time.Time `json:"decidedAt"`
			Delivery  *struct {
				Status, Detail, From string
				At                   time.Time
			} `json:"delivery"`
		} `json:"approval"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*m = legacyMessage(raw.plain)
	if raw.Approval != nil {
		a := raw.Approval.DirectApproval
		if raw.Approval.DecidedAt != nil {
			a.DecidedAt = raw.Approval.DecidedAt.UTC().Format(time.RFC3339Nano)
		}
		if d := raw.Approval.Delivery; d != nil && a.State == "approved" {
			a.Delivery = &ledger.DirectDelivery{Status: d.Status, Detail: d.Detail, From: d.From, At: d.At.UTC().Format(time.RFC3339Nano)}
		} else {
			a.Delivery = nil
		}
		m.Approval = &a
	}
	return nil
}

func legacyNonce(prefix, id string) string {
	id = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, id)
	return prefix + id
}

func (s *Service) migrateLegacy(stateDir string) error {
	path := filepath.Join(stateDir, "duet.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.L.ReadOnly() {
		return nil
	}
	var d legacyData
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	bots := map[string]string{} // legacy agent id → bot id
	for _, a := range d.Agents {
		if a.Archived {
			continue
		}
		bot, err := s.createBot(clean(a.Name, 28), clean(a.Desc, 240), legacyNonce("duetmig-", a.ID))
		if err != nil {
			// same nonce already imported (resume) → find by nonce-derived name
			for _, x := range s.L.Snapshot().Bots {
				if strings.EqualFold(x.Name, clean(a.Name, 28)) {
					bot, err = x, nil
				}
			}
			if err != nil {
				return fmt.Errorf("bot %s: %w", a.ID, err)
			}
		}
		bots[a.ID] = bot.ID
		av, pin, tag := a.Avatar, a.Pinned, a.Tag
		if !validAvatar(av) {
			av = defaultAvatar(bot.ID)
		}
		_ = s.Prefs.SetBot(bot.ID, func(p *botPrefs) { p.Avatar, p.Pinned, p.Tag = &av, pin, tag })
	}
	for _, p := range d.Push {
		_ = s.Prefs.AddPush(p)
	}
	// group: an owner message opens a task; following agent/system messages fill it
	type turn struct {
		user  *legacyMessage
		after []legacyMessage
	}
	turns := map[string][]*turn{}
	order := []string{}
	for i := range d.Messages {
		m := d.Messages[i]
		if bots[m.AgentID] == "" {
			continue
		}
		l := turns[m.AgentID]
		if len(l) == 0 {
			order = append(order, m.AgentID)
		}
		if m.Role == "user" || len(l) == 0 {
			t := &turn{}
			if m.Role == "user" {
				t.user = &m
			} else {
				t.after = append(t.after, m)
			}
			turns[m.AgentID] = append(l, t)
			continue
		}
		l[len(l)-1].after = append(l[len(l)-1].after, m)
	}
	for _, agentID := range order {
		for _, t := range turns[agentID] {
			if err := s.importTurn(bots[agentID], t.user, t.after); err != nil {
				return err
			}
		}
	}
	return os.Rename(path, path+".migrated")
}

func (s *Service) importTurn(botID string, user *legacyMessage, after []legacyMessage) error {
	run := ledger.DirectRun{Runtime: "imported"}
	text, nonce, at := "", "", time.Now()
	if user != nil {
		text, nonce, at = user.Text, legacyNonce("duetmig-", user.ID), user.Created
	} else {
		if len(after) == 0 {
			return nil
		}
		run.Kind = "note"
		text, nonce, at = "[Nhập từ Neko Duet] lời chào của agent", legacyNonce("duetmig-", after[0].ID), after[0].Created
	}
	run.ImportedAt = at.UTC().Format(time.RFC3339Nano)
	run.StartedAt = run.ImportedAt
	w, dup, err := s.L.Accept(ledger.Input{BotID: botID, ClientNonce: nonce, Text: clean(text, 4000)})
	if err != nil {
		return err
	}
	if dup && w.Direct != nil {
		return nil
	}
	parts := []string{}
	for _, m := range after {
		switch {
		case m.Plan != nil && len(m.Plan.Steps) > 0:
			run.PlanIntro, run.Plan = clean(m.Text, 600), m.Plan.Steps
			if len(run.Plan) > 8 {
				run.Plan = run.Plan[:8]
			}
			continue
		case m.Approval != nil:
			a := *m.Approval
			run.Approval = &a
		}
		if len(m.Choices) > 0 {
			run.Choices = m.Choices
		}
		if t := strings.TrimSpace(m.Text); t != "" {
			if m.Role == "system" {
				t = "[" + t + "]"
			}
			parts = append(parts, t)
		}
	}
	w, err = s.L.DirectStart(w.RequestID, w.Epoch, run)
	if err != nil {
		return err
	}
	if reply := strings.Join(parts, "\n\n"); reply != "" {
		if len(reply) > 60000 {
			reply = strings.ToValidUTF8(reply[:60000], "")
		}
		if err := s.L.AppendText(w.RequestID, w.Epoch, reply); err != nil {
			return err
		}
	}
	_, err = s.L.DirectFinish(w.RequestID, w.Epoch, "completed", "", nil)
	return err
}
