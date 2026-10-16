package ledger

import (
	"reflect"
	"sort"
	"strings"
	"time"
)

type Conversation struct {
	ID        string `json:"id"`
	BotID     string `json:"botId"`
	Title     string `json:"title"`
	Revision  uint64 `json:"revision"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"createdAt"`
}
type ConversationCreateInput struct {
	BotID            string `json:"botId"`
	Title            string `json:"title"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
}
type ConversationArchiveInput struct {
	Archived         bool   `json:"archived"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
}
type ConversationCommand struct {
	Kind             string `json:"kind"`
	ID               string `json:"id,omitempty"`
	BotID            string `json:"botId,omitempty"`
	Title            string `json:"title,omitempty"`
	Archived         bool   `json:"archived"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	ClientNonce      string `json:"clientNonce"`
}

func privateConversationID(id string) bool {
	return strings.HasPrefix(id, "conversation-") && lowerHex(strings.TrimPrefix(id, "conversation-"), 32)
}
func (s *Store) conversation(id string) (Conversation, bool) {
	if c, ok := s.conversations[id]; ok {
		return c, true
	}
	for _, b := range s.bots {
		if id == "conversation-"+b.ID {
			return Conversation{ID: id, BotID: b.ID, Title: "Hội thoại ban đầu"}, true
		}
	}
	return Conversation{}, false
}
func (s *Store) conversationsLocked() []Conversation {
	out := []Conversation{}
	for _, b := range s.bots {
		c, _ := s.conversation("conversation-" + b.ID)
		out = append(out, c)
	}
	for _, c := range s.conversations {
		if privateConversationID(c.ID) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s *Store) validConversationEvent(e Event) bool {
	if e.Conversation == nil || e.ConversationInput == nil {
		return false
	}
	c, in := *e.Conversation, *e.ConversationInput
	expected := Event{Sequence: e.Sequence, Type: e.Type, Conversation: &c, ConversationInput: &in, Nonce: in.ClientNonce}
	if !reflect.DeepEqual(e, expected) || !validAccessNonce(in.ClientNonce) || len(s.conversationNonces) >= 512 {
		return false
	}
	if _, ok := s.retentionNonces[in.ClientNonce]; ok {
		return false
	}
	if _, ok := s.conversationNonces[in.ClientNonce]; ok {
		return false
	}
	if s.validInput(Input{BotID: c.BotID, ClientNonce: in.ClientNonce, Text: "conversation validation", Lane: "foreground"}) != nil || c.Revision == 0 || c.Revision > 1<<53-1 || !botText(c.Title, 80) || strings.TrimSpace(c.Title) != c.Title || c.Title == "" || hasSecret([]byte(c.Title)) {
		return false
	}
	old, exists := s.conversation(c.ID)
	if e.Type == "conversation.created" {
		_, err := time.Parse(time.RFC3339Nano, c.CreatedAt)
		return err == nil && privateConversationID(c.ID) && !exists && len(s.conversations) < 128 && c.Revision == 1 && !c.Archived && in == (ConversationCommand{Kind: "create", BotID: c.BotID, Title: c.Title, ClientNonce: in.ClientNonce})
	}
	if e.Type != "conversation.updated" || !exists || old.Revision >= 1<<53-1 || in.Kind != "archive" || in.ID != c.ID || in.ExpectedRevision != old.Revision || in.BotID != "" || in.Title != "" || in.Archived != c.Archived {
		return false
	}
	old.Revision++
	old.Archived = in.Archived
	return old == c
}
func (s *Store) CreateConversation(in ConversationCreateInput) (Conversation, bool, error) {
	in.Title = strings.TrimSpace(in.Title)
	command := ConversationCommand{Kind: "create", BotID: in.BotID, Title: in.Title, ExpectedRevision: in.ExpectedRevision, ClientNonce: in.ClientNonce}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return Conversation{}, false, ErrUnavailable
	}
	if !validAccessNonce(in.ClientNonce) || in.ExpectedRevision != 0 {
		return Conversation{}, false, ErrInvalid
	}
	if e, ok := s.conversationNonces[in.ClientNonce]; ok {
		if e.ConversationInput == nil || *e.ConversationInput != command {
			return Conversation{}, false, ErrConflict
		}
		current, _ := s.conversation(e.Conversation.ID)
		return current, true, nil
	}
	if _, ok := s.retentionNonces[in.ClientNonce]; ok {
		return Conversation{}, false, ErrConflict
	}
	if len(s.conversationNonces) >= 512 || len(s.conversations) >= 128 {
		return Conversation{}, false, ErrUnavailable
	}
	id, err := newID("conversation-")
	if err != nil {
		return Conversation{}, false, err
	}
	c := Conversation{ID: id, BotID: in.BotID, Title: in.Title, Revision: 1, CreatedAt: s.clock().UTC().Format(time.RFC3339Nano)}
	e := Event{Type: "conversation.created", Conversation: &c, ConversationInput: &command, Nonce: in.ClientNonce}
	if !s.validConversationEvent(e) {
		return Conversation{}, false, ErrInvalid
	}
	if err = s.commit(e); err != nil {
		return Conversation{}, false, err
	}
	return c, false, nil
}
func (s *Store) SetConversationArchive(id string, in ConversationArchiveInput) (Conversation, bool, error) {
	command := ConversationCommand{Kind: "archive", ID: id, Archived: in.Archived, ExpectedRevision: in.ExpectedRevision, ClientNonce: in.ClientNonce}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return Conversation{}, false, ErrUnavailable
	}
	if !validAccessNonce(in.ClientNonce) || in.ExpectedRevision >= 1<<53-1 {
		return Conversation{}, false, ErrInvalid
	}
	if e, ok := s.conversationNonces[in.ClientNonce]; ok {
		if e.ConversationInput == nil || *e.ConversationInput != command {
			return Conversation{}, false, ErrConflict
		}
		current, _ := s.conversation(e.Conversation.ID)
		return current, true, nil
	}
	if _, ok := s.retentionNonces[in.ClientNonce]; ok {
		return Conversation{}, false, ErrConflict
	}
	if len(s.conversationNonces) >= 512 {
		return Conversation{}, false, ErrUnavailable
	}
	old, ok := s.conversation(id)
	if !ok {
		return Conversation{}, false, ErrNotFound
	}
	if old.Revision != in.ExpectedRevision {
		return Conversation{}, false, ErrConflict
	}
	c := old
	c.Archived = in.Archived
	c.Revision++
	e := Event{Type: "conversation.updated", Conversation: &c, ConversationInput: &command, Nonce: in.ClientNonce}
	if !s.validConversationEvent(e) {
		return Conversation{}, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return Conversation{}, false, err
	}
	return c, false, nil
}
