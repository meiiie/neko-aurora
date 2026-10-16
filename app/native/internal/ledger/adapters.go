package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"regexp"
	"sort"

	"nekobot.local/native/internal/executors"
)

type AdapterSession struct {
	ID              string `json:"id"`
	Version         int    `json:"version"`
	OwnerUserID     string `json:"ownerUserId"`
	SpaceID         string `json:"spaceId"`
	SourceApp       string `json:"sourceApp"`
	SourceSessionID string `json:"sourceSessionId"`
	SourceThreadID  string `json:"sourceThreadId"`
	ChannelID       string `json:"channelId"`
	ChannelRevision uint64 `json:"channelRevision"`
	ReturnRoute     string `json:"returnRoute"`
}
type AdapterSessionInput struct {
	SourceApp               string `json:"sourceApp"`
	SourceSessionID         string `json:"sourceSessionId"`
	SourceThreadID          string `json:"sourceThreadId"`
	ChannelID               string `json:"channelId"`
	ExpectedChannelRevision uint64 `json:"expectedChannelRevision"`
	ClientNonce             string `json:"clientNonce"`
}
type AdapterPin struct {
	AdapterSessionID string `json:"adapterSessionId"`
	SourceApp        string `json:"sourceApp"`
	SourceSessionID  string `json:"sourceSessionId"`
	SourceThreadID   string `json:"sourceThreadId"`
	SourceMessageID  string `json:"sourceMessageId"`
	ClientNonce      string `json:"clientNonce"`
	ChannelRevision  uint64 `json:"channelRevision"`
	ReturnRoute      string `json:"returnRoute"`
}
type AdapterMessageInput struct {
	Input
	SourceMessageID         string                `json:"sourceMessageId"`
	AccountID               string                `json:"accountId,omitempty"`
	AccountRevision         uint64                `json:"accountRevision,omitempty"`
	ProjectID               string                `json:"projectId,omitempty"`
	ProjectRevision         uint64                `json:"projectRevision,omitempty"`
	FileIDs                 []string              `json:"fileIds,omitempty"`
	ExpectedInstallationPin *executors.InstallPin `json:"expectedInstallationPin,omitempty"`
}
type AdapterReply struct {
	ReplyID         string     `json:"replyId"`
	RequestID       string     `json:"requestId"`
	SourceMessageID string     `json:"sourceMessageId"`
	BotID           string     `json:"botId"`
	Role            string     `json:"role"`
	Text            string     `json:"text"`
	Status          string     `json:"status"`
	Sequence        uint64     `json:"sequence"`
	Artifacts       []Artifact `json:"artifacts,omitempty"`
}
type AdapterPage struct {
	AdapterSessionID string         `json:"adapterSessionId"`
	SourceApp        string         `json:"sourceApp"`
	SourceSessionID  string         `json:"sourceSessionId"`
	SourceThreadID   string         `json:"sourceThreadId"`
	ChannelID        string         `json:"channelId"`
	OwnerUserID      string         `json:"ownerUserId"`
	SpaceID          string         `json:"spaceId"`
	ChannelRevision  uint64         `json:"channelRevision"`
	ReturnRoute      string         `json:"returnRoute"`
	Cursor           uint64         `json:"cursor"`
	Replies          []AdapterReply `json:"replies"`
}

var adapterRef = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,160}$`)

type AdapterCapabilities struct {
	Version      int      `json:"version"`
	Capabilities []string `json:"capabilities"`
}

func (s *Store) AdapterCapabilities() (AdapterCapabilities, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return AdapterCapabilities{}, ErrUnavailable
	}
	return AdapterCapabilities{Version: 1, Capabilities: []string{"local_adapter_installation_intake_cas_v1", "local_adapter_history_repair_v1"}}, nil
}

func adapterNonce(session, nonce string) string {
	h := sha256.Sum256([]byte(session + "\x00" + nonce))
	return "adapter-" + hex.EncodeToString(h[:])
}
func (s *Store) validAdapterEvent(e Event) bool {
	if e.Type != "adapter.created" || e.AdapterSession == nil || e.Bot != nil || e.Channel != nil || e.Project != nil || e.Upload != nil || e.ResourceRevision != 0 || e.ContextRefreshVersion != 0 || e.CompletionVersion != 0 || !reflect.DeepEqual(e.Work, Work{}) || !validAccessNonce(e.Nonce) || len(s.adapters) >= 256 {
		return false
	}
	a := *e.AdapterSession
	c, ok := s.channel(a.ChannelID)
	if !ok || !resourceID(a.ID, "adapter-") || a.Version != 1 || a.OwnerUserID != "local-owner" || a.SpaceID != "local-space" || (a.SourceApp != "wiii" && a.SourceApp != "core-tui") || !adapterRef.MatchString(a.SourceSessionID) || !adapterRef.MatchString(a.SourceThreadID) || a.ChannelRevision != c.Revision || a.ReturnRoute != "/v1/adapters/sessions/"+a.ID+"/replies" {
		return false
	}
	if _, ok := s.adapters[a.ID]; ok {
		return false
	}
	_, exists := s.adapterNonces[e.Nonce]
	return !exists
}
func (s *Store) RegisterAdapter(in AdapterSessionInput) (AdapterSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validAccessNonce(in.ClientNonce) {
		return AdapterSession{}, false, ErrInvalid
	}
	if e, ok := s.adapterNonces[in.ClientNonce]; ok {
		a := *e.AdapterSession
		if a.SourceApp != in.SourceApp || a.SourceSessionID != in.SourceSessionID || a.SourceThreadID != in.SourceThreadID || a.ChannelID != in.ChannelID || a.ChannelRevision != in.ExpectedChannelRevision {
			return AdapterSession{}, false, ErrConflict
		}
		return a, true, nil
	}
	c, ok := s.channel(in.ChannelID)
	if !ok {
		return AdapterSession{}, false, ErrNotFound
	}
	if c.Revision != in.ExpectedChannelRevision {
		return AdapterSession{}, false, ErrConflict
	}
	id, err := newID("adapter-")
	if err != nil {
		return AdapterSession{}, false, err
	}
	a := AdapterSession{ID: id, Version: 1, OwnerUserID: "local-owner", SpaceID: "local-space", SourceApp: in.SourceApp, SourceSessionID: in.SourceSessionID, SourceThreadID: in.SourceThreadID, ChannelID: c.ID, ChannelRevision: c.Revision, ReturnRoute: "/v1/adapters/sessions/" + id + "/replies"}
	e := Event{Type: "adapter.created", AdapterSession: &a, Nonce: in.ClientNonce}
	if !s.validAdapterEvent(e) {
		return AdapterSession{}, false, ErrInvalid
	}
	if err = s.commit(e); err != nil {
		return AdapterSession{}, false, err
	}
	return a, false, nil
}
func (s *Store) validAdapterPin(w Work) bool {
	if w.Adapter == nil {
		return true
	}
	p := w.Adapter
	a, ok := s.adapters[p.AdapterSessionID]
	if !ok || p.SourceApp != a.SourceApp || p.SourceSessionID != a.SourceSessionID || p.SourceThreadID != a.SourceThreadID || !adapterRef.MatchString(p.SourceMessageID) || !validAccessNonce(p.ClientNonce) || p.ChannelRevision != a.ChannelRevision || p.ReturnRoute != a.ReturnRoute || w.ConversationID != a.ChannelID || w.OwnerUserID != a.OwnerUserID || w.SpaceID != a.SpaceID || w.ClientNonce != adapterNonce(a.ID, p.ClientNonce) || w.Publication == nil || w.Publication.AudienceRevision != a.ChannelRevision {
		return false
	}
	for _, old := range s.work {
		if old.RequestID != w.RequestID && old.Adapter != nil && old.Adapter.AdapterSessionID == p.AdapterSessionID && old.Adapter.SourceMessageID == p.SourceMessageID {
			return false
		}
	}
	return true
}
func (s *Store) AcceptAdapter(id string, in AdapterMessageInput) (Work, bool, error) {
	s.mu.Lock()
	a, ok := s.adapters[id]
	if !ok {
		s.mu.Unlock()
		return Work{}, false, ErrNotFound
	}
	if !adapterRef.MatchString(in.SourceMessageID) || !validAccessNonce(in.ClientNonce) {
		s.mu.Unlock()
		return Work{}, false, ErrInvalid
	}
	for _, old := range s.work {
		if old.Adapter != nil && old.Adapter.AdapterSessionID == id && old.Adapter.SourceMessageID == in.SourceMessageID && old.Adapter.ClientNonce != in.ClientNonce {
			s.mu.Unlock()
			return Work{}, false, ErrConflict
		}
	}
	s.mu.Unlock()
	pin := &AdapterPin{AdapterSessionID: id, SourceApp: a.SourceApp, SourceSessionID: a.SourceSessionID, SourceThreadID: a.SourceThreadID, SourceMessageID: in.SourceMessageID, ClientNonce: in.ClientNonce, ChannelRevision: a.ChannelRevision, ReturnRoute: a.ReturnRoute}
	input := in.Input
	input.ClientNonce = adapterNonce(a.ID, in.ClientNonce)
	var expected *executors.InstallPin
	if in.ExpectedInstallationPin != nil {
		p := *in.ExpectedInstallationPin
		expected = &p
	}
	return s.AcceptProduct(input, SubmissionScope{ChannelID: a.ChannelID, ExpectedChannelRevision: &a.ChannelRevision, AccountID: in.AccountID, AccountRevision: in.AccountRevision, ProjectID: in.ProjectID, ProjectRevision: in.ProjectRevision, FileIDs: in.FileIDs, Adapter: pin, ExpectedInstallationPin: expected})
}

func (s *Store) adapterInvalidationEvent(w Work, e Event) bool {
	type dependency struct {
		work   Work
		access uint64
		pinned bool
	}
	stack := []dependency{{work: w}}
	seen := map[string]bool{}
	for len(stack) > 0 {
		d := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[d.work.RequestID] {
			continue
		}
		seen[d.work.RequestID] = true
		current := d.work
		if e.Type == "work.source_access" && e.Work.RequestID == current.RequestID && e.Work.ContextAccess != nil && (!e.Work.ContextAccess.Granted || d.pinned && e.Work.ContextAccess.Revision != d.access) {
			return true
		}
		if e.Channel != nil && current.Publication != nil && e.Channel.ID == current.ConversationID && e.Channel.Revision != current.Publication.AudienceRevision {
			return true
		}
		if r := current.Resources; r != nil {
			if e.Project != nil && e.Project.ID == r.Project.ID && e.Project.Revision != r.Project.Revision {
				return true
			}
			if e.ProjectContinuity != nil && e.ProjectContinuity.ProjectID == r.Project.ID && e.ProjectContinuity.ChannelID == current.ConversationID {
				expected := uint64(0)
				if r.Continuity != nil {
					expected = r.Continuity.Revision
				}
				if e.ProjectContinuity.Revision != expected {
					return true
				}
			}
			if e.Type == "file.access" && e.Upload != nil {
				for _, f := range r.Files {
					if e.Upload.ID == f.ID && (!e.Upload.Granted || e.Upload.AccessRevision != f.AccessRevision || e.Upload.SHA256 != f.SHA256) {
						return true
					}
				}
				if r.Continuity != nil && r.Continuity.Metadata != nil {
					for _, ref := range r.Continuity.Metadata.References {
						if f := ref.File; f != nil && e.Upload.ID == f.FileID && (!e.Upload.Granted || e.Upload.AccessRevision != f.AccessRevision || e.Upload.SHA256 != f.SHA256) {
							return true
						}
					}
				}
			}
		}
		if e.Plugin != nil {
			for _, p := range effectivePluginPins(current) {
				if e.Plugin.Plugin.ID == p.ID && (e.Plugin.Plugin.Status != "ready_builtin_local" || current.ToolPolicyVersion != 0 && e.Plugin.Plugin.Revision != p.Revision) {
					return true
				}
			}
		}
		if current.Context != nil {
			for _, pin := range current.Context.Sources {
				if source, ok := s.work[pin.RequestID]; ok {
					stack = append(stack, dependency{work: source, access: pin.AccessRevision, pinned: true})
				}
			}
			for _, pin := range current.Context.PublishedReports {
				if e.BotState != nil && (e.BotState.BotID == current.BotID && e.BotState.Archived || pin.Source != nil && e.BotState.BotID == pin.BotID && e.BotState.Revision != pin.Source.BotStateRevision) {
					return true
				}
				if pin.Source != nil {
					if source, ok := s.work[pin.Source.RequestID]; ok {
						stack = append(stack, dependency{work: source, access: pin.Source.AccessRevision, pinned: true})
					}
				}
			}
		}
	}
	return false
}

func (s *Store) adapterInvalidationSequence(w Work, authoritySequences []uint64) uint64 {
	for i := len(authoritySequences) - 1; i >= 0; i-- {
		sequence := authoritySequences[i]
		e := s.events[sequence-1]
		if e.Sequence > w.CreatedSequence && s.adapterInvalidationEvent(w, e) {
			return e.Sequence
		}
	}
	return 0
}

func (s *Store) adapterAuthoritySequences() []uint64 {
	sequences := []uint64{}
	for _, e := range s.events {
		if e.Type == "work.source_access" || e.Type == "file.access" && e.Upload != nil || e.Channel != nil || e.Project != nil || e.ProjectContinuity != nil || e.Plugin != nil || e.BotState != nil {
			sequences = append(sequences, e.Sequence)
		}
	}
	return sequences
}

func adapterReplyClearNonce(w Work, causal uint64) string {
	type filePin struct {
		ID, SHA256     string
		AccessRevision uint64
	}
	pin := struct {
		RequestID, ContextSHA256, ProjectID, ContinuitySHA256                string
		CreatedSequence, CausalSequence, ProjectRevision, ContinuityRevision uint64
		Adapter                                                              *AdapterPin
		Publication                                                          *Publication
		ToolPolicyVersion                                                    int
		Plugins                                                              []PluginPin
		Files                                                                []filePin
	}{RequestID: w.RequestID, CreatedSequence: w.CreatedSequence, CausalSequence: causal, Adapter: w.Adapter, Publication: w.Publication, ToolPolicyVersion: w.ToolPolicyVersion, Plugins: effectivePluginPins(w)}
	if w.Context != nil {
		pin.ContextSHA256 = w.Context.SHA256
	}
	if w.Resources != nil {
		pin.ProjectID, pin.ProjectRevision = w.Resources.Project.ID, w.Resources.Project.Revision
		for _, f := range w.Resources.Files {
			pin.Files = append(pin.Files, filePin{ID: f.ID, SHA256: f.SHA256, AccessRevision: f.AccessRevision})
		}
		if w.Resources.Continuity != nil {
			pin.ContinuityRevision, pin.ContinuitySHA256 = w.Resources.Continuity.Revision, w.Resources.Continuity.SHA256
		}
	}
	encoded, _ := json.Marshal(pin)
	h := sha256.Sum256(encoded)
	return "adapter-clear-" + hex.EncodeToString(h[:])
}

func adapterClearOutput(w Work) bool {
	return w.Adapter != nil && (w.Reply != "" || len(w.Artifacts) != 0)
}

func (s *Store) adapterReplyClearCheckpoints() map[string]uint64 {
	checkpoints := map[string]uint64{}
	for _, e := range s.events {
		if e.Type == "adapter.reply_clear_checkpoint" {
			checkpoints[e.Nonce] = e.Sequence
		}
	}
	return checkpoints
}

func (s *Store) validAdapterReplyClearEvent(e Event) bool {
	w, found := s.work[e.Work.RequestID]
	if !found || !adapterClearOutput(w) || s.contextAllowed(w) || e.Sequence != uint64(len(s.events)+1) {
		return false
	}
	causal := s.adapterInvalidationSequence(w, s.adapterAuthoritySequences())
	if causal == 0 {
		return false
	}
	nonce := adapterReplyClearNonce(w, causal)
	if s.adapterReplyClearCheckpoints()[nonce] != 0 {
		return false
	}
	return reflect.DeepEqual(e, Event{Type: "adapter.reply_clear_checkpoint", Sequence: e.Sequence, Work: Work{RequestID: w.RequestID}, Nonce: nonce})
}

func (s *Store) recoverAdapterReplyClears() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ErrUnavailable
	}
	if s.readOnly {
		return nil
	}
	authority := s.adapterAuthoritySequences()
	checkpoints := s.adapterReplyClearCheckpoints()
	count := 0
	for _, w := range s.sortedWorkLocked() {
		if !adapterClearOutput(w) || s.contextAllowed(w) {
			continue
		}
		causal := s.adapterInvalidationSequence(w, authority)
		if causal == 0 {
			continue
		}
		nonce := adapterReplyClearNonce(w, causal)
		if checkpoints[nonce] != 0 {
			continue
		}
		if count == 512 {
			break
		}
		if err := s.commit(Event{Type: "adapter.reply_clear_checkpoint", Work: Work{RequestID: w.RequestID}, Nonce: nonce}); err != nil {
			return err
		}
		checkpoints[nonce] = uint64(len(s.events))
		count++
	}
	return nil
}

func (s *Store) AdapterReplies(id string, after uint64) (AdapterPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return AdapterPage{}, ErrUnavailable
	}
	a, ok := s.adapters[id]
	if !ok {
		return AdapterPage{}, ErrNotFound
	}
	c, ok := s.channel(a.ChannelID)
	if !ok || c.Revision != a.ChannelRevision {
		return AdapterPage{}, ErrConflict
	}
	if after > uint64(len(s.events)) {
		return AdapterPage{}, ErrConflict
	}
	page := AdapterPage{AdapterSessionID: a.ID, SourceApp: a.SourceApp, SourceSessionID: a.SourceSessionID, SourceThreadID: a.SourceThreadID, ChannelID: a.ChannelID, OwnerUserID: a.OwnerUserID, SpaceID: a.SpaceID, ChannelRevision: a.ChannelRevision, ReturnRoute: a.ReturnRoute, Cursor: after, Replies: []AdapterReply{}}
	end := after + 200
	if end > uint64(len(s.events)) {
		end = uint64(len(s.events))
	}
	invalid := []Work{}
	for _, w := range s.sortedWorkLocked() {
		if !adapterClearOutput(w) || w.Adapter.AdapterSessionID != id || s.contextAllowed(w) {
			continue
		}
		invalid = append(invalid, w)
	}
	authoritySequences := []uint64{}
	checkpoints := map[string]uint64{}
	if len(invalid) > 0 {
		authoritySequences = s.adapterAuthoritySequences()
		checkpoints = s.adapterReplyClearCheckpoints()
	}
	clears := map[uint64][]AdapterReply{}
	for _, w := range invalid {
		sequence := s.adapterInvalidationSequence(w, authoritySequences)
		if sequence == 0 {
			return AdapterPage{}, ErrConflict
		}
		if checkpoint := checkpoints[adapterReplyClearNonce(w, sequence)]; checkpoint != 0 {
			sequence = checkpoint
		} else if sequence <= s.adapterOpenedSequence {
			return AdapterPage{}, ErrConflict
		}
		if sequence > after && sequence-after > 4096 {
			return AdapterPage{}, ErrConflict
		}
		if sequence <= after {
			continue
		}
		if sequence > end {
			end = sequence
		}
		clears[sequence] = append(clears[sequence], AdapterReply{ReplyID: "reply-" + w.RequestID, RequestID: w.RequestID, SourceMessageID: w.Adapter.SourceMessageID, BotID: w.BotID, Role: "assistant", Text: "", Status: w.Status, Sequence: sequence})
	}
	for _, e := range s.events[after:end] {
		candidate := page
		candidate.Cursor = e.Sequence
		candidate.Replies = append([]AdapterReply{}, page.Replies...)
		upsert := func(reply AdapterReply) {
			for i, old := range candidate.Replies {
				if old.ReplyID == reply.ReplyID {
					candidate.Replies[i] = reply
					return
				}
			}
			candidate.Replies = append(candidate.Replies, reply)
		}
		w := e.Work
		if w.Adapter != nil && w.Adapter.AdapterSessionID == id {
			reply := AdapterReply{ReplyID: "reply-" + w.RequestID, RequestID: w.RequestID, SourceMessageID: w.Adapter.SourceMessageID, BotID: w.BotID, Role: "assistant", Text: w.Reply, Status: w.Status, Sequence: e.Sequence, Artifacts: append([]Artifact(nil), w.Artifacts...)}
			if !s.contextAllowed(s.work[w.RequestID]) {
				reply.Text = ""
				reply.Artifacts = nil
			}
			upsert(reply)
		}
		for _, clear := range clears[e.Sequence] {
			upsert(clear)
		}
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return AdapterPage{}, ErrUnavailable
		}
		if len(encoded) > 900000 || len(candidate.Replies) > 512 {
			if len(clears) > 0 {
				return AdapterPage{}, ErrConflict
			}
			break
		}
		page = candidate
	}
	sort.Slice(page.Replies, func(i, j int) bool {
		if page.Replies[i].Sequence == page.Replies[j].Sequence {
			return page.Replies[i].ReplyID < page.Replies[j].ReplyID
		}
		return page.Replies[i].Sequence < page.Replies[j].Sequence
	})
	return page, nil
}
