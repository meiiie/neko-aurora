package ledger

import (
	"reflect"
	"sort"
	"strings"
)

type Channel struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerUserID string    `json:"ownerUserId"`
	SpaceID     string    `json:"spaceId"`
	Revision    uint64    `json:"revision"`
	BotIDs      []string  `json:"botIds"`
	HumanACL    *HumanACL `json:"humanAcl,omitempty"`
	SelfRole    string    `json:"selfRole,omitempty"`
}
type ChannelInput struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	ExpectedRevision uint64    `json:"expectedRevision"`
	BotIDs           []string  `json:"botIds"`
	ClientNonce      string    `json:"clientNonce"`
	HumanACL         *HumanACL `json:"humanAcl,omitempty"`
}
type Publication struct {
	Version          int    `json:"version"`
	ActorID          string `json:"actorId"`
	Platform         string `json:"platform"`
	SourceChannelID  string `json:"sourceChannelId"`
	SourceMessageID  string `json:"sourceMessageId"`
	ReplyChannelID   string `json:"replyChannelId"`
	ReplyThreadID    string `json:"replyThreadId"`
	AudienceRevision uint64 `json:"audienceRevision"`
}

func (s *Store) channel(id string) (Channel, bool) {
	if privateConversationID(id) {
		if c, ok := s.conversation(id); ok {
			return Channel{ID: c.ID, Name: c.Title, OwnerUserID: "local-owner", SpaceID: "local-space", Revision: 1, BotIDs: []string{c.BotID}}, true
		}
		return Channel{}, false
	}
	if c, ok := s.channels[id]; ok {
		return c, true
	}
	for _, b := range s.bots {
		if id == "conversation-"+b.ID {
			return Channel{ID: id, Name: b.Name, OwnerUserID: "local-owner", SpaceID: "local-space", Revision: 1, BotIDs: []string{b.ID}}, true
		}
	}
	return Channel{}, false
}
func member(c Channel, bot string) bool {
	for _, id := range c.BotIDs {
		if id == bot {
			return true
		}
	}
	return false
}
func cloneChannel(c Channel) Channel {
	c.BotIDs = append([]string{}, c.BotIDs...)
	c.HumanACL = copyHumanACL(c.HumanACL)
	return c
}
func validChannelID(id string) bool {
	return strings.HasPrefix(id, "channel-") && lowerHex(strings.TrimPrefix(id, "channel-"), 32)
}
func (s *Store) validChannel(c Channel) bool {
	if c.SelfRole != "" || !validHumanACL(c.HumanACL) || !validChannelID(c.ID) || c.OwnerUserID != "local-owner" || c.SpaceID != "local-space" || c.Revision == 0 || !botText(c.Name, 80) || strings.TrimSpace(c.Name) == "" || len(c.BotIDs) == 0 || len(c.BotIDs) > 8 || !sort.StringsAreSorted(c.BotIDs) {
		return false
	}
	for i, id := range c.BotIDs {
		if (i > 0 && c.BotIDs[i-1] == id) || s.validInput(Input{id, "channel-fixture-validation", "validate", "foreground"}) != nil {
			return false
		}
	}
	return true
}
func (s *Store) validChannelEvent(e Event) bool {
	if e.Channel == nil || e.Bot != nil || !reflect.DeepEqual(e.Work, Work{}) || !validAccessNonce(e.Nonce) || !s.validChannel(*e.Channel) {
		return false
	}
	if _, exists := s.channelNonces[e.Nonce]; exists {
		return false
	}
	c := *e.Channel
	old, exists := s.channels[c.ID]
	if e.Type == "channel.created" {
		return !exists && c.Revision == 1 && len(s.channels) < 32
	}
	return e.Type == "channel.updated" && exists && old.Revision < ^uint64(0) && c.Revision == old.Revision+1
}
func (s *Store) SaveChannel(in ChannelInput) (Channel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.aclWriteAllowed(in.HumanACL) {
		return Channel{}, false, ErrForbidden
	}
	return s.saveChannelLocked(in, false)
}
func (s *Store) saveChannelLocked(in ChannelInput, trusted bool) (Channel, bool, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.BotIDs = append([]string{}, in.BotIDs...)
	sort.Strings(in.BotIDs)
	in.HumanACL = normalizeHumanACL(in.HumanACL)
	if in.HumanACL == nil && in.ID != "" {
		in.HumanACL = copyHumanACL(s.channels[in.ID].HumanACL)
	}
	if !validAccessNonce(in.ClientNonce) {
		return Channel{}, false, ErrInvalid
	}
	if prior, ok := s.channelNonces[in.ClientNonce]; ok {
		c := prior.Channel
		if !reflect.DeepEqual(c.HumanACL, in.HumanACL) || c.Name != in.Name || !reflect.DeepEqual(c.BotIDs, in.BotIDs) || (in.ID != "" && in.ID != c.ID) || in.ExpectedRevision+1 != c.Revision {
			return Channel{}, false, ErrConflict
		}
		current := s.channels[c.ID]
		if current.Revision != c.Revision {
			return Channel{}, false, ErrConflict
		}
		return cloneChannel(current), true, nil
	}
	if len(s.channelNonces) >= 512 {
		return Channel{}, false, ErrUnavailable
	}
	id, revision, typ := in.ID, uint64(1), "channel.created"
	if id == "" {
		if in.ExpectedRevision != 0 {
			return Channel{}, false, ErrConflict
		}
		var err error
		id, err = newID("channel-")
		if err != nil {
			return Channel{}, false, err
		}
	} else {
		old, ok := s.channels[id]
		if !ok || old.Revision != in.ExpectedRevision || old.Revision == ^uint64(0) {
			return Channel{}, false, ErrConflict
		}
		revision = old.Revision + 1
		typ = "channel.updated"
	}
	c := Channel{ID: id, Name: in.Name, OwnerUserID: "local-owner", SpaceID: "local-space", Revision: revision, BotIDs: in.BotIDs, HumanACL: in.HumanACL}
	e := Event{Type: typ, Channel: &c, Nonce: in.ClientNonce}
	if !s.validChannelEvent(e) {
		return Channel{}, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return Channel{}, false, err
	}
	if err := s.suspendInvalidPublication(); err != nil {
		return Channel{}, false, err
	}
	return cloneChannel(c), false, nil
}
func (s *Store) publicationAllowed(w Work) bool {
	if w.Publication == nil {
		return true
	}
	p := w.Publication
	c, ok := s.channel(p.SourceChannelID)
	if !ok || !validRequesterWork(w) || (w.RequesterPin != nil && !humanCanWrite(c, *w.RequesterPin)) || p.Version != 1 || p.ActorID != requesterActor(w) || p.Platform != "nekobot-local" || p.SourceChannelID != w.ConversationID || p.ReplyChannelID != w.ConversationID || p.SourceMessageID != w.SourceMessageID || p.ReplyThreadID != "thread-"+w.SourceMessageID || p.AudienceRevision != c.Revision || w.SpaceID != c.SpaceID || w.OwnerUserID != c.OwnerUserID {
		return false
	}
	source := w.BotID
	if w.SourceBotID != "" {
		source = w.SourceBotID
	}
	if !member(c, source) {
		return false
	}
	// An owner tag in an implicit private lane admits only the current request;
	// target history still requires matching bot AND conversation in captureContext.
	return member(c, w.BotID) || (!validChannelID(c.ID) && w.RoutedAlias != "")
}
func (s *Store) validPublication(w Work) bool {
	if w.Publication == nil {
		return w.ConversationID == "conversation-"+w.BotID
	}
	return s.publicationAllowed(w)
}
func (s *Store) suspendInvalidPublication() error {
	for _, old := range s.sortedWorkLocked() {
		if !suspendible(old) || s.publicationAllowed(old) {
			continue
		}
		w := copyWork(old)
		w.Status = "suspending"
		if w.RuntimeStopped || (old.Status == "waiting_host" && w.AttemptID == "") {
			w.Status = "suspended"
		}
		w.DesiredState = "suspended"
		w.Epoch++
		w.Failure = "publication_audience_changed"
		if err := s.commit(Event{Type: "work.publication_suspended", Work: w}); err != nil {
			return err
		}
	}
	return nil
}
func validPublicationSuspension(s *Store, old, w Work) bool {
	if !suspendible(old) || s.publicationAllowed(old) || w.Epoch != old.Epoch+1 || w.DesiredState != "suspended" || w.Failure != "publication_audience_changed" {
		return false
	}
	status := "suspending"
	if old.RuntimeStopped || (old.Status == "waiting_host" && old.AttemptID == "") {
		status = "suspended"
	}
	return w.Status == status
}
func inputNonceKey(in Input, channel string) string {
	if validChannelID(channel) || privateConversationID(channel) {
		return nonceKey(channel+"|"+in.BotID, in.ClientNonce)
	}
	return nonceKey(in.BotID, in.ClientNonce)
}

func (s *Store) AcceptScoped(in Input, channelID, accountID string, revision uint64) (Work, bool, error) {
	if accountID == "" {
		if revision != 0 {
			return Work{}, false, ErrInvalid
		}
		if in.Lane == "" {
			in.Lane = "foreground"
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.acceptLocked(in, nil, channelID)
	}
	return s.acceptAccountScoped(in, channelID, accountID, revision)
}
