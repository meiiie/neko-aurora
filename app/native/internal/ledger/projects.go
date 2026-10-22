package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"nekoaurora.local/native/internal/connections"
	"nekoaurora.local/native/internal/executors"
	"nekoaurora.local/native/internal/identity"
)

type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Goal        string   `json:"goal"`
	OwnerUserID string   `json:"ownerUserId"`
	SpaceID     string   `json:"spaceId"`
	Revision    uint64   `json:"revision"`
	ChannelIDs  []string `json:"channelIds"`
}
type ProjectInput struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Goal             string   `json:"goal"`
	ExpectedRevision uint64   `json:"expectedRevision"`
	ChannelIDs       []string `json:"channelIds"`
	ClientNonce      string   `json:"clientNonce"`
}
type UploadedFile struct {
	ExtractionVersion int    `json:"extractionVersion,omitempty"`
	TextFormat        string `json:"textFormat,omitempty"`
	TextSHA256        string `json:"textSHA256,omitempty"`
	TextBytes         int    `json:"textBytes,omitempty"`
	ID                string `json:"id"`
	ProjectID         string `json:"projectId"`
	ChannelID         string `json:"channelId"`
	OwnerUserID       string `json:"ownerUserId"`
	SpaceID           string `json:"spaceId"`
	Filename          string `json:"filename"`
	SHA256            string `json:"sha256"`
	Bytes             int    `json:"bytes"`
	MIME              string `json:"mime"`
	Version           int    `json:"version"`
	Status            string `json:"status"`
	Excerpt           string `json:"excerpt,omitempty"`
	Truncated         bool   `json:"truncated"`
	Granted           bool   `json:"granted"`
	AccessRevision    uint64 `json:"accessRevision"`
}
type UploadInput struct {
	ProjectID       string `json:"projectId"`
	ProjectRevision uint64 `json:"projectRevision"`
	ChannelID       string `json:"channelId"`
	ClientNonce     string `json:"clientNonce"`
	Filename        string `json:"filename"`
	Content         []byte `json:"contentBase64"`
}
type FileAccessInput struct {
	FileID           string `json:"fileId"`
	ExpectedRevision uint64 `json:"expectedRevision"`
	Granted          bool   `json:"granted"`
	ClientNonce      string `json:"clientNonce"`
}
type ResourcePin struct {
	Continuity       *ContinuityPin `json:"continuity,omitempty"`
	Project          Project        `json:"project"`
	RequestedFileIDs []string       `json:"requestedFileIds"`
	Files            []UploadedFile `json:"files"`
}
type SubmissionScope struct {
	coreProfileVerified        bool
	CoreProfileSelection       *CoreProfileSelection `json:"coreProfileSelection,omitempty"`
	requester                  *identity.VerifiedPrincipal
	UseBotDefaults             bool                  `json:"useBotDefaults,omitempty"`
	ExpectedBotStateRevision   *uint64               `json:"expectedBotStateRevision,omitempty"`
	ExpectedBotProfileRevision *uint64               `json:"expectedBotProfileRevision,omitempty"`
	ExpectedContinuityRevision *uint64               `json:"expectedContinuityRevision,omitempty"`
	PluginSelection            *[]PluginSelection    `json:"pluginSelection,omitempty"`
	ChannelID                  string                `json:"channelId,omitempty"`
	ExpectedChannelRevision    *uint64               `json:"expectedChannelRevision,omitempty"`
	AccountID                  string                `json:"accountId,omitempty"`
	AccountRevision            uint64                `json:"accountRevision,omitempty"`
	ProjectID                  string                `json:"projectId,omitempty"`
	ProjectRevision            uint64                `json:"projectRevision,omitempty"`
	FileIDs                    []string              `json:"fileIds,omitempty"`
	Adapter                    *AdapterPin           `json:"-"`
	ExpectedInstallationPin    *executors.InstallPin `json:"-"`
}

func cloneProject(p Project) Project { p.ChannelIDs = append([]string{}, p.ChannelIDs...); return p }
func copyResources(p *ResourcePin) *ResourcePin {
	if p == nil {
		return nil
	}
	c := *p
	c.Continuity = copyContinuityPin(p.Continuity)
	c.Project = cloneProject(c.Project)
	c.RequestedFileIDs = append([]string{}, p.RequestedFileIDs...)
	c.Files = append([]UploadedFile{}, p.Files...)
	return &c
}
func publicFile(f UploadedFile) UploadedFile { f.Excerpt = ""; return f }
func contains(values []string, id string) bool {
	for _, v := range values {
		if v == id {
			return true
		}
	}
	return false
}
func resourceID(id, prefix string) bool {
	return strings.HasPrefix(id, prefix) && lowerHex(strings.TrimPrefix(id, prefix), 32)
}
func (s *Store) validProject(p Project) bool {
	if !resourceID(p.ID, "project-") || p.OwnerUserID != "local-owner" || p.SpaceID != "local-space" || p.Revision == 0 || strings.TrimSpace(p.Name) == "" || !botText(p.Name, 80) || !botText(p.Goal, 2000) || len(p.ChannelIDs) == 0 || len(p.ChannelIDs) > 16 || !sort.StringsAreSorted(p.ChannelIDs) {
		return false
	}
	for i, id := range p.ChannelIDs {
		if i > 0 && p.ChannelIDs[i-1] == id {
			return false
		}
		if _, ok := s.channel(id); !ok {
			return false
		}
	}
	return true
}
func (s *Store) validProductEvent(e Event) bool {
	if e.Bot != nil || e.Channel != nil || !reflect.DeepEqual(e.Work, Work{}) || !validAccessNonce(e.Nonce) {
		return false
	}
	if e.CompletionVersion != 0 || e.ContextRefreshVersion != 0 || len(s.productNonces) >= 2048 || (e.Type != "file.uploaded" && e.ResourceRevision != 0) {
		return false
	}
	if _, ok := s.projectStateNonces[e.Nonce]; ok {
		return false
	}
	if _, ok := s.productNonces[e.Nonce]; ok {
		return false
	}
	switch e.Type {
	case "project.created", "project.updated":
		if e.Project == nil || e.Upload != nil || !s.validProject(*e.Project) {
			return false
		}
		old, exists := s.projects[e.Project.ID]
		if e.Type == "project.created" {
			return !exists && e.Project.Revision == 1 && len(s.projects) < 32
		}
		return exists && old.Revision < ^uint64(0) && e.Project.Revision == old.Revision+1
	case "file.uploaded":
		if e.Upload != nil && s.archivedProject(e.Upload.ProjectID) {
			return false
		}
		if e.Upload == nil || e.Project != nil {
			return false
		}
		f := *e.Upload
		p, ok := s.projects[f.ProjectID]
		if !ok || e.ResourceRevision != p.Revision || !contains(p.ChannelIDs, f.ChannelID) || !resourceID(f.ID, "file-") || f.OwnerUserID != "local-owner" || f.SpaceID != "local-space" || !validUploadName(f.Filename) || !lowerHex(f.SHA256, 64) || f.Bytes < 1 || f.Bytes > 1<<20 || f.Version != 1 || !f.Granted || f.AccessRevision != 0 || len(s.uploads) >= 512 {
			return false
		}
		total := f.Bytes
		for _, existing := range s.uploads {
			total += existing.Bytes
		}
		if total > 64<<20 {
			return false
		}
		if _, ok := s.uploads[f.ID]; ok {
			return false
		}
		return validUploadDescriptor(f)
	case "file.access":
		if e.Upload == nil || e.Project != nil {
			return false
		}
		old, ok := s.uploads[e.Upload.ID]
		if !ok || old.AccessRevision == ^uint64(0) || e.Upload.AccessRevision != old.AccessRevision+1 {
			return false
		}
		old.Granted = e.Upload.Granted
		old.AccessRevision = e.Upload.AccessRevision
		return reflect.DeepEqual(old, *e.Upload)
	}
	return false
}
func (s *Store) SaveProject(in ProjectInput) (Project, bool, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Goal = strings.TrimSpace(in.Goal)
	in.ChannelIDs = append([]string{}, in.ChannelIDs...)
	sort.Strings(in.ChannelIDs)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validAccessNonce(in.ClientNonce) {
		return Project{}, false, ErrInvalid
	}
	if e, ok := s.productNonces[in.ClientNonce]; ok {
		if e.Project == nil {
			return Project{}, false, ErrConflict
		}
		p := *e.Project
		if p.Name != in.Name || p.Goal != in.Goal || !reflect.DeepEqual(p.ChannelIDs, in.ChannelIDs) || (in.ID != "" && p.ID != in.ID) || p.Revision != in.ExpectedRevision+1 {
			return Project{}, false, ErrConflict
		}
		if s.projects[p.ID].Revision != p.Revision {
			return Project{}, false, ErrConflict
		}
		return cloneProject(p), true, nil
	}
	if _, used := s.projectStateNonces[in.ClientNonce]; used {
		return Project{}, false, ErrConflict
	}
	if len(s.productNonces) >= 2048 {
		return Project{}, false, ErrUnavailable
	}
	id, typ, rev := in.ID, "project.created", uint64(1)
	if id == "" {
		if in.ExpectedRevision != 0 {
			return Project{}, false, ErrConflict
		}
		var err error
		id, err = newID("project-")
		if err != nil {
			return Project{}, false, err
		}
	} else {
		old, ok := s.projects[id]
		if !ok || old.Revision != in.ExpectedRevision || old.Revision == ^uint64(0) {
			return Project{}, false, ErrConflict
		}
		typ = "project.updated"
		rev = old.Revision + 1
	}
	p := Project{ID: id, Name: in.Name, Goal: in.Goal, OwnerUserID: "local-owner", SpaceID: "local-space", Revision: rev, ChannelIDs: in.ChannelIDs}
	e := Event{Type: typ, Project: &p, Nonce: in.ClientNonce}
	if !s.validProductEvent(e) {
		return Project{}, false, ErrInvalid
	}
	if err := s.commit(e); err != nil {
		return Project{}, false, err
	}
	if err := s.suspendInvalidContext(); err != nil {
		return Project{}, false, err
	}
	return cloneProject(p), false, nil
}

var secretPattern = regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|\b(?:sk-(?:proj-)?[A-Za-z0-9_-]{24,}|gh[pousr]_[A-Za-z0-9]{30,}|AKIA[A-Z0-9]{16}|xox[baprs]-[A-Za-z0-9-]{20,})\b|(?i:(?:api[_-]?key|access[_-]?token|password|client[_-]?secret)\s*[:=]\s*["'][^"'\r\n]{12,}["'])`)

func hasSecret(data []byte) bool { return secretPattern.Match(data) }
func validUploadName(name string) bool {
	lower := strings.ToLower(name)
	return name != "" && len(name) <= 120 && botText(name, 120) && filepath.Base(name) == name && name != "." && name != ".." && !strings.ContainsAny(name, "/\\:\x00") && lower != ".env" && !strings.HasPrefix(lower, ".env.") && !strings.HasSuffix(lower, ".pem") && !strings.HasSuffix(lower, ".key") && !strings.HasSuffix(lower, ".p12") && !strings.HasSuffix(lower, ".pfx")
}
func readyUploadText(name string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return (ext == ".txt" || ext == ".md" || ext == ".csv" || ext == ".json") && utf8.Valid(data) && strings.IndexFunc(string(data), func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0
}
func validUploadDescriptor(f UploadedFile) bool {
	if f.ExtractionVersion != 0 || f.TextFormat != "" || f.TextSHA256 != "" || f.TextBytes != 0 {
		if f.Status != "ready_text" || !extractedDescriptor(f) || len(f.Excerpt) == 0 || len(f.Excerpt) > 16000 {
			return false
		}
		if f.Truncated {
			return f.TextBytes > 16000 && len(f.Excerpt) >= 15997 && len(f.Excerpt) < f.TextBytes
		}
		h := sha256.Sum256([]byte(f.Excerpt))
		return len(f.Excerpt) == f.TextBytes && hex.EncodeToString(h[:]) == f.TextSHA256
	}
	if f.Status == "quarantined_unsupported" {
		return f.Excerpt == "" && f.MIME == "application/octet-stream" && !f.Truncated
	}
	if f.Status != "ready_text" || f.MIME != "text/plain; charset=utf-8" || len(f.Excerpt) == 0 || len(f.Excerpt) > 16000 || !readyUploadText(f.Filename, []byte(f.Excerpt)) || hasSecret([]byte(f.Excerpt)) {
		return false
	}
	if f.Truncated {
		return f.Bytes > 16000 && len(f.Excerpt) >= 15997 && len(f.Excerpt) < f.Bytes
	}
	if f.Bytes != len(f.Excerpt) {
		return false
	}
	h := sha256.Sum256([]byte(f.Excerpt))
	return hex.EncodeToString(h[:]) == f.SHA256
}
func (s *Store) UploadFile(in UploadInput) (UploadedFile, bool, error) {
	if s.readOnly {
		return UploadedFile{}, false, ErrReadOnly
	}
	if !validAccessNonce(in.ClientNonce) || !validUploadName(in.Filename) || len(in.Content) == 0 || len(in.Content) > 1<<20 || hasSecret(in.Content) {
		return UploadedFile{}, false, ErrInvalid
	}
	in.Content = bytes.Clone(in.Content)
	h := sha256.Sum256(in.Content)
	digest := hex.EncodeToString(h[:])
	text := in.Content
	ready := readyUploadText(in.Filename, in.Content)
	extracted, textFormat, textMIME, extractionErr := s.extractFileText(in.Filename, in.Content)
	if extractionErr == nil && !safeExtractedText(extracted.Text) {
		return UploadedFile{}, false, ErrInvalid
	}
	if extractionErr == nil && extracted.TextBytes > 0 {
		text = []byte(extracted.Text)
		ready = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return UploadedFile{}, false, ErrUnavailable
	}
	if prior, ok := s.productNonces[in.ClientNonce]; ok {
		if prior.Type != "file.uploaded" || prior.Upload == nil {
			return UploadedFile{}, false, ErrConflict
		}
		f := *prior.Upload
		if f.ProjectID != in.ProjectID || f.ChannelID != in.ChannelID || f.Filename != in.Filename || f.SHA256 != digest || prior.ResourceRevision != in.ProjectRevision {
			return UploadedFile{}, false, ErrConflict
		}
		return publicFile(s.uploads[f.ID]), true, nil
	}
	if _, used := s.projectStateNonces[in.ClientNonce]; used {
		return UploadedFile{}, false, ErrConflict
	}
	if s.archivedProject(in.ProjectID) {
		return UploadedFile{}, false, ErrConflict
	}
	p, ok := s.projects[in.ProjectID]
	if !ok {
		return UploadedFile{}, false, ErrNotFound
	}
	if p.Revision != in.ProjectRevision || !contains(p.ChannelIDs, in.ChannelID) {
		return UploadedFile{}, false, ErrConflict
	}
	if len(s.productNonces) >= 2048 {
		return UploadedFile{}, false, ErrUnavailable
	}
	total := 0
	for _, f := range s.uploads {
		total += f.Bytes
	}
	if total+len(in.Content) > 64<<20 {
		return UploadedFile{}, false, ErrUnavailable
	}
	identity := sha256.Sum256([]byte(in.ProjectID + "\x00" + in.ChannelID + "\x00" + in.ClientNonce))
	id := "file-" + hex.EncodeToString(identity[:16])
	f := UploadedFile{ID: id, ProjectID: p.ID, ChannelID: in.ChannelID, OwnerUserID: "local-owner", SpaceID: "local-space", Filename: in.Filename, SHA256: digest, Bytes: len(in.Content), MIME: "application/octet-stream", Version: 1, Status: "quarantined_unsupported", Granted: true}
	if extractionErr == nil && extracted.TextBytes > 0 {
		f.ExtractionVersion = 1
		f.TextFormat = textFormat
		f.TextSHA256 = extracted.TextSHA256
		f.TextBytes = extracted.TextBytes
	}
	if ready {
		f.Status = "ready_text"
		f.MIME = "text/plain; charset=utf-8"
		if f.ExtractionVersion == 1 {
			f.MIME = textMIME
		}
		b := text
		if len(b) > 16000 {
			b = b[:16000]
			for !utf8.Valid(b) {
				b = b[:len(b)-1]
			}
			f.Truncated = true
		}
		f.Excerpt = string(b)
	}
	e := Event{Type: "file.uploaded", Upload: &f, Nonce: in.ClientNonce, ResourceRevision: p.Revision}
	if !s.validProductEvent(e) {
		return UploadedFile{}, false, ErrInvalid
	}
	dir := filepath.Join(s.dir, "uploads")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return UploadedFile{}, false, ErrUnavailable
	}
	path := filepath.Join(dir, id)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		existing, readErr := s.readUploadedBytes(f)
		if readErr != nil || !bytes.Equal(existing, in.Content) {
			return UploadedFile{}, false, ErrConflict
		}
	} else {
		if err != nil {
			return UploadedFile{}, false, ErrUnavailable
		}
		n, writeErr := file.Write(in.Content)
		if writeErr == nil && n != len(in.Content) {
			writeErr = io.ErrShortWrite
		}
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return UploadedFile{}, false, ErrUnavailable
		}
	}
	if err = s.commit(e); err != nil {
		return UploadedFile{}, false, err
	}
	return publicFile(f), false, nil
}
func (s *Store) SetFileAccess(in FileAccessInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validAccessNonce(in.ClientNonce) {
		return ErrInvalid
	}
	if e, ok := s.productNonces[in.ClientNonce]; ok {
		if e.Type != "file.access" || e.Upload == nil || e.Upload.ID != in.FileID || e.Upload.Granted != in.Granted || e.Upload.AccessRevision != in.ExpectedRevision+1 {
			return ErrConflict
		}
		return s.suspendInvalidContext()
	}
	if _, used := s.projectStateNonces[in.ClientNonce]; used {
		return ErrConflict
	}
	f, ok := s.uploads[in.FileID]
	if !ok {
		return ErrNotFound
	}
	if f.AccessRevision != in.ExpectedRevision || f.AccessRevision == ^uint64(0) {
		return ErrConflict
	}
	if len(s.productNonces) >= 2048 {
		return ErrUnavailable
	}
	f.AccessRevision++
	f.Granted = in.Granted
	if err := s.commit(Event{Type: "file.access", Upload: &f, Nonce: in.ClientNonce}); err != nil {
		return err
	}
	return s.suspendInvalidContext()
}
func (s *Store) ReadProjectFile(project, channel, id string) (UploadedFile, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.uploads[id]
	if !ok || f.ProjectID != project || f.ChannelID != channel {
		return UploadedFile{}, nil, ErrNotFound
	}
	p, ok := s.projects[project]
	if !ok || !contains(p.ChannelIDs, channel) || !f.Granted {
		return UploadedFile{}, nil, ErrConflict
	}
	b, err := s.readUploadedBytes(f)
	if err != nil {
		return UploadedFile{}, nil, ErrUnavailable
	}
	h := sha256.Sum256(b)
	if len(b) != f.Bytes || hex.EncodeToString(h[:]) != f.SHA256 {
		return UploadedFile{}, nil, ErrUnavailable
	}
	return publicFile(f), b, nil
}
func (s *Store) readUploadedBytes(f UploadedFile) ([]byte, error) {
	path := filepath.Join(s.dir, "uploads", f.ID)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(f.Bytes) || f.Bytes < 1 || f.Bytes > 1<<20 {
		return nil, ErrUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, ErrUnavailable
	}
	h := sha256.Sum256(b)
	if len(b) != f.Bytes || hex.EncodeToString(h[:]) != f.SHA256 {
		return nil, ErrUnavailable
	}
	return b, nil
}
func sortedIDs(ids []string) []string {
	out := append([]string{}, ids...)
	sort.Strings(out)
	return out
}
func resourceSelectionEqual(p *ResourcePin, scope SubmissionScope) bool {
	if scope.ExpectedContinuityRevision != nil {
		revision := uint64(0)
		if p != nil && p.Continuity != nil {
			revision = p.Continuity.Revision
		}
		if revision != *scope.ExpectedContinuityRevision {
			return false
		}
	}
	if p == nil {
		return scope.ProjectID == "" && scope.ProjectRevision == 0 && len(scope.FileIDs) == 0
	}
	return p.Project.ID == scope.ProjectID && p.Project.Revision == scope.ProjectRevision && reflect.DeepEqual(p.RequestedFileIDs, sortedIDs(scope.FileIDs))
}
func (s *Store) pinResources(w Work, scope SubmissionScope) (*ResourcePin, error) {
	if scope.ProjectID == "" {
		if scope.ExpectedContinuityRevision != nil && *scope.ExpectedContinuityRevision != 0 {
			return nil, ErrConflict
		}
		if scope.ProjectRevision != 0 || len(scope.FileIDs) != 0 {
			return nil, ErrInvalid
		}
		return nil, nil
	}
	p, ok := s.projects[scope.ProjectID]
	if !ok {
		return nil, ErrNotFound
	}
	if p.Revision != scope.ProjectRevision || !contains(p.ChannelIDs, w.ConversationID) {
		return nil, ErrConflict
	}
	if scope.ExpectedContinuityRevision != nil && s.projectContinuity[continuityKey(p.ID, w.ConversationID)].Revision != *scope.ExpectedContinuityRevision {
		return nil, ErrConflict
	}
	if len(scope.FileIDs) > 8 {
		return nil, ErrInvalid
	}
	ids := sortedIDs(scope.FileIDs)
	for i, id := range ids {
		if i > 0 && ids[i-1] == id {
			return nil, ErrInvalid
		}
		f, ok := s.uploads[id]
		if !ok || f.ProjectID != p.ID || f.ChannelID != w.ConversationID || !f.Granted {
			return nil, ErrConflict
		}
		if f.Status != "ready_text" {
			return nil, ErrInvalid
		}
	}
	query := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(w.Text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len(word) >= 3 {
			query[word] = true
		}
	}
	type candidate struct {
		file  UploadedFile
		score int
	}
	var candidates []candidate
	for _, f := range s.uploads {
		if f.ProjectID != p.ID || f.ChannelID != w.ConversationID || !f.Granted || f.Status != "ready_text" {
			continue
		}
		score := 0
		body := strings.ToLower(f.Filename + " " + f.Excerpt)
		for word := range query {
			if strings.Contains(body, word) {
				score++
			}
		}
		if contains(ids, f.ID) {
			score += 10000
		}
		if score > 0 {
			candidates = append(candidates, candidate{f, score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].file.ID < candidates[j].file.ID
	})
	pin := &ResourcePin{Project: cloneProject(p), RequestedFileIDs: ids, Files: []UploadedFile{}}
	var err error
	pin.Continuity, err = s.pinContinuity(p.ID, w.ConversationID)
	if err != nil {
		return nil, err
	}
	budget := 16000
	for _, candidate := range candidates {
		f := candidate.file
		if len(pin.Files) >= 8 {
			break
		}
		max := budget
		if max > 4000 {
			max = 4000
		}
		if max <= 0 {
			if contains(ids, f.ID) {
				return nil, ErrInvalid
			}
			break
		}
		if len(f.Excerpt) > max {
			b := []byte(f.Excerpt)[:max]
			for !utf8.Valid(b) {
				b = b[:len(b)-1]
			}
			f.Excerpt = string(b)
			f.Truncated = true
		}
		budget -= len(f.Excerpt)
		pin.Files = append(pin.Files, f)
	}
	for _, id := range ids {
		found := false
		for _, f := range pin.Files {
			found = found || f.ID == id
		}
		if !found {
			return nil, ErrInvalid
		}
	}
	return pin, nil
}
func (s *Store) resourceAllowed(w Work) bool {
	if !s.continuityAllowed(w) {
		return false
	}
	if w.Resources == nil {
		return true
	}
	p, ok := s.projects[w.Resources.Project.ID]
	if !ok || p.Revision != w.Resources.Project.Revision || !contains(p.ChannelIDs, w.ConversationID) {
		return false
	}
	for _, source := range w.Resources.Files {
		f, ok := s.uploads[source.ID]
		if !ok || !f.Granted || f.AccessRevision != source.AccessRevision || f.SHA256 != source.SHA256 || f.ProjectID != p.ID || f.ChannelID != w.ConversationID || f.Status != "ready_text" {
			return false
		}
	}
	return true
}
func (s *Store) AcceptProduct(in Input, scope SubmissionScope) (Work, bool, error) {
	if scope.CoreProfileSelection != nil && (scope.AccountID != "" || scope.AccountRevision != 0 || scope.UseBotDefaults || scope.requester != nil || scope.Adapter != nil) {
		return Work{}, false, ErrInvalid
	}
	if scope.UseBotDefaults && (scope.AccountID != "" || scope.AccountRevision != 0 || scope.ExpectedBotStateRevision == nil) || scope.ExpectedBotStateRevision != nil && *scope.ExpectedBotStateRevision >= 1<<53 {
		return Work{}, false, ErrInvalid
	}
	if scope.ExpectedBotProfileRevision != nil && *scope.ExpectedBotProfileRevision > 1<<53-1 {
		return Work{}, false, ErrInvalid
	}
	if in.Lane == "" {
		in.Lane = "foreground"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, principalErr := s.intakePrincipalGuard(scope.requester, scope.ChannelID, scope.ExpectedChannelRevision)
	if principalErr != nil {
		return Work{}, false, principalErr
	}
	defer release()
	if scope.ExpectedInstallationPin != nil && !executors.ValidInstallPin(*scope.ExpectedInstallationPin) {
		return Work{}, false, ErrInvalid
	}
	if s.closed || s.poisoned {
		return Work{}, false, ErrUnavailable
	}
	if id, ok := s.findInputNonce(in, scope.ChannelID, scope.requester); ok {
		old := s.work[id]
		if !reflect.DeepEqual(old.CoreProfileSelection, scope.CoreProfileSelection) {
			return Work{}, false, ErrConflict
		}
		if old.UseBotDefaults != scope.UseBotDefaults || !reflect.DeepEqual(old.ExpectedBotStateRevision, scope.ExpectedBotStateRevision) {
			return Work{}, false, ErrConflict
		}
		if scope.ExpectedInstallationPin != nil && (old.ExecutorPin == nil || *old.ExecutorPin != *scope.ExpectedInstallationPin) {
			return Work{}, false, ErrConflict
		}
		if !reflect.DeepEqual(old.PluginSelection, scope.PluginSelection) || !reflect.DeepEqual(old.Adapter, scope.Adapter) {
			return Work{}, false, ErrConflict
		}
		if originalBot(old) != in.BotID || old.Text != in.Text || old.Lane != in.Lane || !reflect.DeepEqual(old.ExpectedBotProfileRevision, scope.ExpectedBotProfileRevision) || (scope.ChannelID != "" && old.ConversationID != scope.ChannelID) || !resourceSelectionEqual(old.Resources, scope) || (scope.ExpectedChannelRevision != nil && (old.Publication == nil || old.Publication.AudienceRevision != *scope.ExpectedChannelRevision)) {
			return Work{}, false, ErrConflict
		}
		if scope.UseBotDefaults {
			if old.BotState == nil {
				return Work{}, false, ErrConflict
			}
		} else if scope.AccountID == "" {
			if scope.AccountRevision != 0 || old.Account != nil {
				return Work{}, false, ErrConflict
			}
		} else if old.Account == nil || old.Account.AccountID != scope.AccountID || old.Account.Revision != scope.AccountRevision {
			return Work{}, false, ErrConflict
		}
		return copyWork(old), true, nil
	}
	if s.archivedProject(scope.ProjectID) {
		return Work{}, false, ErrConflict
	}
	if err := s.validInput(in); err != nil {
		return Work{}, false, err
	}
	if scope.CoreProfileSelection != nil {
		if err := s.verifyProfileSelection(scope); err != nil {
			return Work{}, false, err
		}
		scope.coreProfileVerified = true
	}
	target, err := s.targetBot(in)
	if err != nil {
		return Work{}, false, err
	}
	if s.archivedBot(in.BotID) || s.archivedBot(target) {
		return Work{}, false, ErrConflict
	}
	state, ok := s.botState(target)
	if !ok {
		return Work{}, false, ErrNotFound
	}
	if scope.ExpectedBotStateRevision != nil && state.Revision != *scope.ExpectedBotStateRevision {
		return Work{}, false, ErrConflict
	}
	if scope.UseBotDefaults {
		scope.AccountID, scope.AccountRevision = state.DefaultAccountID, state.DefaultAccountRevision
	}
	if scope.AccountID == "" {
		if scope.AccountRevision != 0 {
			return Work{}, false, ErrInvalid
		}
		return s.acceptLocked(in, nil, scope.ChannelID, scope)
	}
	if s.accounts == nil {
		return Work{}, false, ErrConflict
	}
	var work Work
	var duplicate bool
	err = s.accounts.WithSelection(scope.AccountID, scope.AccountRevision, func(pin connections.Pin) error {
		if scope.UseBotDefaults && pin.Model != state.DefaultModel {
			return ErrConflict
		}
		var err error
		work, duplicate, err = s.acceptLocked(in, &pin, scope.ChannelID, scope)
		return err
	})
	if errors.Is(err, connections.ErrConflict) {
		err = ErrConflict
	}
	return work, duplicate, err
}
