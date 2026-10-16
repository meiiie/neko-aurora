package ledger

import (
	"errors"
	"nekobot.local/native/internal/identity"
	"sort"
)

var ErrForbidden = errors.New("principal_forbidden")

type HumanMember struct {
	ActorID string `json:"actorId"`
	Role    string `json:"role"`
}
type HumanACL struct {
	Version     int           `json:"version"`
	AuthorityID string        `json:"authorityId"`
	Members     []HumanMember `json:"members"`
}

func copyHumanACL(a *HumanACL) *HumanACL {
	if a == nil {
		return nil
	}
	out := *a
	out.Members = append([]HumanMember{}, a.Members...)
	return &out
}
func validHumanACL(a *HumanACL) bool {
	if a == nil {
		return true
	}
	if a.Version != 1 || !manifestID.MatchString(a.AuthorityID) || len(a.Members) > 16 {
		return false
	}
	last := ""
	for _, m := range a.Members {
		if !manifestID.MatchString(m.ActorID) || m.ActorID == "local-owner" || m.ActorID <= last || (m.Role != "reader" && m.Role != "writer") {
			return false
		}
		last = m.ActorID
	}
	return true
}
func humanRole(c Channel, p identity.VerifiedPrincipal) string {
	if p.SpaceID != c.SpaceID {
		return ""
	}
	if p.ActorID == c.OwnerUserID {
		return "admin"
	}
	if !validChannelID(c.ID) || c.HumanACL == nil || c.HumanACL.AuthorityID != p.AuthorityID {
		return ""
	}
	for _, m := range c.HumanACL.Members {
		if m.ActorID == p.ActorID {
			return m.Role
		}
	}
	return ""
}
func humanCanRead(c Channel, p identity.VerifiedPrincipal) bool { return humanRole(c, p) != "" }
func humanCanWrite(c Channel, p identity.VerifiedPrincipal) bool {
	r := humanRole(c, p)
	return r == "admin" || r == "writer"
}
func copyRequester(p *identity.VerifiedPrincipal) *identity.VerifiedPrincipal {
	if p == nil {
		return nil
	}
	out := *p
	return &out
}
func requesterActor(w Work) string {
	if w.RequesterPin != nil {
		return w.RequesterPin.ActorID
	}
	return w.OwnerUserID
}
func originalBot(w Work) string {
	if w.SourceBotID != "" {
		return w.SourceBotID
	}
	return w.BotID
}
func scopeNonceKey(in Input, channel string, p *identity.VerifiedPrincipal) string {
	if p != nil {
		return identity.ActorNamespace(*p) + "|" + in.ClientNonce
	}
	return inputNonceKey(in, channel)
}
func (s *Store) findInputNonce(in Input, channel string, p *identity.VerifiedPrincipal) (string, bool) {
	if id, ok := s.nonces[scopeNonceKey(in, channel, p)]; ok {
		return id, true
	}
	if p != nil && p.ActorID == "local-owner" {
		id, ok := s.nonces[inputNonceKey(in, channel)]
		if ok && s.work[id].RequesterPin == nil {
			return id, true
		}
	}
	return "", false
}
func validRequesterWork(w Work) bool {
	if w.RequesterPin == nil {
		return true
	}
	p := *w.RequesterPin
	if p.ActorID != "local-owner" && (w.BotProfile != nil || w.ExpectedBotProfileRevision != nil || w.UseBotDefaults || w.ExpectedBotStateRevision != nil || w.BotState != nil || w.Account != nil || w.Resources != nil || w.PluginSelection != nil || w.Lane != "foreground") {
		return false
	}
	return identity.ValidPrincipal(p) && p.SpaceID == w.SpaceID && w.OwnerUserID == "local-owner" && w.Collaboration == nil && w.Adapter == nil && w.Publication != nil && w.Publication.ActorID == p.ActorID && (p.ActorID == "local-owner" || validChannelID(w.ConversationID))
}
func (s *Store) ConfigureIdentity(a *identity.Authority) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identity = a
}
func (s *Store) activeExecutionAllowed(w Work) bool {
	if !s.coreProfileCurrent(w) {
		return false
	}
	if w.RequesterPin == nil {
		return s.identity == nil
	}
	return s.identity != nil && s.identity.Valid(*w.RequesterPin) && s.publicationAllowed(w)
}
func (s *Store) executionPrincipalGuard(w Work) (func(), error) {
	if err := s.coreProfileGuard(w); err != nil {
		return nil, err
	}
	if w.RequesterPin == nil {
		if s.identity != nil {
			return nil, ErrForbidden
		}
		return func() {}, nil
	}
	if s.identity == nil || !s.publicationAllowed(w) {
		return nil, ErrForbidden
	}
	return s.identity.Hold(*w.RequesterPin)
}
func (s *Store) intakePrincipalGuard(p *identity.VerifiedPrincipal, channel string, revision *uint64) (func(), error) {
	if p == nil {
		if s.identity != nil {
			return nil, ErrForbidden
		}
		return func() {}, nil
	}
	if s.identity == nil {
		return nil, ErrForbidden
	}
	c, ok := s.channel(channel)
	if !ok || !humanCanWrite(c, *p) {
		return nil, ErrForbidden
	}
	if validChannelID(channel) && (revision == nil || *revision != c.Revision) {
		return nil, ErrConflict
	}
	return s.identity.Hold(*p)
}
func (s *Store) AcceptFor(p identity.VerifiedPrincipal, in Input, scope SubmissionScope) (Work, bool, error) {
	if p.ActorID != "local-owner" && (scope.UseBotDefaults || scope.ExpectedBotStateRevision != nil || scope.ExpectedBotProfileRevision != nil || scope.AccountID != "" || scope.AccountRevision != 0 || scope.ProjectID != "" || scope.ProjectRevision != 0 || len(scope.FileIDs) > 0 || scope.ExpectedContinuityRevision != nil || scope.PluginSelection != nil || scope.Adapter != nil || in.Lane != "" && in.Lane != "foreground") {
		return Work{}, false, ErrForbidden
	}
	if !identity.ValidPrincipal(p) || scope.Adapter != nil {
		return Work{}, false, ErrForbidden
	}
	if scope.ChannelID == "" && p.ActorID == "local-owner" {
		scope.ChannelID = "conversation-" + in.BotID
	}
	scope.requester = copyRequester(&p)
	return s.AcceptProduct(in, scope)
}
func validPrincipalSuspension(old, w Work) bool {
	if !suspendible(old) || w.Epoch != old.Epoch+1 || w.DesiredState != "suspended" || w.Failure != "requester_authority_changed" {
		return false
	}
	status := "suspending"
	if old.RuntimeStopped || old.Status == "waiting_host" && old.AttemptID == "" {
		status = "suspended"
	}
	return w.Status == status
}
func (s *Store) ReconcilePrincipals() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, old := range s.sortedWorkLocked() {
		if !suspendible(old) || s.activeExecutionAllowed(old) {
			continue
		}
		w := copyWork(old)
		w.Status = "suspending"
		if old.RuntimeStopped || old.Status == "waiting_host" && old.AttemptID == "" {
			w.Status = "suspended"
		}
		w.DesiredState = "suspended"
		w.Epoch++
		w.Failure = "requester_authority_changed"
		if err := s.commit(Event{Type: "work.principal_suspended", Work: w}); err != nil {
			return err
		}
	}
	return nil
}
func normalizeHumanACL(a *HumanACL) *HumanACL {
	out := copyHumanACL(a)
	if out != nil {
		sort.Slice(out.Members, func(i, j int) bool { return out.Members[i].ActorID < out.Members[j].ActorID })
	}
	return out
}
func (s *Store) aclWriteAllowed(a *HumanACL) bool {
	if a == nil {
		return true
	}
	if s.identity == nil || a.AuthorityID != s.identity.AuthorityID() || !validHumanACL(a) {
		return false
	}
	for _, m := range a.Members {
		if !s.identity.KnownActor(m.ActorID) {
			return false
		}
	}
	return true
}
func (s *Store) SessionCurrent(p identity.VerifiedPrincipal) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identity != nil && s.identity.Valid(p)
}
func (s *Store) ReadScopeCurrent(p identity.VerifiedPrincipal, channel string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.channel(channel)
	return ok && s.identity != nil && s.identity.Valid(p) && humanCanRead(c, p)
}
func (s *Store) SaveChannelFor(p identity.VerifiedPrincipal, in ChannelInput) (Channel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	in.HumanACL = normalizeHumanACL(in.HumanACL)
	if p.ActorID != "local-owner" || s.identity == nil || !s.aclWriteAllowed(in.HumanACL) {
		return Channel{}, false, ErrForbidden
	}
	release, err := s.identity.Hold(p)
	if err != nil {
		return Channel{}, false, err
	}
	defer release()
	return s.saveChannelLocked(in, true)
}
func (s *Store) DeliverReportFor(p identity.VerifiedPrincipal, in ReportDeliveryInput) (ReportDelivery, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ActorID != "local-owner" || s.identity == nil {
		return ReportDelivery{}, false, ErrForbidden
	}
	release, err := s.identity.Hold(p)
	if err != nil {
		return ReportDelivery{}, false, err
	}
	defer release()
	return s.deliverReportLocked(in)
}
func (s *Store) ownerPrincipalGuard(p identity.VerifiedPrincipal) (func(), error) {
	if p.ActorID != "local-owner" || s.identity == nil {
		return nil, ErrForbidden
	}
	return s.identity.Hold(p)
}
func (s *Store) SetBotProfileFor(p identity.VerifiedPrincipal, id string, in BotProfileInput) (BotProfile, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.ownerPrincipalGuard(p)
	if err != nil {
		return BotProfile{}, false, err
	}
	defer release()
	return s.setBotProfileLocked(id, in)
}
func (s *Store) RenameBotFor(p identity.VerifiedPrincipal, id string, in BotNameInput) (Bot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.ownerPrincipalGuard(p)
	if err != nil {
		return Bot{}, false, err
	}
	defer release()
	return s.renameBotLocked(id, in)
}
func (s *Store) CreateBotFor(p identity.VerifiedPrincipal, in BotInput) (Bot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.ownerPrincipalGuard(p)
	if err != nil {
		return Bot{}, false, err
	}
	defer release()
	return s.createBotLocked(in)
}
func (s *Store) ControlBotFor(p identity.VerifiedPrincipal, in BotControlInput) (BotState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.ownerPrincipalGuard(p)
	if err != nil {
		return BotState{}, false, err
	}
	defer release()
	return s.controlBotLocked(in)
}
