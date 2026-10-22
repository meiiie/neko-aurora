package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ProjectDecision struct {
	ID            string   `json:"id"`
	Text          string   `json:"text"`
	Status        string   `json:"status"`
	EffectiveDate string   `json:"effectiveDate,omitempty"`
	EntityID      string   `json:"entityId,omitempty"`
	ReferenceIDs  []string `json:"referenceIds"`
}
type ProjectFileReference struct {
	FileID         string `json:"fileId"`
	SHA256         string `json:"sha256"`
	AccessRevision uint64 `json:"accessRevision"`
}
type ProjectReference struct {
	ID         string                `json:"id"`
	Title      string                `json:"title"`
	SourceDate string                `json:"sourceDate,omitempty"`
	EntityID   string                `json:"entityId,omitempty"`
	Version    string                `json:"version,omitempty"`
	File       *ProjectFileReference `json:"file,omitempty"`
	URL        string                `json:"url,omitempty"`
}
type ProjectContinuityInput struct {
	ProjectID               string             `json:"-"`
	ChannelID               string             `json:"channelId"`
	ExpectedProjectRevision uint64             `json:"expectedProjectRevision"`
	ExpectedChannelRevision uint64             `json:"expectedChannelRevision"`
	ExpectedRevision        uint64             `json:"expectedRevision"`
	ClientNonce             string             `json:"clientNonce"`
	Instructions            string             `json:"instructions"`
	Decisions               []ProjectDecision  `json:"decisions"`
	References              []ProjectReference `json:"references"`
}
type ProjectContinuity struct {
	Version         int                `json:"version"`
	ProjectID       string             `json:"projectId"`
	ChannelID       string             `json:"channelId"`
	ProjectRevision uint64             `json:"projectRevision"`
	ChannelRevision uint64             `json:"channelRevision"`
	Revision        uint64             `json:"revision"`
	OwnerUserID     string             `json:"ownerUserId,omitempty"`
	SpaceID         string             `json:"spaceId,omitempty"`
	AuthorType      string             `json:"authorType,omitempty"`
	RecordedAt      string             `json:"recordedAt,omitempty"`
	Instructions    string             `json:"instructions,omitempty"`
	Decisions       []ProjectDecision  `json:"decisions"`
	References      []ProjectReference `json:"references"`
	SHA256          string             `json:"sha256"`
}
type ContinuityPin struct {
	Revision uint64             `json:"revision"`
	SHA256   string             `json:"sha256"`
	Metadata *ProjectContinuity `json:"metadata,omitempty"`
}
type ProjectContinuityView struct {
	Continuity ProjectContinuity `json:"continuity"`
	Usable     bool              `json:"usable"`
}

func continuityKey(project, channel string) string { return project + "\x00" + channel }
func cloneContinuity(c ProjectContinuity) ProjectContinuity {
	c.Decisions = append([]ProjectDecision{}, c.Decisions...)
	for i := range c.Decisions {
		c.Decisions[i].ReferenceIDs = append([]string{}, c.Decisions[i].ReferenceIDs...)
	}
	c.References = append([]ProjectReference{}, c.References...)
	for i := range c.References {
		if c.References[i].File != nil {
			f := *c.References[i].File
			c.References[i].File = &f
		}
	}
	return c
}
func copyContinuityPin(p *ContinuityPin) *ContinuityPin {
	if p == nil {
		return nil
	}
	c := *p
	if p.Metadata != nil {
		m := cloneContinuity(*p.Metadata)
		c.Metadata = &m
	}
	return &c
}
func continuityDigest(c ProjectContinuity) string {
	c.SHA256 = ""
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func metadataID(id string) bool {
	return len(id) > 0 && len(id) <= 64 && strings.IndexFunc(id, func(c rune) bool {
		return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_')
	}) < 0
}
func metadataDate(value string) bool {
	if value == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
func metadataText(value string, limit int) bool {
	return botText(value, limit) && !hasSecret([]byte(value))
}
func metadataBody(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) && !hasSecret([]byte(value)) && strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}
func validContinuityData(c ProjectContinuity) bool {
	if !metadataBody(c.Instructions, 2000) || len(c.Decisions) > 8 || len(c.References) > 8 {
		return false
	}
	refs := map[string]bool{}
	for _, r := range c.References {
		if !metadataID(r.ID) || refs[r.ID] || strings.TrimSpace(r.Title) == "" || !metadataText(r.Title, 160) || !metadataText(r.EntityID, 80) || !metadataText(r.Version, 80) || !metadataDate(r.SourceDate) || (r.File == nil) == (r.URL == "") {
			return false
		}
		refs[r.ID] = true
		if r.File != nil && (!resourceID(r.File.FileID, "file-") || !lowerHex(r.File.SHA256, 64)) {
			return false
		}
		if r.URL != "" {
			u, err := url.Parse(r.URL)
			if err != nil || len(r.URL) > 512 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !metadataText(r.URL, 512) {
				return false
			}
		}
	}
	decisions := map[string]bool{}
	for _, d := range c.Decisions {
		if !metadataID(d.ID) || decisions[d.ID] || strings.TrimSpace(d.Text) == "" || !metadataBody(d.Text, 1000) || (d.Status != "confirmed" && d.Status != "contested") || !metadataDate(d.EffectiveDate) || !metadataText(d.EntityID, 80) || len(d.ReferenceIDs) > 8 {
			return false
		}
		decisions[d.ID] = true
		used := map[string]bool{}
		for _, id := range d.ReferenceIDs {
			if !refs[id] || used[id] {
				return false
			}
			used[id] = true
		}
	}
	b, err := json.Marshal(c)
	return err == nil && len(b) <= 6144 && !hasSecret(b)
}
func (s *Store) continuityScope(project, channel string) bool {
	p, ok := s.projects[project]
	c, found := s.channel(channel)
	return ok && found && p.OwnerUserID == "local-owner" && p.SpaceID == "local-space" && c.OwnerUserID == p.OwnerUserID && c.SpaceID == p.SpaceID && contains(p.ChannelIDs, channel)
}
func (s *Store) continuityReferencesAllowed(c ProjectContinuity) bool {
	if !s.continuityScope(c.ProjectID, c.ChannelID) {
		return false
	}
	for _, r := range c.References {
		if r.File != nil {
			f, ok := s.uploads[r.File.FileID]
			if !ok || !f.Granted || f.Status != "ready_text" || f.ProjectID != c.ProjectID || f.ChannelID != c.ChannelID || f.SHA256 != r.File.SHA256 || f.AccessRevision != r.File.AccessRevision {
				return false
			}
		}
	}
	return true
}
func (s *Store) validContinuityEvent(e Event) bool {
	if _, used := s.projectStateNonces[e.Nonce]; used {
		return false
	}
	if e.Type != "project.continuity_saved" || e.ProjectContinuity == nil || e.Plugin != nil || e.Collaboration != nil || e.TaskControl != nil || e.AdapterSession != nil || e.Project != nil || e.Upload != nil || e.Channel != nil || e.Bot != nil || e.ResourceRevision != 0 || e.CompletionVersion != 0 || e.ContextRefreshVersion != 0 || !reflect.DeepEqual(e.Work, Work{}) || !validAccessNonce(e.Nonce) || hasSecret([]byte(e.Nonce)) || len(s.productNonces) >= 2048 {
		return false
	}
	if _, ok := s.productNonces[e.Nonce]; ok {
		return false
	}
	c := *e.ProjectContinuity
	p, ok := s.projects[c.ProjectID]
	channel, found := s.channel(c.ChannelID)
	if !ok || !found || c.ProjectRevision != p.Revision || c.ChannelRevision != channel.Revision || !s.continuityReferencesAllowed(c) || c.Version != 1 || c.OwnerUserID != "local-owner" || c.SpaceID != "local-space" || c.AuthorType != "owner" || !validContinuityData(c) || c.SHA256 != continuityDigest(c) {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, c.RecordedAt); err != nil {
		return false
	}
	prior := s.projectContinuity[continuityKey(c.ProjectID, c.ChannelID)]
	return prior.Revision < ^uint64(0) && c.Revision == prior.Revision+1 && (prior.Revision != 0 || len(s.projectContinuity) < 512)
}
func inputContinuity(in ProjectContinuityInput) ProjectContinuity {
	return cloneContinuity(ProjectContinuity{Version: 1, ProjectID: in.ProjectID, ChannelID: in.ChannelID, ProjectRevision: in.ExpectedProjectRevision, ChannelRevision: in.ExpectedChannelRevision, Revision: in.ExpectedRevision + 1, Instructions: in.Instructions, Decisions: in.Decisions, References: in.References})
}
func (s *Store) SaveProjectContinuity(in ProjectContinuityInput) (ProjectContinuity, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ProjectContinuity{}, false, ErrUnavailable
	}
	if !validAccessNonce(in.ClientNonce) || hasSecret([]byte(in.ClientNonce)) || in.ExpectedRevision == ^uint64(0) {
		return ProjectContinuity{}, false, ErrInvalid
	}
	c := inputContinuity(in)
	if prior, ok := s.productNonces[in.ClientNonce]; ok {
		if prior.ProjectContinuity == nil {
			return ProjectContinuity{}, false, ErrConflict
		}
		original := cloneContinuity(*prior.ProjectContinuity)
		expected := original
		expected.OwnerUserID = ""
		expected.SpaceID = ""
		expected.AuthorType = ""
		expected.RecordedAt = ""
		expected.SHA256 = ""
		if !reflect.DeepEqual(c, expected) || !s.continuityScope(c.ProjectID, c.ChannelID) {
			return ProjectContinuity{}, false, ErrConflict
		}
		if err := s.suspendInvalidContext(); err != nil {
			return ProjectContinuity{}, false, err
		}
		return original, true, nil
	}
	if _, used := s.projectStateNonces[in.ClientNonce]; used {
		return ProjectContinuity{}, false, ErrConflict
	}
	p, ok := s.projects[in.ProjectID]
	channel, found := s.channel(in.ChannelID)
	if !ok || !found || !s.continuityScope(in.ProjectID, in.ChannelID) {
		return ProjectContinuity{}, false, ErrNotFound
	}
	old := s.projectContinuity[continuityKey(in.ProjectID, in.ChannelID)]
	if p.Revision != in.ExpectedProjectRevision || channel.Revision != in.ExpectedChannelRevision || old.Revision != in.ExpectedRevision {
		return ProjectContinuity{}, false, ErrConflict
	}
	c.OwnerUserID = p.OwnerUserID
	c.SpaceID = p.SpaceID
	c.AuthorType = "owner"
	c.RecordedAt = s.clock().Format(time.RFC3339Nano)
	c.SHA256 = continuityDigest(c)
	if len(s.productNonces) >= 2048 {
		return ProjectContinuity{}, false, ErrUnavailable
	}
	e := Event{Type: "project.continuity_saved", ProjectContinuity: &c, Nonce: in.ClientNonce}
	if !s.validContinuityEvent(e) {
		return ProjectContinuity{}, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return ProjectContinuity{}, false, err
	}
	if err := s.suspendInvalidContext(); err != nil {
		return ProjectContinuity{}, false, err
	}
	return cloneContinuity(c), false, nil
}
func (s *Store) GetProjectContinuity(project, channel string) (ProjectContinuityView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return ProjectContinuityView{}, ErrUnavailable
	}
	if !s.continuityScope(project, channel) {
		return ProjectContinuityView{}, ErrNotFound
	}
	c, ok := s.projectContinuity[continuityKey(project, channel)]
	if !ok {
		return ProjectContinuityView{Continuity: ProjectContinuity{Version: 1, ProjectID: project, ChannelID: channel}, Usable: true}, nil
	}
	return ProjectContinuityView{Continuity: cloneContinuity(c), Usable: s.continuityReferencesAllowed(c)}, nil
}
func (s *Store) pinContinuity(project, channel string) (*ContinuityPin, error) {
	c, ok := s.projectContinuity[continuityKey(project, channel)]
	if !ok {
		return nil, nil
	}
	if !s.continuityReferencesAllowed(c) {
		return nil, ErrConflict
	}
	c = cloneContinuity(c)
	return &ContinuityPin{Revision: c.Revision, SHA256: c.SHA256, Metadata: &c}, nil
}
func (s *Store) continuityAllowed(w Work) bool {
	if w.Resources == nil {
		return true
	}
	c, exists := s.projectContinuity[continuityKey(w.Resources.Project.ID, w.ConversationID)]
	pin := w.Resources.Continuity
	if pin == nil {
		return !exists
	}
	return exists && pin.Metadata != nil && pin.Revision == c.Revision && pin.SHA256 == c.SHA256 && reflect.DeepEqual(*pin.Metadata, c) && s.continuityReferencesAllowed(c)
}
func publicResources(p *ResourcePin) *ResourcePin {
	c := copyResources(p)
	if c != nil && c.Continuity != nil {
		c.Continuity.Metadata = nil
	}
	return c
}
func publicContinuity(c ProjectContinuity) ProjectContinuity {
	return ProjectContinuity{Version: c.Version, ProjectID: c.ProjectID, ChannelID: c.ChannelID, ProjectRevision: c.ProjectRevision, ChannelRevision: c.ChannelRevision, Revision: c.Revision, SHA256: c.SHA256}
}
func publicContinuityWork(w Work) Work {
	w = copyWork(w)
	w.RuntimeContext = nil
	w.Resources = publicResources(w.Resources)
	if w.Context != nil {
		w.Context.Resources = publicResources(w.Context.Resources)
	}
	return w
}

func PublicContinuityWork(w Work) Work { return publicContinuityWork(w) }
func PublicContinuityCollaboration(g Collaboration) Collaboration {
	g = copyCollaboration(g)
	g.PublishedReportContext = nil
	g.Resources = publicResources(g.Resources)
	return g
}

func (s *Store) PublicSnapshot() Snapshot {
	out := s.Snapshot()
	for i := range out.Work {
		out.Work[i] = publicContinuityWork(out.Work[i])
	}
	for i := range out.Collaborations {
		out.Collaborations[i] = PublicContinuityCollaboration(out.Collaborations[i])
	}
	return out
}
