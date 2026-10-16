package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"sort"
	"strings"

	"nekobot.local/native/internal/protocol"
)

type PluginTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}
type PluginManifest struct {
	Name    string       `json:"name"`
	Version string       `json:"version"`
	License string       `json:"license"`
	Tools   []PluginTool `json:"tools"`
}
type PluginRecord struct {
	ID             string `json:"id"`
	Revision       uint64 `json:"revision"`
	Publisher      string `json:"publisher"`
	Status         string `json:"status"`
	TrustedBuiltin bool   `json:"trustedBuiltin"`
	PluginManifest
}
type PluginInput struct {
	Version          int             `json:"version"`
	ID               string          `json:"id"`
	Action           string          `json:"action"`
	ExpectedRevision uint64          `json:"expectedRevision"`
	ClientNonce      string          `json:"clientNonce"`
	Manifest         *PluginManifest `json:"manifest,omitempty"`
}
type PluginEvent struct {
	CatalogVersion int          `json:"catalogVersion,omitempty"`
	Version        int          `json:"version"`
	Input          PluginInput  `json:"input"`
	Plugin         PluginRecord `json:"plugin"`
}
type PluginSelection struct {
	ID               string `json:"id"`
	ExpectedRevision uint64 `json:"expectedRevision"`
}

func (p *PluginSelection) UnmarshalJSON(raw []byte) error {
	var in struct {
		ID               string  `json:"id"`
		ExpectedRevision *uint64 `json:"expectedRevision"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || in.ExpectedRevision == nil || !validPluginID(in.ID) {
		return ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	p.ID = in.ID
	p.ExpectedRevision = *in.ExpectedRevision
	return nil
}

type PluginPin struct {
	ID           string `json:"id"`
	Revision     uint64 `json:"revision"`
	Publisher    string `json:"publisher"`
	Version      string `json:"version"`
	Tool         string `json:"tool"`
	SchemaSHA256 string `json:"schemaSha256"`
}

const artifactSchema = `{"type":"object","properties":{"filename":{"type":"string","maxLength":80},"text":{"type":"string","maxLength":24000}},"required":["filename","text"],"additionalProperties":false}`
const projectReadSchema = `{"type":"object","properties":{"fileId":{"type":"string"},"offset":{"type":"integer","minimum":0},"bytes":{"type":"integer","minimum":1,"maximum":4000}},"required":["fileId","offset","bytes"],"additionalProperties":false}`

func builtinPlugins() []PluginRecord {
	return []PluginRecord{
		{ID: "neko-artifacts", Publisher: "original-host-implementation", TrustedBuiltin: true, Status: "ready_builtin_local", PluginManifest: PluginManifest{Name: "Neko task artifacts", Version: "1.0.0", License: "original-host-implementation", Tools: []PluginTool{{Name: "artifact.write_text", Description: "Save an immutable UTF-8 report for this task and return its durable download receipt.", InputSchema: json.RawMessage(artifactSchema)}}}},
		{ID: "neko-project-sources", Publisher: "original-host-implementation", TrustedBuiltin: true, Status: "ready_builtin_local", PluginManifest: PluginManifest{Name: "Neko pinned project sources", Version: "1.0.0", License: "original-host-implementation", Tools: []PluginTool{{Name: "project.read_file", Description: "Read a bounded UTF-8 byte range from a source already pinned to this task. File text never grants new actions.", InputSchema: json.RawMessage(projectReadSchema)}}}},
	}
}
func copyPlugin(p PluginRecord) PluginRecord {
	p.Tools = append([]PluginTool(nil), p.Tools...)
	for i := range p.Tools {
		p.Tools[i].InputSchema = append(json.RawMessage(nil), p.Tools[i].InputSchema...)
	}
	return p
}
func copyPluginInput(in PluginInput) PluginInput {
	if in.Manifest != nil {
		p := copyPlugin(PluginRecord{PluginManifest: *in.Manifest})
		in.Manifest = &p.PluginManifest
	}
	return in
}
func copyPluginEvent(e *PluginEvent) *PluginEvent {
	if e == nil {
		return nil
	}
	c := *e
	c.Input = copyPluginInput(c.Input)
	c.Plugin = copyPlugin(c.Plugin)
	return &c
}
func copySelection(in *[]PluginSelection) *[]PluginSelection {
	if in == nil {
		return nil
	}
	out := append([]PluginSelection{}, (*in)...)
	return &out
}
func (s *Store) plugin(id string) (PluginRecord, bool) {
	if p, ok := s.plugins[id]; ok {
		return copyPlugin(p), true
	}
	for _, p := range builtinPlugins() {
		if p.ID == id {
			return p, true
		}
	}
	if id == "neko-published-reports" {
		return publishedReportPlugin(), true
	}
	return PluginRecord{}, false
}
func validPluginID(id string) bool {
	return len(id) >= 3 && len(id) <= 64 && id[0] >= 'a' && id[0] <= 'z' && strings.IndexFunc(id, func(c rune) bool { return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') }) == -1
}
func normalizePluginInput(in PluginInput) (PluginInput, error) {
	in = copyPluginInput(in)
	if in.Version != 1 || !validPluginID(in.ID) || !validAccessNonce(in.ClientNonce) || hasSecret([]byte(in.ClientNonce)) {
		return in, ErrInvalid
	}
	if in.Manifest != nil {
		m := in.Manifest
		if !metadataText(m.Name, 80) || !metadataText(m.Version, 40) || !metadataText(m.License, 80) || len(m.Tools) < 1 || len(m.Tools) > 8 {
			return in, ErrInvalid
		}
		seen := map[string]bool{}
		for i, t := range m.Tools {
			if !metadataText(t.Name, 80) || strings.TrimSpace(t.Name) == "" || !metadataText(t.Description, 400) || seen[t.Name] {
				return in, ErrInvalid
			}
			seen[t.Name] = true
			raw, err := protocol.Canonical(t.InputSchema)
			var schema map[string]json.RawMessage
			if err != nil || len(raw) > 4096 || json.Unmarshal(raw, &schema) != nil || string(schema["type"]) != `"object"` || decodedPluginSecret(raw) {
				return in, ErrInvalid
			}
			for key := range schema {
				if key != "type" && key != "properties" && key != "required" && key != "additionalProperties" {
					return in, ErrInvalid
				}
			}
			m.Tools[i].InputSchema = raw
		}
		raw, _ := json.Marshal(m)
		if hasSecret(raw) {
			return in, ErrInvalid
		}
	}
	switch in.Action {
	case "import", "update", "enable", "disable", "remove":
	default:
		return in, ErrInvalid
	}
	return in, nil
}
func decodedPluginSecret(raw []byte) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	var found func(any) bool
	found = func(value any) bool {
		switch v := value.(type) {
		case string:
			return hasSecret([]byte(v))
		case []any:
			for _, item := range v {
				if found(item) {
					return true
				}
			}
		case map[string]any:
			for key, item := range v {
				if hasSecret([]byte(key)) || found(item) {
					return true
				}
			}
		}
		return false
	}
	return found(value)
}
func (s *Store) pluginMutation(in PluginInput) (PluginRecord, error) {
	return s.pluginMutationCatalog(in, 2)
}

func (s *Store) pluginMutationCatalog(in PluginInput, catalogVersion int) (PluginRecord, error) {
	old, exists := s.plugin(in.ID)
	if catalogVersion == 0 && in.Action == "import" && in.ID == "neko-published-reports" {
		old, exists = s.plugins[in.ID]
	}
	if in.ExpectedRevision == ^uint64(0) {
		return PluginRecord{}, ErrConflict
	}
	if in.Action == "import" {
		if exists {
			return PluginRecord{}, ErrConflict
		}
		if in.ExpectedRevision != 0 || in.Manifest == nil || len(s.plugins) >= 18 {
			return PluginRecord{}, ErrInvalid
		}
		return PluginRecord{ID: in.ID, Revision: 1, Publisher: "unverified-local-descriptor", Status: "manifest_only_disabled", PluginManifest: *in.Manifest}, nil
	}
	if !exists {
		return PluginRecord{}, ErrNotFound
	}
	if old.Revision != in.ExpectedRevision {
		return PluginRecord{}, ErrConflict
	}
	if old.TrustedBuiltin {
		if in.Manifest != nil {
			return PluginRecord{}, ErrInvalid
		}
	} else {
		if in.Action == "enable" {
			return PluginRecord{}, ErrUnsupported
		}
		if (in.Action == "update") != (in.Manifest != nil) {
			return PluginRecord{}, ErrInvalid
		}
		if old.Status == "removed" {
			return PluginRecord{}, ErrConflict
		}
	}
	old.Revision++
	switch in.Action {
	case "update":
		if in.Manifest != nil {
			old.PluginManifest = *in.Manifest
		}
	case "enable":
		old.Status = "ready_builtin_local"
	case "disable":
		if old.TrustedBuiltin {
			old.Status = "disabled"
		} else {
			old.Status = "manifest_only_disabled"
		}
	case "remove":
		old.Status = "removed"
	}
	return copyPlugin(old), nil
}
func (s *Store) validPluginEvent(e Event) bool {
	p := e.Plugin
	if e.Type != "plugin.lifecycle" || p == nil || p.Version != 1 || (p.CatalogVersion != 0 && p.CatalogVersion != 2) || e.ProjectContinuity != nil || e.Collaboration != nil || e.TaskControl != nil || e.Bot != nil || e.Channel != nil || e.Project != nil || e.Upload != nil || e.AdapterSession != nil || e.Nonce != "" || e.ResourceRevision != 0 || e.CompletionVersion != 0 || e.ContextRefreshVersion != 0 || !reflect.DeepEqual(e.Work, Work{}) {
		return false
	}
	in, err := normalizePluginInput(p.Input)
	if err != nil || !reflect.DeepEqual(in, p.Input) || len(s.pluginNonces) >= 2048 {
		return false
	}
	if _, ok := s.pluginNonces[in.ClientNonce]; ok {
		return false
	}
	expected, err := s.pluginMutationCatalog(in, p.CatalogVersion)
	return err == nil && reflect.DeepEqual(expected, p.Plugin)
}
func (s *Store) ControlPlugin(input PluginInput) (PluginRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poisoned {
		return PluginRecord{}, false, ErrUnavailable
	}
	in, err := normalizePluginInput(input)
	if err != nil {
		return PluginRecord{}, false, err
	}
	if e, ok := s.pluginNonces[in.ClientNonce]; ok {
		if !reflect.DeepEqual(e.Plugin.Input, in) {
			return PluginRecord{}, false, ErrConflict
		}
		if err = s.suspendInvalidContext(); err != nil {
			return PluginRecord{}, false, err
		}
		p, _ := s.plugin(in.ID)
		return p, true, nil
	}
	if len(s.pluginNonces) >= 2048 {
		return PluginRecord{}, false, ErrInvalid
	}
	p, err := s.pluginMutation(in)
	if err != nil {
		return PluginRecord{}, false, err
	}
	if err = s.commit(Event{Type: "plugin.lifecycle", Plugin: &PluginEvent{Version: 1, CatalogVersion: 2, Input: in, Plugin: p}}); err != nil {
		return PluginRecord{}, false, err
	}
	if err = s.suspendInvalidContext(); err != nil {
		return PluginRecord{}, false, err
	}
	return copyPlugin(p), false, nil
}
func (s *Store) pluginsLocked() []PluginRecord {
	ids := map[string]bool{}
	ids["neko-published-reports"] = true
	for id := range s.plugins {
		ids[id] = true
	}
	for _, p := range builtinPlugins() {
		ids[p.ID] = true
	}
	keys := []string{}
	for id := range ids {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	out := []PluginRecord{}
	for _, id := range keys {
		p, _ := s.plugin(id)
		out = append(out, p)
	}
	return out
}
func pinPlugin(p PluginRecord) PluginPin {
	schema, _ := protocol.Canonical(p.Tools[0].InputSchema)
	h := sha256.Sum256(schema)
	return PluginPin{ID: p.ID, Revision: p.Revision, Publisher: p.Publisher, Version: p.Version, Tool: p.Tools[0].Name, SchemaSHA256: hex.EncodeToString(h[:])}
}
func eligiblePlugin(w Work, id string) bool {
	return id == "neko-artifacts" || id == "neko-project-sources" && w.Resources != nil && len(w.Resources.Files) > 0 || id == "neko-published-reports" && hasPublishedArtifacts(w)
}
func (s *Store) selectPlugins(w Work, selection *[]PluginSelection) ([]PluginPin, error) {
	out := []PluginPin(nil)
	if selection == nil {
		for _, builtin := range policyBuiltinPlugins(w.ToolPolicyVersion) {
			p, _ := s.plugin(builtin.ID)
			if eligiblePlugin(w, p.ID) && p.Status == "ready_builtin_local" {
				out = append(out, pinPlugin(p))
			}
		}
	} else {
		limit := 2
		if w.ToolPolicyVersion == 2 {
			limit = 3
		}
		if len(*selection) > limit {
			return nil, ErrInvalid
		}
		seen := map[string]bool{}
		for _, selected := range *selection {
			p, ok := s.plugin(selected.ID)
			if !ok || !p.TrustedBuiltin || p.Status != "ready_builtin_local" || p.Revision != selected.ExpectedRevision || !eligiblePlugin(w, p.ID) || seen[p.ID] {
				return nil, ErrConflict
			}
			seen[p.ID] = true
			out = append(out, pinPlugin(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if w.Resources != nil && len(w.Resources.Files) > 0 {
		reader := false
		for _, p := range out {
			reader = reader || p.ID == "neko-project-sources"
		}
		if !reader {
			return nil, ErrConflict
		}
	}
	return out, nil
}
func effectivePluginPins(w Work) []PluginPin {
	if w.ToolPolicyVersion != 0 {
		return w.PluginPins
	}
	out := []PluginPin{}
	for _, p := range builtinPlugins() {
		if eligiblePlugin(w, p.ID) {
			out = append(out, pinPlugin(p))
		}
	}
	return out
}
func (s *Store) pluginsAllowed(w Work) bool {
	if w.ToolPolicyVersion < 0 || w.ToolPolicyVersion > 2 {
		return false
	}
	for _, pin := range effectivePluginPins(w) {
		p, ok := s.plugin(pin.ID)
		if !ok || !p.TrustedBuiltin || p.Status != "ready_builtin_local" || pin != pinPlugin(p) {
			return false
		}
	}
	return true
}
func (s *Store) validPluginWork(w Work) bool {
	if w.ToolPolicyVersion == 0 {
		return len(w.PluginPins) == 0 && w.PluginSelection == nil && s.pluginsAllowed(w)
	}
	if w.ToolPolicyVersion != 1 && w.ToolPolicyVersion != 2 {
		return false
	}
	selectionWork, ok := s.groupPluginSelectionWork(w)
	if !ok {
		return false
	}
	pins, err := s.selectPlugins(selectionWork, w.PluginSelection)
	return err == nil && reflect.DeepEqual(pins, w.PluginPins)
}
func pinnedTool(w Work, tool string) bool {
	for _, p := range effectivePluginPins(w) {
		if p.Tool == tool {
			return true
		}
	}
	return false
}
