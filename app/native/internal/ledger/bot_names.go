package ledger

import (
	"reflect"
	"strings"
)

type BotNameInput struct {
	Name                    string `json:"name"`
	ExpectedDisplayRevision uint64 `json:"expectedDisplayRevision"`
	ClientNonce             string `json:"clientNonce"`
}

func (s *Store) validBotNameEvent(e Event) bool {
	if e.Bot == nil || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: "bot.renamed", Bot: e.Bot, Nonce: e.Nonce}) || !validAccessNonce(e.Nonce) || e.Bot.Name == "" || e.Bot.Name != strings.TrimSpace(e.Bot.Name) || !botText(e.Bot.Name, 80) {
		return false
	}
	if _, used := s.botNonces[e.Nonce]; used {
		return false
	}
	if _, used := s.botNameNonces[e.Nonce]; used {
		return false
	}
	if _, used := s.botProfileNonces[e.Nonce]; used {
		return false
	}
	if _, state := s.botStateNonces[e.Nonce]; state {
		return false
	}
	for _, prior := range s.bots {
		if prior.ID != e.Bot.ID && strings.EqualFold(prior.Name, e.Bot.Name) {
			return false
		}
	}
	for _, prior := range s.bots {
		if prior.ID != e.Bot.ID {
			continue
		}
		if prior.Name == e.Bot.Name || prior.DisplayRevision >= 1<<53-1 {
			return false
		}
		expected := prior
		expected.Name = e.Bot.Name
		expected.DisplayRevision++
		return reflect.DeepEqual(expected, *e.Bot)
	}
	return false
}

func (s *Store) RenameBot(id string, in BotNameInput) (Bot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.renameBotLocked(id, in)
}
func (s *Store) renameBotLocked(id string, in BotNameInput) (Bot, bool, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || !botText(in.Name, 80) || !validAccessNonce(in.ClientNonce) || in.ExpectedDisplayRevision >= 1<<53-1 {
		return Bot{}, false, ErrInvalid
	}
	if s.closed || s.poisoned {
		return Bot{}, false, ErrUnavailable
	}
	if _, state := s.botStateNonces[in.ClientNonce]; state {
		return Bot{}, false, ErrConflict
	}
	if _, used := s.botNonces[in.ClientNonce]; used {
		return Bot{}, false, ErrConflict
	}
	if _, used := s.botProfileNonces[in.ClientNonce]; used {
		return Bot{}, false, ErrConflict
	}
	if e, used := s.botNameNonces[in.ClientNonce]; used {
		if e.Bot.ID != id || e.Bot.Name != in.Name || e.Bot.DisplayRevision-1 != in.ExpectedDisplayRevision {
			return Bot{}, false, ErrConflict
		}
		for _, bot := range s.bots {
			if bot.ID == id {
				return bot, true, nil
			}
		}
		return Bot{}, false, ErrCorrupt
	}
	for _, bot := range s.bots {
		if bot.ID != id {
			continue
		}
		if bot.DisplayRevision != in.ExpectedDisplayRevision {
			return Bot{}, false, ErrConflict
		}
		bot.Name = in.Name
		bot.DisplayRevision++
		e := Event{Type: "bot.renamed", Bot: &bot, Nonce: in.ClientNonce}
		if !s.validBotNameEvent(e) {
			return Bot{}, false, ErrConflict
		}
		if err := s.commit(e); err != nil {
			return Bot{}, false, err
		}
		return bot, false, nil
	}
	return Bot{}, false, ErrNotFound
}
