package ledger

import (
	"errors"
	"nekobot.local/native/internal/connections"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type BotState struct {
	Version                int    `json:"version"`
	BotID                  string `json:"botId"`
	Revision               uint64 `json:"revision"`
	Archived               bool   `json:"archived"`
	DefaultAccountID       string `json:"defaultAccountId,omitempty"`
	DefaultAccountRevision uint64 `json:"defaultAccountRevision,omitempty"`
	DefaultModel           string `json:"defaultModel,omitempty"`
}

type BotControlInput struct {
	BotID                  string `json:"botId"`
	Action                 string `json:"action"`
	ExpectedRevision       uint64 `json:"expectedRevision"`
	ClientNonce            string `json:"clientNonce"`
	DefaultAccountID       string `json:"defaultAccountId,omitempty"`
	DefaultAccountRevision uint64 `json:"defaultAccountRevision,omitempty"`
	DefaultModel           string `json:"defaultModel,omitempty"`
}

var modelPreferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

func validModelPreference(value string) bool {
	return value == "" || modelPreferencePattern.MatchString(value) && !strings.Contains(value, "://") && !hasSecret([]byte(value))
}
func validBotState(p BotState) bool {
	return p.Version == 1 && p.Revision < 1<<53 && validModelPreference(p.DefaultModel) && (p.DefaultAccountID == "" && p.DefaultAccountRevision == 0 || resourceID(p.DefaultAccountID, "account-") && p.DefaultAccountRevision > 0 && p.DefaultAccountRevision < 1<<53 && p.DefaultModel != "")
}
func copyBotState(p *BotState) *BotState {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}
func (s *Store) botState(id string) (BotState, bool) {
	for _, bot := range s.bots {
		if bot.ID == id {
			if state, ok := s.botStates[id]; ok {
				return state, true
			}
			return BotState{Version: 1, BotID: id}, true
		}
	}
	return BotState{}, false
}
func (s *Store) archivedBot(id string) bool { p, ok := s.botState(id); return ok && p.Archived }
func (s *Store) botStatesLocked() []BotState {
	out := []BotState{}
	for _, bot := range s.bots {
		p, _ := s.botState(bot.ID)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BotID < out[j].BotID })
	return out
}
func botControlMutation(prior BotState, in BotControlInput) (BotState, error) {
	if in.BotID != prior.BotID || in.ExpectedRevision != prior.Revision || prior.Revision >= 1<<53-1 || !validAccessNonce(in.ClientNonce) {
		return BotState{}, ErrConflict
	}
	next := prior
	next.Revision++
	switch in.Action {
	case "archive", "restore":
		if in.DefaultAccountID != "" || in.DefaultAccountRevision != 0 || in.DefaultModel != "" {
			return BotState{}, ErrInvalid
		}
		next.Archived = in.Action == "archive"
		if prior.Archived == next.Archived {
			return BotState{}, ErrConflict
		}
	case "defaults":
		next.DefaultAccountID, next.DefaultAccountRevision, next.DefaultModel = in.DefaultAccountID, in.DefaultAccountRevision, in.DefaultModel
		if prior.DefaultAccountID == next.DefaultAccountID && prior.DefaultAccountRevision == next.DefaultAccountRevision && prior.DefaultModel == next.DefaultModel {
			return BotState{}, ErrConflict
		}
	default:
		return BotState{}, ErrInvalid
	}
	if !validBotState(next) {
		return BotState{}, ErrInvalid
	}
	return next, nil
}
func (s *Store) botMetadataNonceUsed(nonce string) bool {
	_, a := s.botNonces[nonce]
	_, b := s.botNameNonces[nonce]
	_, c := s.botProfileNonces[nonce]
	_, d := s.botStateNonces[nonce]
	return a || b || c || d
}
func (s *Store) lifecycleNonceUsed(nonce string) bool {
	_, b := s.botStateNonces[nonce]
	_, p := s.projectStateNonces[nonce]
	_, c := s.challengeNonces[nonce]
	return b || p || c
}
func (s *Store) previousCommandNonceUsed(nonce string) bool {
	_, t := s.taskControlNonces[nonce]
	_, g := s.collaborationNonces[nonce]
	_, a := s.collaborationActions[nonce]
	return t || g || a
}
func (s *Store) validBotStateEvent(e Event) bool {
	if e.BotState == nil || e.BotControlInput == nil || e.Type != "bot.state_updated" || e.Nonce != e.BotControlInput.ClientNonce || !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: e.Type, BotState: e.BotState, BotControlInput: e.BotControlInput, Nonce: e.Nonce}) || s.botMetadataNonceUsed(e.Nonce) || s.lifecycleNonceUsed(e.Nonce) || s.previousCommandNonceUsed(e.Nonce) || len(s.botStateNonces) >= 2048 {
		return false
	}
	prior, ok := s.botState(e.BotState.BotID)
	if !ok {
		return false
	}
	next, err := botControlMutation(prior, *e.BotControlInput)
	return err == nil && next == *e.BotState
}
func (s *Store) ControlBot(in BotControlInput) (BotState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.controlBotLocked(in)
}
func (s *Store) controlBotLocked(in BotControlInput) (BotState, bool, error) {
	if !validAccessNonce(in.ClientNonce) || in.ExpectedRevision >= 1<<53-1 || !validModelPreference(in.DefaultModel) {
		return BotState{}, false, ErrInvalid
	}
	if s.closed || s.poisoned {
		return BotState{}, false, ErrUnavailable
	}
	if s.readOnly {
		return BotState{}, false, ErrReadOnly
	}
	if event, ok := s.botStateNonces[in.ClientNonce]; ok {
		if *event.BotControlInput != in {
			return BotState{}, false, ErrConflict
		}
		p, _ := s.botState(in.BotID)
		return p, true, nil
	}
	if s.botMetadataNonceUsed(in.ClientNonce) || s.lifecycleNonceUsed(in.ClientNonce) || s.previousCommandNonceUsed(in.ClientNonce) {
		return BotState{}, false, ErrConflict
	}
	if len(s.botStateNonces) >= 2048 {
		return BotState{}, false, ErrUnavailable
	}
	prior, ok := s.botState(in.BotID)
	if !ok {
		return BotState{}, false, ErrNotFound
	}
	next, err := botControlMutation(prior, in)
	if err != nil {
		return BotState{}, false, err
	}
	commit := func() error {
		return s.commit(Event{Type: "bot.state_updated", BotState: &next, BotControlInput: &in, Nonce: in.ClientNonce})
	}
	if in.Action == "defaults" && next.DefaultAccountID != "" {
		if s.accounts == nil {
			return BotState{}, false, ErrConflict
		}
		err = s.accounts.WithSelection(next.DefaultAccountID, next.DefaultAccountRevision, func(pin connections.Pin) error {
			if pin.Model != next.DefaultModel {
				return ErrConflict
			}
			return commit()
		})
		if errors.Is(err, connections.ErrConflict) {
			err = ErrConflict
		}
		return next, false, err
	}
	if err = commit(); err != nil {
		return BotState{}, false, err
	}
	return next, false, nil
}
func (s *Store) validBotStateIntake(w Work, linked bool) bool {
	if !w.UseBotDefaults && w.BotState != nil {
		return false
	}
	if w.UseBotDefaults {
		if w.BotState == nil || w.ExpectedBotStateRevision == nil || !validBotState(*w.BotState) || w.BotState.BotID != w.BotID || w.BotState.Revision != *w.ExpectedBotStateRevision {
			return false
		}
		if w.BotState.DefaultAccountID == "" {
			if w.Account != nil {
				return false
			}
		} else if w.Account == nil || w.Account.AccountID != w.BotState.DefaultAccountID || w.Account.Revision != w.BotState.DefaultAccountRevision || w.Account.Model != w.BotState.DefaultModel {
			return false
		}
	}
	if linked {
		return true
	}
	p, ok := s.botState(w.BotID)
	if !ok || w.ExpectedBotStateRevision != nil && *w.ExpectedBotStateRevision != p.Revision {
		return false
	}
	return !w.UseBotDefaults || *w.BotState == p
}
func (s *Store) targetBot(in Input) (string, error) {
	target, _, err := s.resolveRoute(in.Text)
	if err != nil {
		return "", err
	}
	if target == "" {
		target = in.BotID
	}
	return target, nil
}
