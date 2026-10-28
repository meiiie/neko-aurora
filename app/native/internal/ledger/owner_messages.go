package ledger

import (
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type OwnerMessageInput struct {
	ThreadID    string `json:"threadId"`
	ClientNonce string `json:"clientNonce"`
	Text        string `json:"text"`
	ReferenceID string `json:"referenceId,omitempty"`
}

// Derived context is evaluated only for the first successful admission.
type OwnerMessagePlan struct {
	BotIDs          []string
	ChannelRevision uint64
	ReferenceText   string
}

type OwnerMessageReceipt struct {
	Input         OwnerMessageInput `json:"input"`
	ReferenceText string            `json:"referenceText,omitempty"`
	WorkIDs       []string          `json:"workIds"`
}

func ownerMessageKey(in OwnerMessageInput) string { return nonceKey(in.ThreadID, in.ClientNonce) }

func (s *Store) rememberOwnerMessage(r OwnerMessageReceipt) {
	if s.ownerMessages == nil {
		s.ownerMessages = make(map[string]OwnerMessageReceipt)
	}
	r.WorkIDs = append([]string(nil), r.WorkIDs...)
	s.ownerMessages[ownerMessageKey(r.Input)] = r
}

func (s *Store) ownerMessageWork(r OwnerMessageReceipt) []Work {
	out := make([]Work, len(r.WorkIDs))
	for i, id := range r.WorkIDs {
		out[i] = copyWork(s.work[id])
	}
	return out
}

func validOwnerMessageRecord(r record) bool {
	if r.Version != 2 || r.Message == nil || !reflect.DeepEqual(r.Event, Event{}) || len(r.Batch) == 0 || len(r.Batch) > 8 || len(r.Message.WorkIDs) != len(r.Batch) {
		return false
	}
	in := r.Message.Input
	group := validChannelID(in.ThreadID)
	if in.ThreadID == "" || !utf8.ValidString(in.Text) || strings.TrimSpace(in.Text) == "" || len(in.ClientNonce) < 8 || len(in.ClientNonce) > 100 || strings.HasPrefix(in.ClientNonce, "network-") || in.ReferenceID == in.ThreadID || len(in.ReferenceID) > 128 || in.ReferenceID == "" && r.Message.ReferenceText != "" || !group && len(r.Batch) != 1 {
		return false
	}
	seen := map[string]bool{}
	for i, e := range r.Batch {
		w := e.Work
		nonce, conversation := in.ClientNonce, "conversation-"+in.ThreadID
		if group {
			nonce, conversation = in.ClientNonce+"_g"+strconv.Itoa(i+1), in.ThreadID
			if w.SourceBotID != "" || w.RoutedAlias != "" {
				return false
			}
		}
		if !reflect.DeepEqual(e, Event{Sequence: e.Sequence, Type: "work.accepted", Work: w}) || w.RequestID != r.Message.WorkIDs[i] || seen[originalBot(w)] || w.ClientNonce != nonce || w.ConversationID != conversation || w.Text != in.Text+r.Message.ReferenceText || w.Lane != "foreground" || w.RequesterPin != nil || w.Account != nil || w.Adapter != nil || w.Collaboration != nil || !group && originalBot(w) != in.ThreadID {
			return false
		}
		seen[originalBot(w)] = true
	}
	return true
}

func (s *Store) replayRecord(r record) error {
	events := []Event{r.Event}
	switch r.Version {
	case 1:
		if r.Batch != nil || r.Message != nil {
			return ErrCorrupt
		}
	case 2:
		if !validOwnerMessageRecord(r) {
			return ErrCorrupt
		}
		if _, exists := s.ownerMessages[ownerMessageKey(r.Message.Input)]; exists {
			return ErrCorrupt
		}
		events = r.Batch
	default:
		return ErrCorrupt
	}
	for _, e := range events {
		if e.Sequence != uint64(len(s.events)+1) || !validReplay(s, e) {
			return ErrCorrupt
		}
		s.apply(e)
	}
	if r.Message != nil {
		s.rememberOwnerMessage(*r.Message)
	}
	return nil
}

// AcceptOwnerMessage commits the receipt and every recipient in one journal frame.
// A lost response can be retried after membership or referenced history changes.
func (s *Store) AcceptOwnerMessage(in OwnerMessageInput, plan OwnerMessagePlan) ([]Work, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return nil, false, ErrUnavailable
	}
	if s.readOnly {
		return nil, false, ErrReadOnly
	}
	if s.identity != nil {
		return nil, false, ErrForbidden
	}
	if r, ok := s.ownerMessages[ownerMessageKey(in)]; ok {
		if r.Input != in {
			return nil, false, ErrConflict
		}
		return s.ownerMessageWork(r), true, nil
	}
	group := validChannelID(in.ThreadID)
	if len(plan.BotIDs) == 0 || len(plan.BotIDs) > 8 || !group && (len(plan.BotIDs) != 1 || plan.BotIDs[0] != in.ThreadID || plan.ChannelRevision != 0) || group && plan.ChannelRevision == 0 {
		return nil, false, ErrInvalid
	}
	if in.ReferenceID != "" {
		_, channel := s.channels[in.ReferenceID]
		if !s.hasBot(in.ReferenceID) && !channel {
			return nil, false, ErrNotFound
		}
	}
	scope := SubmissionScope{}
	if group {
		scope.ChannelID, scope.ExpectedChannelRevision = in.ThreadID, &plan.ChannelRevision
		scope.groupRecipient = true
	}
	r := record{Version: 2, Previous: s.digest, Message: &OwnerMessageReceipt{Input: in, ReferenceText: plan.ReferenceText}}
	// Only the sequence cursor is staged. Pending siblings are not history sources.
	originalEvents := s.events
	staging := true
	s.events = append([]Event(nil), originalEvents...)
	defer func() {
		if staging {
			s.events = originalEvents
		}
	}()
	seen := map[string]bool{}
	for i, bot := range plan.BotIDs {
		if seen[bot] {
			return nil, false, ErrInvalid
		}
		seen[bot] = true
		nonce := in.ClientNonce
		if group {
			nonce += "_g" + strconv.Itoa(i+1)
		}
		w, duplicate, err := s.prepareAcceptanceLocked(Input{BotID: bot, ClientNonce: nonce, Text: in.Text + plan.ReferenceText, Lane: "foreground"}, nil, scope.ChannelID, scope)
		if err != nil {
			return nil, false, err
		}
		if duplicate {
			if !group && in.ReferenceID == "" {
				return []Work{w}, true, nil
			}
			return nil, false, ErrConflict
		}
		e := Event{Sequence: uint64(len(s.events) + 1), Type: "work.accepted", Work: w}
		if !validReplay(s, e) {
			return nil, false, ErrInvalid
		}
		r.Batch = append(r.Batch, e)
		r.Message.WorkIDs = append(r.Message.WorkIDs, w.RequestID)
		s.events = append(s.events, e)
	}
	s.events = originalEvents
	staging = false
	if !validOwnerMessageRecord(r) {
		return nil, false, ErrInvalid
	}
	if err := s.commitRecord(r); err != nil {
		return nil, false, err
	}
	return s.ownerMessageWork(*r.Message), false, nil
}
