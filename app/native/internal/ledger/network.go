package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"
)

func (s *Store) commitNetwork(typ string, n NetworkEvent) error {
	e := Event{Sequence: uint64(len(s.events) + 1), Type: typ, Network: &n}
	if !s.validNetworkEvent(e) {
		return ErrInvalid
	}
	e.Sequence = 0
	return s.commit(e)
}

func (s *Store) RegisterNetworkSession(in NetworkSessionInput) (NetworkSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned || s.syncID == "" {
		return NetworkSession{}, false, ErrUnavailable
	}
	if in.ConversationID == "" {
		in.ConversationID = "conversation-" + in.BotID
	}
	if in.ExpectedChannelRevision == 0 && in.ConversationID == "conversation-"+in.BotID {
		in.ExpectedChannelRevision = 1
	}
	if in.DeviceID == "" {
		in.DeviceID = "server"
	}
	if in.Alias == "" {
		in.Alias = in.BotID
	}
	if in.Tier == "" {
		in.Tier = "pull"
	}
	if (privateConversationID(in.ConversationID) || in.DeviceID != "server" || s.executorRequired || s.contextAdmissionRequired || in.Scope.Principal != nil) && in.Tier != "none" {
		in.Tier = "pull"
	}
	c, ok := s.networkChannel(in.ConversationID, in.ExpectedChannelRevision, in.BotID)
	if !ok {
		return NetworkSession{}, false, ErrConflict
	}
	var source Work
	if in.Scope.SourceWorkID != "" {
		w, err := s.networkSource(in.Scope)
		if err != nil || in.DeviceID != "server" || w.BotID != in.BotID && (!validChannelID(c.ID) || w.ConversationID != c.ID || !member(c, w.BotID)) {
			return NetworkSession{}, false, ErrForbidden
		}
		source = w
	}
	release, err := s.networkScopeGuard(in.Scope, c, true)
	if err != nil {
		return NetworkSession{}, false, err
	}
	defer release()
	x := NetworkSession{ID: networkSessionID(s.syncID, in.BotID, in.ConversationID, in.DeviceID), InstanceID: s.syncID, OwnerUserID: "local-owner", SpaceID: "local-space", BotID: in.BotID, ConversationID: in.ConversationID, DeviceID: in.DeviceID, Alias: in.Alias, Tier: in.Tier, ChannelRevision: c.Revision, Revision: 1, ClientNonce: in.ClientNonce, ActorID: networkActorID(in.Scope), RequesterPin: copyRequester(in.Scope.Principal)}
	if in.Scope.SourceWorkID != "" {
		x.RequesterPin = copyRequester(source.RequesterPin)
		if x.RequesterPin != nil {
			x.ActorID = x.RequesterPin.ActorID
		}
	}
	if old, used := s.networkNonces[networkSessionNonce(x)]; used {
		expected := copyNetworkSession(*old.Network.Session)
		x.Revision = expected.Revision
		if !reflect.DeepEqual(expected, x) {
			return NetworkSession{}, false, ErrConflict
		}
		return copyNetworkSession(s.networkSessions[x.ID]), true, nil
	}
	if old, exists := s.networkSessions[x.ID]; exists {
		if old.BotID == x.BotID && old.ConversationID == x.ConversationID && old.DeviceID == x.DeviceID && old.Alias == x.Alias && old.Tier == x.Tier && old.ChannelRevision == x.ChannelRevision {
			return copyNetworkSession(old), true, nil
		}
		x.Revision = old.Revision + 1
	}
	if len(s.networkSessions) >= 1024 {
		saw := false
		for id := range s.networkSessions {
			if id == x.ID {
				saw = true
			}
		}
		if !saw {
			return NetworkSession{}, false, ErrConflict
		}
	}
	if err = s.commitNetwork("network.session", NetworkEvent{Session: &x}); err != nil {
		return NetworkSession{}, false, err
	}
	return copyNetworkSession(x), false, nil
}

func (s *Store) networkReadable(scope NetworkReadScope, c Channel) bool {
	if scope.SourceWorkID != "" {
		w, ok := s.work[scope.SourceWorkID]
		return ok && w.ConversationID == c.ID && member(c, w.BotID)
	}
	if scope.SourceEpoch != 0 || scope.SourceAttempt != "" {
		return false
	}
	if s.identity == nil {
		return scope.Principal == nil
	}
	return scope.Principal != nil && s.networkPrincipal(scope.Principal, c, false)
}

func (s *Store) networkReadGuard(scope NetworkReadScope) (func(), error) {
	if err := s.networkReadScope(scope); err != nil {
		return nil, err
	}
	if scope.SourceWorkID != "" {
		return s.executionPrincipalGuard(s.work[scope.SourceWorkID])
	}
	if s.identity != nil {
		return s.identity.Hold(*scope.Principal)
	}
	return func() {}, nil
}
func (s *Store) networkReadScope(scope NetworkReadScope) error {
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	if !networkText(scope.ActorID, 160) || scope.Principal != nil && scope.ActorID != "" && scope.ActorID != scope.Principal.ActorID {
		return ErrForbidden
	}
	if scope.SourceWorkID != "" {
		_, e := s.networkSource(scope)
		return e
	}
	if scope.SourceEpoch != 0 || scope.SourceAttempt != "" {
		return ErrForbidden
	}
	if s.identity != nil && (scope.Principal == nil || !s.identity.Valid(*scope.Principal)) {
		return ErrForbidden
	}
	if s.identity == nil && scope.Principal != nil {
		return ErrForbidden
	}
	return nil
}
func (s *Store) networkDirectory(scope NetworkReadScope) []NetworkSession {
	out := []NetworkSession{}
	for _, x := range s.networkSessions {
		c, ok := s.networkChannel(x.ConversationID, x.ChannelRevision, x.BotID)
		if ok && s.networkReadable(scope, c) {
			out = append(out, copyNetworkSession(x))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Alias == out[j].Alias {
			return out[i].ID < out[j].ID
		}
		return out[i].Alias < out[j].Alias
	})
	return out
}
func (s *Store) NetworkDirectory(scope NetworkReadScope) ([]NetworkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, e := s.networkReadGuard(scope)
	if e != nil {
		return nil, e
	}
	defer release()
	return s.networkDirectory(scope), nil
}

func (s *Store) SendNetworkMessage(in NetworkMessageInput) (NetworkMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return NetworkMessage{}, false, ErrUnavailable
	}
	to, ok := s.networkSessions[in.ToSessionID]
	if !ok {
		return NetworkMessage{}, false, ErrNotFound
	}
	c, ok := s.networkChannel(to.ConversationID, to.ChannelRevision, to.BotID)
	if !ok {
		return NetworkMessage{}, false, ErrConflict
	}
	var source Work
	if in.Scope.SourceWorkID != "" {
		var sourceErr error
		source, sourceErr = s.networkSource(in.Scope)
		if sourceErr != nil {
			return NetworkMessage{}, false, sourceErr
		}
	}
	release, err := s.networkScopeGuard(in.Scope, c, true)
	if err != nil {
		return NetworkMessage{}, false, err
	}
	defer release()
	actor := networkActorID(in.Scope)
	m := NetworkMessage{ToSessionID: to.ID, ToBotID: to.BotID, ConversationID: to.ConversationID, ChannelRevision: to.ChannelRevision, ActorID: actor, Origin: "human", FromSessionID: "human-" + networkHash(actor), Text: in.Text, Wake: in.Wake, ClientNonce: in.ClientNonce, ReplyTo: in.ReplyTo, Attachments: append([]NetworkAttachment(nil), in.Attachments...), RequesterPin: copyRequester(in.Scope.Principal)}
	if in.Scope.SourceWorkID != "" {
		w := source
		if !validChannelID(c.ID) || w.ConversationID != c.ID || !member(c, w.BotID) {
			return NetworkMessage{}, false, ErrForbidden
		}
		m.Origin = "agent"
		m.SourceWorkID = w.RequestID
		m.SourceEpoch = w.Epoch
		m.SourceAttempt = NetworkAttempt(w)
		m.FromBotID = w.BotID
		m.FromSessionID = s.networkSourceSession(w, to.InstanceID)
		m.RootWorkID = s.networkRoot(w)
		m.RequesterPin = copyRequester(w.RequesterPin)
		if m.RequesterPin != nil {
			m.ActorID = m.RequesterPin.ActorID
		}
	}
	if prior, used := s.networkNonces[networkMessageNonce(m)]; used {
		old := copyNetworkMessage(*prior.Network.Message)
		m.ID = old.ID
		m.CreatedAt = old.CreatedAt
		m.ExpiresAt = old.ExpiresAt
		m.CreatedSequence = old.CreatedSequence
		if m.Origin == "agent" && m.SourceWorkID == old.SourceWorkID && m.SourceEpoch == old.SourceEpoch {
			m.SourceAttempt = old.SourceAttempt
		}
		if !reflect.DeepEqual(old, m) {
			return NetworkMessage{}, false, ErrConflict
		}
		return copyNetworkMessage(s.networkMessages[old.ID]), true, nil
	}
	if !validAccessNonce(m.ClientNonce) || !networkText(m.Text, NetworkMaxTextBytes) || strings.TrimSpace(m.Text) == "" || !networkFilesValid(m.Attachments) {
		return NetworkMessage{}, false, ErrInvalid
	}
	m.ID, err = newID("mail-")
	if err != nil {
		return NetworkMessage{}, false, err
	}
	now := s.clock()
	m.CreatedAt = now.UTC().Format(time.RFC3339Nano)
	m.ExpiresAt = now.Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	m.CreatedSequence = uint64(len(s.events) + 1)
	if !s.networkLimits(m, now) {
		return NetworkMessage{}, false, ErrConflict
	}
	if err = s.commitNetwork("network.message", NetworkEvent{Message: &m}); err != nil {
		return NetworkMessage{}, false, err
	}
	return copyNetworkMessage(m), false, nil
}

func (s *Store) DispatchNetworkMessage(id string) (Work, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.networkMessages[id]
	if !ok {
		return Work{}, false, ErrNotFound
	}
	if workID := s.networkWork[id]; workID != "" {
		return copyWork(s.work[workID]), true, nil
	}
	if m.Origin == "agent" && (s.executorRequired || s.contextAdmissionRequired) {
		return Work{}, false, ErrUnavailable
	}
	expiry, _ := networkTime(m.ExpiresAt)
	if !m.Wake || !expiry.After(s.clock()) || s.networkContext[id] != "" {
		return Work{}, false, ErrConflict
	}
	c, ok := s.networkChannel(m.ConversationID, m.ChannelRevision, m.ToBotID)
	if !ok {
		return Work{}, false, ErrConflict
	}
	release, err := s.intakePrincipalGuard(m.RequesterPin, c.ID, &c.Revision)
	if err != nil {
		return Work{}, false, err
	}
	defer release()
	return s.acceptLocked(Input{BotID: m.ToBotID, ClientNonce: "network-" + m.ID, Text: networkWorkText(m), Lane: "foreground"}, nil, c.ID, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision, requester: copyRequester(m.RequesterPin), peerMessageID: m.ID})
}

func (s *Store) NetworkPendingWakeIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{}
	now := s.clock()
	for id, m := range s.networkMessages {
		target := s.networkSessions[m.ToSessionID]
		if target.DeviceID != "server" || target.Tier != "turn" || !(target.ConversationID == "conversation-"+target.BotID || validChannelID(target.ConversationID)) {
			continue
		}
		until, _ := networkTime(m.ExpiresAt)
		if m.Wake && s.networkWork[id] == "" && s.networkContext[id] == "" && until.After(now) {
			if c, ok := s.networkChannel(m.ConversationID, m.ChannelRevision, m.ToBotID); ok && s.networkDeliveryPrincipalCurrent(m, c) {
				out = append(out, id)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return s.networkMessages[out[i]].CreatedSequence < s.networkMessages[out[j]].CreatedSequence
	})
	return out
}
func (s *Store) NetworkPendingContextIDs(workID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.networkPendingContextIDs(workID)
}
func (s *Store) networkPendingContextIDs(workID string) ([]string, error) {
	w, ok := s.work[workID]
	if !ok {
		return nil, ErrNotFound
	}
	if !(w.Status == "running" || w.Status == "waiting_host") || w.Publication == nil || !s.activeExecutionAllowed(w) {
		return nil, ErrConflict
	}
	c, ok := s.networkChannel(w.ConversationID, w.Publication.AudienceRevision, w.BotID)
	if !ok {
		return nil, ErrConflict
	}
	out := []string{}
	for id, m := range s.networkMessages {
		expires, _ := networkTime(m.ExpiresAt)
		if !m.Wake && s.networkSessions[m.ToSessionID].DeviceID == "server" && s.networkContext[id] == "" && s.networkWork[id] == "" && m.ToBotID == w.BotID && m.ConversationID == w.ConversationID && m.ChannelRevision == w.Publication.AudienceRevision && expires.After(s.clock()) {
			if s.networkDeliveryPrincipalCurrent(m, c) {
				out = append(out, id)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return s.networkMessages[out[i]].CreatedSequence < s.networkMessages[out[j]].CreatedSequence
	})
	if len(out) > 32 {
		out = out[:32]
	}
	return out, nil
}
func (s *Store) DeliverNetworkContext(workID string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[workID]
	if !ok {
		return ErrNotFound
	}
	left := []string{}
	messages := []NetworkMessage{}
	for _, id := range ids {
		if s.networkContext[id] == workID {
			continue
		}
		left = append(left, id)
		if m, ok := s.networkMessages[id]; ok {
			messages = append(messages, m)
		} else {
			return ErrNotFound
		}
	}
	if len(left) == 0 {
		return nil
	}
	release, err := s.networkExecutionHold(w, messages)
	if err != nil {
		return err
	}
	defer release()
	sort.Strings(left)
	d := NetworkDelivery{WorkID: workID, MessageIDs: left, At: s.clock().UTC().Format(time.RFC3339Nano)}
	return s.commitNetwork("network.context", NetworkEvent{Delivery: &d})
}
func (s *Store) NetworkContext(workID string) ([]NetworkMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.work[workID]
	if !ok {
		return nil, ErrNotFound
	}
	if w.Publication == nil {
		return nil, ErrForbidden
	}
	if _, ok := s.networkChannel(w.ConversationID, w.Publication.AudienceRevision, w.BotID); !ok {
		return nil, ErrForbidden
	}
	out := []NetworkMessage{}
	for id, m := range s.networkMessages {
		if w.PeerMessageID == id || s.networkContext[id] == workID {
			if m.ConversationID != w.ConversationID || m.ToBotID != w.BotID || m.ChannelRevision != w.Publication.AudienceRevision {
				return nil, ErrConflict
			}
			out = append(out, copyNetworkMessage(m))
		}
	}
	release, err := s.networkExecutionHold(w, out)
	if err != nil {
		return nil, err
	}
	defer release()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedSequence < out[j].CreatedSequence })
	return out, nil
}

func (s *Store) networkCanSeeMessage(scope NetworkReadScope, m NetworkMessage) bool {
	c, ok := s.channel(m.ConversationID)
	if !ok || !s.networkReadable(scope, c) {
		return false
	}
	if scope.SourceWorkID != "" {
		w := s.work[scope.SourceWorkID]
		return w.Publication != nil && m.ChannelRevision == w.Publication.AudienceRevision && (m.FromBotID == w.BotID || m.ToBotID == w.BotID)
	}
	return true
}
func (s *Store) networkView(scope NetworkReadScope, m NetworkMessage) NetworkMessageView {
	v := NetworkMessageView{NetworkMessage: copyNetworkMessage(m), DeliveryState: "queued", ReadAt: s.networkReads[m.ID+"|"+networkActorID(scope)]}
	v.WorkID = s.networkWork[m.ID]
	if v.WorkID == "" {
		v.WorkID = s.networkContext[m.ID]
		if v.WorkID != "" {
			v.DeliveryState = "delivered"
		}
	}
	if v.WorkID != "" {
		w := s.work[v.WorkID]
		v.TaskState = w.Status
		if w.Direct != nil || w.Core != nil || w.Status == "completed" || w.Status == "failed" {
			v.DeliveryState = "delivered"
		}
		v.Approval = s.networkApproval(w)
	} else {
		expiry, _ := networkTime(m.ExpiresAt)
		if !expiry.After(s.clock()) {
			v.DeliveryState = "expired"
		} else if _, ok := s.networkChannel(m.ConversationID, m.ChannelRevision, m.ToBotID); !ok {
			v.DeliveryState = "blocked"
		}
	}
	return v
}
func (s *Store) networkApproval(w Work) *NetworkApproval {
	if w.Direct == nil || w.Direct.Runtime == "imported" || w.Direct.Approval == nil {
		return nil
	}
	a := w.Direct.Approval
	if a.DecidedAt == "" || a.State != "approved" && a.State != "declined" {
		return nil
	}
	var prior *DirectApproval
	verified := false
	for _, e := range s.events {
		if e.Work.RequestID != w.RequestID || e.Work.Direct == nil {
			continue
		}
		next := e.Work.Direct.Approval
		if e.Type == "work.direct_decided" && prior != nil && prior.State == "pending" && next != nil && next.State == a.State && next.DecidedAt == a.DecidedAt && next.DecidedAt != "" {
			verified = true
		}
		if next != nil {
			p := *next
			prior = &p
		}
	}
	if !verified {
		return nil
	}
	data, _ := json.Marshal(struct{ WorkID, MessageID, Title, Kind, Body, To, Subject, Use, State, DecidedAt string }{w.RequestID, w.PeerMessageID, a.Title, a.Kind, a.Body, a.To, a.Subject, a.Use, a.State, a.DecidedAt})
	digest := sha256.Sum256(data)
	return &NetworkApproval{WorkID: w.RequestID, State: a.State, DecidedAt: a.DecidedAt, Digest: hex.EncodeToString(digest[:])}
}
func (s *Store) NetworkSnapshot(scope NetworkReadScope) (NetworkSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.networkReadGuard(scope)
	if err != nil {
		return NetworkSnapshot{}, err
	}
	defer release()
	out := NetworkSnapshot{Sessions: s.networkDirectory(scope), Messages: []NetworkMessageView{}}
	for _, m := range s.networkMessages {
		if s.networkCanSeeMessage(scope, m) {
			out.Messages = append(out.Messages, s.networkView(scope, m))
		}
	}
	sort.Slice(out.Messages, func(i, j int) bool { return out.Messages[i].CreatedSequence < out.Messages[j].CreatedSequence })
	return out, nil
}
func (s *Store) NetworkInbox(scope NetworkReadScope, sessionID string) ([]NetworkMessageView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.networkReadGuard(scope)
	if err != nil {
		return nil, err
	}
	defer release()
	x, ok := s.networkSessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	c, ok := s.channel(x.ConversationID)
	if !ok || !s.networkReadable(scope, c) {
		return nil, ErrForbidden
	}
	out := []NetworkMessageView{}
	for _, m := range s.networkMessages {
		if m.ToSessionID == sessionID && s.networkCanSeeMessage(scope, m) {
			out = append(out, s.networkView(scope, m))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedSequence < out[j].CreatedSequence })
	return out, nil
}
func (s *Store) networkControl(id, nonce, action string, scope NetworkReadScope) (NetworkMessageView, bool, error) {
	if scope.SourceWorkID != "" || scope.SourceEpoch != 0 || scope.SourceAttempt != "" {
		return NetworkMessageView{}, false, ErrForbidden
	}
	m, ok := s.networkMessages[id]
	if !ok {
		return NetworkMessageView{}, false, ErrNotFound
	}
	c, ok := s.networkChannel(m.ConversationID, m.ChannelRevision, m.ToBotID)
	if !ok {
		return NetworkMessageView{}, false, ErrConflict
	}
	release, err := s.networkScopeGuard(scope, c, action == "wake")
	if err != nil {
		return NetworkMessageView{}, false, err
	}
	defer release()
	x := NetworkControl{MessageID: id, Action: action, ActorID: networkActorID(scope), ClientNonce: nonce, At: s.clock().UTC().Format(time.RFC3339Nano), RequesterPin: copyRequester(scope.Principal)}
	if old, exists := s.networkNonces[networkControlNonce(x)]; exists {
		prior := old.Network.Control
		x.At = prior.At
		if !reflect.DeepEqual(*prior, x) {
			return NetworkMessageView{}, false, ErrConflict
		}
		return s.networkView(scope, s.networkMessages[id]), true, nil
	}
	if err = s.commitNetwork("network.control", NetworkEvent{Control: &x}); err != nil {
		return NetworkMessageView{}, false, err
	}
	return s.networkView(scope, s.networkMessages[id]), false, nil
}
func (s *Store) WakeNetworkMessage(id, nonce string, scope NetworkReadScope) (NetworkMessage, bool, error) {
	s.mu.Lock()
	m, ok := s.networkMessages[id]
	if !ok {
		s.mu.Unlock()
		return NetworkMessage{}, false, ErrNotFound
	}
	if scope.SourceWorkID != "" || m.Origin != "human" || m.Wake || m.ActorID != networkActorID(scope) {
		s.mu.Unlock()
		return NetworkMessage{}, false, ErrForbidden
	}
	c, ok := s.networkChannel(m.ConversationID, m.ChannelRevision, m.ToBotID)
	if !ok {
		s.mu.Unlock()
		return NetworkMessage{}, false, ErrConflict
	}
	release, err := s.networkScopeGuard(scope, c, true)
	if err != nil {
		s.mu.Unlock()
		return NetworkMessage{}, false, err
	}
	release()
	copy := copyNetworkMessage(m)
	s.mu.Unlock()
	return s.SendNetworkMessage(NetworkMessageInput{ToSessionID: copy.ToSessionID, Text: copy.Text, ClientNonce: nonce, ReplyTo: copy.ID, Wake: true, Attachments: copy.Attachments, Scope: scope})
}
func (s *Store) ReadNetworkMessage(id, nonce string, scope NetworkReadScope) (NetworkMessageView, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.networkControl(id, nonce, "read", scope)
}
