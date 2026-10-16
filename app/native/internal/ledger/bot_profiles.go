package ledger

import (
	"reflect"
	"unicode"
	"unicode/utf8"
)

type BotProfile struct {
	BotID          string `json:"botId"`
	Revision       uint64 `json:"revision"`
	Instructions   string `json:"instructions"`
	IncludeHistory bool   `json:"includeHistory"`
}

type BotProfileInput struct {
	Instructions     string `json:"instructions"`
	IncludeHistory   bool   `json:"includeHistory"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
}

func validProfileText(text string) bool {
	if !utf8.ValidString(text) || len(text) > 4000 || hasSecret([]byte(text)) {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func copyBotProfile(p *BotProfile) *BotProfile {
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}

func (s *Store) profile(id string) (BotProfile, bool) {
	for _, bot := range s.bots {
		if bot.ID == id {
			if p, ok := s.botProfiles[id]; ok {
				return p, true
			}
			return BotProfile{BotID: id, IncludeHistory: true}, true
		}
	}
	return BotProfile{}, false
}

func (s *Store) pinBotProfile(w *Work) {
	if w.RequesterPin != nil && w.RequesterPin.ActorID != "local-owner" {
		return
	}
	if p, ok := s.botProfiles[w.BotID]; ok {
		w.BotProfile = copyBotProfile(&p)
	}
}

func (s *Store) validProfileIntake(w Work) bool {
	if w.RequesterPin != nil && w.RequesterPin.ActorID != "local-owner" {
		return w.BotProfile == nil && w.ExpectedBotProfileRevision == nil && w.Context != nil && w.Context.Profile == nil
	}
	if w.Context != nil && w.Context.Version == 4 && w.BotProfile == nil {
		return false
	}
	current, found := s.profile(w.BotID)
	if !found || w.ExpectedBotProfileRevision != nil && *w.ExpectedBotProfileRevision != current.Revision {
		return false
	}
	p, ok := s.botProfiles[w.BotID]
	if !ok {
		return w.BotProfile == nil && (w.Context == nil || w.Context.Profile == nil)
	}
	return reflect.DeepEqual(w.BotProfile, &p) && w.Context != nil && (w.Context.Version == 4 || w.Context.Version == 5) && reflect.DeepEqual(w.Context.Profile, &p)
}

func (s *Store) validBotProfileEvent(e Event) bool {
	p := e.BotProfile
	if p == nil || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: "bot.profile_updated", BotProfile: p, Nonce: e.Nonce}) || !validAccessNonce(e.Nonce) || !validProfileText(p.Instructions) {
		return false
	}
	old, ok := s.profile(p.BotID)
	if !ok || old.Revision >= 1<<53-1 || p.Revision != old.Revision+1 || old.Instructions == p.Instructions && old.IncludeHistory == p.IncludeHistory {
		return false
	}
	_, created := s.botNonces[e.Nonce]
	_, renamed := s.botNameNonces[e.Nonce]
	_, configured := s.botProfileNonces[e.Nonce]
	_, state := s.botStateNonces[e.Nonce]
	return !created && !renamed && !configured && !state && len(s.botProfileNonces) < 2048
}

func (s *Store) SetBotProfile(id string, in BotProfileInput) (BotProfile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setBotProfileLocked(id, in)
}
func (s *Store) setBotProfileLocked(id string, in BotProfileInput) (BotProfile, bool, error) {
	if !validAccessNonce(in.ClientNonce) || !validProfileText(in.Instructions) || in.ExpectedRevision >= 1<<53-1 {
		return BotProfile{}, false, ErrInvalid
	}
	if s.closed || s.poisoned {
		return BotProfile{}, false, ErrUnavailable
	}
	if _, state := s.botStateNonces[in.ClientNonce]; state {
		return BotProfile{}, false, ErrConflict
	}
	if _, ok := s.botNonces[in.ClientNonce]; ok {
		return BotProfile{}, false, ErrConflict
	}
	if _, ok := s.botNameNonces[in.ClientNonce]; ok {
		return BotProfile{}, false, ErrConflict
	}
	if e, ok := s.botProfileNonces[in.ClientNonce]; ok {
		p := e.BotProfile
		if p.BotID != id || p.Revision-1 != in.ExpectedRevision || p.Instructions != in.Instructions || p.IncludeHistory != in.IncludeHistory {
			return BotProfile{}, false, ErrConflict
		}
		current, found := s.profile(id)
		if !found {
			return BotProfile{}, false, ErrCorrupt
		}
		return current, true, nil
	}
	p, ok := s.profile(id)
	if !ok {
		return BotProfile{}, false, ErrNotFound
	}
	if p.Revision != in.ExpectedRevision {
		return BotProfile{}, false, ErrConflict
	}
	p.Revision++
	p.Instructions, p.IncludeHistory = in.Instructions, in.IncludeHistory
	e := Event{Type: "bot.profile_updated", BotProfile: &p, Nonce: in.ClientNonce}
	if !s.validBotProfileEvent(e) {
		return BotProfile{}, false, ErrConflict
	}
	if err := s.commit(e); err != nil {
		return BotProfile{}, false, err
	}
	return p, false, nil
}
