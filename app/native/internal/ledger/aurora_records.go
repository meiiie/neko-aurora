package ledger

// Aurora records: owner-side presentation and audit facts for the Aurora
// UIs, journaled in the same hash chain as everything else so there is no
// side file. These records include:
//
//	aurora.look      – a bot's avatar, pin and tag
//	aurora.push      – this owner's Web Push subscriptions (add/remove)
//	aurora.activity  – a dedicated, append-only activity journal
//	aurora.page      – immutable owner document revisions and save receipts
//
// They never touch Work, so they cannot affect the runtime pipeline.

import (
	"reflect"
	"strings"
	"time"
)

// AuroraAvatar is a flat shape avatar (Flat: one of 13 shapes in one of 12
// colours). The pixel fields remain only so earlier journal records replay.
type AuroraAvatar struct {
	Shell int    `json:"shell"`
	Hat   string `json:"hat,omitempty"`
	Eyes  string `json:"eyes,omitempty"`
	Extra string `json:"extra,omitempty"`
	Flat  bool   `json:"flat,omitempty"`
	Shape int    `json:"shape,omitempty"`
	Color int    `json:"color,omitempty"`
}

const AuroraShapes, AuroraColors = 13, 12

type AuroraLook struct {
	BotID  string        `json:"botId"`
	Avatar *AuroraAvatar `json:"avatar,omitempty"`
	Pinned bool          `json:"pinned,omitempty"`
	Tag    string        `json:"tag,omitempty"`
}

type AuroraPush struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh,omitempty"`
	Auth     string `json:"auth,omitempty"`
	Removed  bool   `json:"removed,omitempty"`
}

type AuroraActivity struct {
	At     string `json:"at"`
	BotID  string `json:"botId"`
	WorkID string `json:"workId,omitempty"`
	Kind   string `json:"kind"` // message|plan|step|approval|decision|delivery|stop|error|reply
	Text   string `json:"text"`
}

type AuroraRecord struct {
	Look     *AuroraLook     `json:"look,omitempty"`
	Push     *AuroraPush     `json:"push,omitempty"`
	Activity *AuroraActivity `json:"activity,omitempty"`
	Page     *PageChange     `json:"page,omitempty"`
}

const auroraActivityKeep = 2000

var auroraActivityKinds = map[string]bool{"message": true, "plan": true, "step": true, "approval": true, "decision": true, "delivery": true, "stop": true, "error": true, "reply": true, "agent": true, "secret": true, "connector": true}
var auroraHat = map[string]bool{"none": true, "beanie": true, "cap": true, "top": true, "crown": true}
var auroraEyes = map[string]bool{"none": true, "round": true, "shades": true, "visor": true, "mono": true}
var auroraExtra = map[string]bool{"none": true, "bow": true, "flower": true, "antenna": true, "scarf": true}

func ValidAuroraAvatar(a AuroraAvatar) bool {
	if a.Flat {
		return a.Shell == 0 && a.Hat == "" && a.Eyes == "" && a.Extra == "" && a.Shape >= 0 && a.Shape < AuroraShapes && a.Color >= 0 && a.Color < AuroraColors
	}
	return a.Shape == 0 && a.Color == 0 && a.Shell >= 0 && a.Shell < 10 && auroraHat[a.Hat] && auroraEyes[a.Eyes] && auroraExtra[a.Extra]
}

func (s *Store) hasBot(id string) bool {
	for _, b := range s.bots {
		if b.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) validAuroraEvent(e Event) bool {
	r := e.Aurora
	if r == nil || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: e.Type, Aurora: r}) {
		return false
	}
	n := 0
	for _, set := range []bool{r.Look != nil, r.Push != nil, r.Activity != nil, r.Page != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return false
	}
	switch e.Type {
	case "aurora.page":
		return r.Page != nil && s.validPageChange(*r.Page)
	case "aurora.look":
		l := r.Look
		return l != nil && s.hasBot(l.BotID) && directText(l.Tag, 40) && (l.Avatar == nil || ValidAuroraAvatar(*l.Avatar))
	case "aurora.push":
		p := r.Push
		if p == nil || !strings.HasPrefix(p.Endpoint, "https://") || !directText(p.Endpoint, 2048) {
			return false
		}
		if p.Removed {
			return p.P256dh == "" && p.Auth == ""
		}
		return p.P256dh != "" && p.Auth != "" && directText(p.P256dh, 256) && directText(p.Auth, 128)
	case "aurora.activity":
		a := r.Activity
		if a == nil || !auroraActivityKinds[a.Kind] || !directText(a.Text, 600) || strings.TrimSpace(a.Text) == "" || !directText(a.WorkID, 128) {
			return false
		}
		_, err := time.Parse(time.RFC3339Nano, a.At)
		return err == nil && (s.hasBot(a.BotID) || a.BotID == "" && a.WorkID == "" && (a.Kind == "secret" || a.Kind == "connector"))
	}
	return false
}

func (s *Store) applyAurora(e Event) {
	r := *e.Aurora
	switch {
	case r.Page != nil:
		p := *r.Page
		r.Page = &p
		if s.pages == nil {
			s.pages = map[string][]PageRevision{}
			s.pageNonces = map[string]PageChange{}
		}
		s.pages[p.Page.ID] = append(s.pages[p.Page.ID], p.Page)
		s.pageNonces[p.Input.ClientNonce] = p
	case r.Look != nil:
		l := *r.Look
		if l.Avatar != nil {
			a := *l.Avatar
			l.Avatar = &a
		}
		r.Look = &l
		s.auroraLooks[l.BotID] = l
	case r.Push != nil:
		p := *r.Push
		r.Push = &p
		out := s.auroraPush[:0:0]
		for _, x := range s.auroraPush {
			if x.Endpoint != p.Endpoint {
				out = append(out, x)
			}
		}
		if !p.Removed {
			out = append(out, p)
		}
		s.auroraPush = out
	case r.Activity != nil:
		a := *r.Activity
		r.Activity = &a
		s.auroraActivity = append(s.auroraActivity, a)
		if len(s.auroraActivity) > auroraActivityKeep {
			s.auroraActivity = append([]AuroraActivity(nil), s.auroraActivity[len(s.auroraActivity)-auroraActivityKeep:]...)
		}
	}
	e.Aurora = &r
	s.events = append(s.events, e)
}

func (s *Store) commitAurora(typ string, r AuroraRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	e := Event{Type: typ, Aurora: &r}
	e.Sequence = uint64(len(s.events) + 1)
	if !s.validAuroraEvent(e) {
		return ErrInvalid
	}
	e.Sequence = 0
	return s.commit(e)
}

// SetAuroraLook records a bot's look; unchanged looks are not re-journaled.
func (s *Store) SetAuroraLook(l AuroraLook) error {
	s.mu.Lock()
	cur, ok := s.auroraLooks[l.BotID]
	s.mu.Unlock()
	if ok && reflect.DeepEqual(cur, l) {
		return nil
	}
	return s.commitAurora("aurora.look", AuroraRecord{Look: &l})
}

func (s *Store) AuroraLookOf(botID string) (AuroraLook, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.auroraLooks[botID]
	if ok && l.Avatar != nil {
		a := *l.Avatar
		l.Avatar = &a
	}
	return l, ok
}

func (s *Store) AddAuroraPush(p AuroraPush) error {
	s.mu.Lock()
	for _, x := range s.auroraPush {
		if x == p {
			s.mu.Unlock()
			return nil
		}
	}
	s.mu.Unlock()
	p.Removed = false
	return s.commitAurora("aurora.push", AuroraRecord{Push: &p})
}

func (s *Store) RemoveAuroraPush(endpoint string) error {
	s.mu.Lock()
	found := false
	for _, x := range s.auroraPush {
		found = found || x.Endpoint == endpoint
	}
	s.mu.Unlock()
	if !found {
		return nil
	}
	return s.commitAurora("aurora.push", AuroraRecord{Push: &AuroraPush{Endpoint: endpoint, Removed: true}})
}

func (s *Store) AuroraPushSubs() []AuroraPush {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AuroraPush(nil), s.auroraPush...)
}

// RecordAuroraActivity appends one line to the activity journal.
func (s *Store) RecordAuroraActivity(a AuroraActivity) error {
	if a.At == "" {
		a.At = directNow()
	}
	if len(a.Text) > 600 {
		cut := 597
		for cut > 0 && !utf8Start(a.Text[cut]) {
			cut--
		}
		a.Text = a.Text[:cut] + "…"
	}
	return s.commitAurora("aurora.activity", AuroraRecord{Activity: &a})
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// AuroraActivityLog returns the newest entries first.
func (s *Store) AuroraActivityLog(botID string, limit int) []AuroraActivity {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []AuroraActivity{}
	for i := len(s.auroraActivity) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		if botID == "" || s.auroraActivity[i].BotID == botID {
			out = append(out, s.auroraActivity[i])
		}
	}
	return out
}
