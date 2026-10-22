package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"nekoaurora.local/native/internal/identity"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const manifestSafeInteger uint64 = 1<<53 - 1

var manifestID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

type ManifestScope struct {
	OwnerUserID      string  `json:"ownerUserId"`
	SpaceID          string  `json:"spaceId"`
	BotID            string  `json:"botId"`
	ProjectID        *string `json:"projectId"`
	ChannelID        string  `json:"channelId"`
	AudienceRevision uint64  `json:"audienceRevision"`
}
type ManifestPrimary struct {
	ID       string `json:"id"`
	SenderID string `json:"senderId"`
	Text     string `json:"text"`
}
type ManifestRange struct{ Offset, End int }
type ManifestSpan struct {
	Offset int    `json:"offset"`
	End    int    `json:"end"`
	SHA256 string `json:"sha256"`
	Text   string `json:"text"`
}
type ManifestEntity struct {
	ID    *string `json:"id"`
	Label string  `json:"label"`
}
type ManifestSource struct {
	ID             string           `json:"id"`
	Kind           string           `json:"kind"`
	Version        uint64           `json:"version"`
	SHA256         string           `json:"sha256"`
	Bytes          int              `json:"bytes"`
	Scope          ManifestScope    `json:"scope"`
	AccessRevision uint64           `json:"accessRevision"`
	ObservedAt     *string          `json:"observedAt"`
	EffectiveFrom  *string          `json:"effectiveFrom"`
	EffectiveTo    *string          `json:"effectiveTo"`
	Entities       []ManifestEntity `json:"entities"`
	Span           ManifestSpan     `json:"span"`
}
type ManifestCandidate struct {
	Key      string       `json:"key"`
	SourceID string       `json:"sourceId"`
	EntityID *string      `json:"entityId"`
	State    string       `json:"state"`
	Span     ManifestSpan `json:"span"`
}
type ContextManifest struct {
	Requester        *identity.VerifiedPrincipal `json:"requester,omitempty"`
	Version          int                         `json:"version"`
	ContextPinSHA256 string                      `json:"contextPinSHA256"`
	Scope            ManifestScope               `json:"scope"`
	PrimaryMessage   ManifestPrimary             `json:"primaryMessage"`
	Sources          []ManifestSource            `json:"sources"`
	FactCandidates   []ManifestCandidate         `json:"factCandidates"`
}

// Only an authenticated intake/launch caller may construct this attestation. Reader is a live catalog record, not client JSON.
type TrustedManifestIntake struct {
	Scope           ManifestScope
	Primary         ManifestPrimary
	CollaborationID string
	PrimaryRange    *ManifestRange
	Reader          *PluginRecord
	Candidates      []ManifestCandidate
}
type BuiltContextManifest struct {
	Manifest ContextManifest
	JSON     []byte
	SHA256   string
}

// This pure builder proves consistency of supplied pins; the caller must still enforce live ACL, account, broker and launch fences.
func BuildContextManifest(w Work, intake TrustedManifestIntake) (BuiltContextManifest, error) {
	if !manifestIntakeMatches(w, intake) || !manifestLiteralSafe(reflect.ValueOf(w.Context)) || !manifestLiteralSafe(reflect.ValueOf(w.Resources)) || !manifestLiteralSafe(reflect.ValueOf(intake)) {
		return BuiltContextManifest{}, ErrInvalid
	}
	primary := intake.Primary
	r := ManifestRange{End: len(primary.Text)}
	if intake.PrimaryRange != nil {
		r = *intake.PrimaryRange
	}
	if !manifestRangeValid(primary.Text, r) {
		return BuiltContextManifest{}, ErrInvalid
	}
	m := ContextManifest{Version: 1, ContextPinSHA256: w.Context.SHA256, Scope: manifestCopyScope(intake.Scope), PrimaryMessage: primary, Sources: []ManifestSource{}, FactCandidates: []ManifestCandidate{}}
	if w.RequesterPin != nil {
		m.Version = 2
		m.Requester = copyRequester(w.RequesterPin)
	}
	m.Sources = append(m.Sources, ManifestSource{ID: primary.ID, Kind: "original-user", Version: 1, SHA256: manifestHash(primary.Text), Bytes: len(primary.Text), Scope: manifestCopyScope(m.Scope), Entities: []ManifestEntity{}, Span: manifestSpan(primary.Text[r.Offset:r.End], r.Offset)})
	addDerived := func(id string, version, access uint64, value any, entities []ManifestEntity) bool {
		b, err := manifestCanonical(value)
		if err != nil || len(b) == 0 || len(b) > 8192 || version == 0 || version > manifestSafeInteger || access > manifestSafeInteger {
			return false
		}
		if entities == nil {
			entities = []ManifestEntity{}
		}
		m.Sources = append(m.Sources, ManifestSource{ID: id, Kind: "derived", Version: version, SHA256: manifestHash(string(b)), Bytes: len(b), Scope: manifestCopyScope(m.Scope), AccessRevision: access, Entities: entities, Span: manifestSpan(string(b), 0)})
		return true
	}
	if w.Collaboration != nil {
		if !addDerived("stage:"+w.RequestID, uint64(w.Collaboration.Stage), 0, map[string]any{"kind": "generated-collaboration-stage", "collaborationId": w.Collaboration.ID, "stage": w.Collaboration.Stage, "role": "assistant", "text": w.Text, "ownerGoalMessageId": primary.ID}, nil) {
			return BuiltContextManifest{}, ErrInvalid
		}
	}
	for _, source := range w.Context.Sources {
		if !manifestID.MatchString(source.RequestID) || !manifestID.MatchString(source.MessageID) || source.Epoch == 0 || !addDerived("history:"+source.RequestID, source.Epoch, source.AccessRevision, map[string]any{"kind": "pinned-prior-turn", "requestId": source.RequestID, "messageId": source.MessageID, "botId": source.BotID, "sequence": source.Sequence, "epoch": source.Epoch, "artifacts": source.Artifacts, "messages": []any{map[string]any{"role": "user", "senderId": nil, "origin": "prior task input; original authorship not attested", "text": source.Text}, map[string]any{"role": "assistant", "botId": source.BotID, "text": source.Reply}}}, nil) {
			return BuiltContextManifest{}, ErrInvalid
		}
	}
	publishedFiles := map[string]ManifestSource{}
	for _, report := range w.Context.PublishedReports {
		if report.Source == nil || report.Version != 1 || report.SHA256 != publishedReportDigest(report) || !addDerived("published:"+report.DeliveryID, report.Sequence, report.Source.AccessRevision, report, nil) {
			return BuiltContextManifest{}, ErrInvalid
		}
		for _, file := range report.Files {
			a := file.Artifact
			if !lowerHex(a.ID, 64) || !lowerHex(a.SHA256, 64) || !manifestRangeValid(file.Excerpt, ManifestRange{End: len(file.Excerpt)}) || len(file.Excerpt) > 4000 || len(file.Excerpt) > a.Bytes {
				return BuiltContextManifest{}, ErrInvalid
			}
			candidate := ManifestSource{ID: a.ID, Kind: "retrieved", Version: 1, SHA256: a.SHA256, Bytes: a.Bytes, Scope: manifestCopyScope(m.Scope), AccessRevision: report.Source.AccessRevision, Entities: []ManifestEntity{}, Span: manifestSpan(file.Excerpt, 0)}
			if prior, found := publishedFiles[a.ID]; found {
				if !reflect.DeepEqual(prior, candidate) {
					return BuiltContextManifest{}, ErrInvalid
				}
				continue
			}
			publishedFiles[a.ID] = candidate
			m.Sources = append(m.Sources, candidate)
		}
	}
	if w.Resources != nil {
		resources := w.Resources
		p := resources.Project
		if !addDerived("project:"+p.ID, p.Revision, 0, map[string]any{"kind": "pinned-project-description", "project": p, "pluginPins": w.PluginPins}, nil) {
			return BuiltContextManifest{}, ErrInvalid
		}
		if resources.Continuity != nil {
			pin := resources.Continuity
			c := pin.Metadata
			if c == nil || c.Version != 1 || c.AuthorType != "owner" || c.OwnerUserID != w.OwnerUserID || c.SpaceID != w.SpaceID || c.ProjectID != p.ID || c.ChannelID != w.ConversationID || c.ProjectRevision == 0 || c.ProjectRevision > p.Revision || c.ChannelRevision == 0 || c.ChannelRevision > m.Scope.AudienceRevision || c.Revision != pin.Revision || c.Revision == 0 || c.SHA256 != pin.SHA256 || continuityDigest(*c) != pin.SHA256 || !validContinuityData(*c) {
				return BuiltContextManifest{}, ErrInvalid
			}
			entities := []ManifestEntity{}
			for _, d := range c.Decisions {
				if d.EntityID != "" {
					id := d.EntityID
					entities = append(entities, ManifestEntity{ID: &id, Label: d.ID})
				}
			}
			for _, ref := range c.References {
				entity := ManifestEntity{Label: ref.Title}
				if ref.EntityID != "" {
					id := ref.EntityID
					entity.ID = &id
				}
				entities = append(entities, entity)
			}
			if !addDerived("continuity:"+p.ID, c.Revision, 0, map[string]any{"kind": "owner-recorded-project-continuity", "authority": "descriptive metadata only; references are not read grants", "record": c}, entities) {
				return BuiltContextManifest{}, ErrInvalid
			}
		}
		if len(resources.Files) > 0 && !manifestReaderMatches(w, intake.Reader) {
			return BuiltContextManifest{}, ErrInvalid
		}
		for _, file := range resources.Files {
			if !manifestFileMatches(file, w) {
				return BuiltContextManifest{}, ErrInvalid
			}
			textSHA, textBytes := fileTextIdentity(file)
			m.Sources = append(m.Sources, ManifestSource{ID: file.ID, Kind: "retrieved", Version: uint64(file.Version), SHA256: textSHA, Bytes: textBytes, Scope: manifestCopyScope(m.Scope), AccessRevision: file.AccessRevision, Entities: []ManifestEntity{}, Span: manifestSpan(file.Excerpt, 0)})
		}
	}
	for _, candidate := range intake.Candidates {
		c := candidate
		if c.EntityID != nil {
			id := *c.EntityID
			c.EntityID = &id
		}
		m.FactCandidates = append(m.FactCandidates, c)
	}
	if !manifestValid(m) {
		return BuiltContextManifest{}, ErrInvalid
	}
	b, err := manifestCanonical(m)
	if err != nil || len(b) > 48*1024 {
		return BuiltContextManifest{}, ErrInvalid
	}
	return BuiltContextManifest{Manifest: m, JSON: b, SHA256: manifestHash(string(b))}, nil
}

func manifestIntakeMatches(w Work, in TrustedManifestIntake) bool {
	s := in.Scope
	if !validRequesterWork(w) || !manifestScopeValid(s) || s.OwnerUserID != w.OwnerUserID || s.SpaceID != w.SpaceID || s.BotID != w.BotID || s.ChannelID != w.ConversationID || w.Context == nil || !manifestID.MatchString(w.RequestID) || !manifestID.MatchString(w.SourceMessageID) || !manifestID.MatchString(in.Primary.ID) || in.Primary.SenderID != requesterActor(w) || len(in.Primary.Text) == 0 || len(in.Primary.Text) > 32768 || !utf8.ValidString(in.Primary.Text) || hasSecret([]byte(in.Primary.Text)) {
		return false
	}
	pub := w.Publication
	if pub == nil || pub.Version != 1 || pub.ActorID != requesterActor(w) || pub.SourceChannelID != w.ConversationID || pub.ReplyChannelID != w.ConversationID || pub.SourceMessageID != w.SourceMessageID || pub.AudienceRevision != s.AudienceRevision {
		return false
	}
	if w.Collaboration == nil {
		if in.CollaborationID != "" || in.Primary.ID != w.SourceMessageID || in.Primary.Text != w.Text {
			return false
		}
	} else {
		g := Collaboration{ID: in.CollaborationID, CollaborationInput: CollaborationInput{Goal: in.Primary.Text}}
		if !manifestID.MatchString(g.ID) || g.ID != w.Collaboration.ID || w.Collaboration.Stage < 1 || w.Collaboration.Stage > 3 || in.Primary.ID == w.SourceMessageID || w.Text != stageText(g, w.Collaboration.Stage) {
			return false
		}
	}
	if w.Resources == nil {
		if s.ProjectID != nil {
			return false
		}
	} else {
		p := w.Resources.Project
		if s.ProjectID == nil || *s.ProjectID != p.ID || !manifestID.MatchString(p.ID) || p.OwnerUserID != w.OwnerUserID || p.SpaceID != w.SpaceID || p.Revision == 0 || !contains(p.ChannelIDs, w.ConversationID) {
			return false
		}
	}
	context := *w.Context
	if context.Version < 1 || context.Version > 5 || (context.Version < 5 && len(context.PublishedReports) > 0) || (w.Resources != nil && context.Version < 3) || context.OwnerUserID != w.OwnerUserID || context.SpaceID != w.SpaceID || context.BotID != w.BotID || context.ConversationID != w.ConversationID || context.BotConfigRevision < 1 || context.BotConfigRevision != w.BotConfigRevision || context.ToolPolicyVersion != w.ToolPolicyVersion || !reflect.DeepEqual(context.PluginPins, w.PluginPins) || !reflect.DeepEqual(context.Resources, w.Resources) || !reflect.DeepEqual(context.Profile, w.BotProfile) || !lowerHex(context.SHA256, 64) {
		return false
	}
	if w.BotProfile != nil && ((context.Version != 4 && context.Version != 5) || w.BotProfile.BotID != w.BotID || w.BotProfile.Revision < 1 || w.BotProfile.Revision > manifestSafeInteger || !validProfileText(w.BotProfile.Instructions)) {
		return false
	}
	expected := context.SHA256
	context.SHA256 = ""
	b, err := json.Marshal(context)
	return err == nil && manifestHash(string(b)) == expected
}
func manifestScopeValid(s ManifestScope) bool {
	return manifestID.MatchString(s.OwnerUserID) && manifestID.MatchString(s.SpaceID) && manifestID.MatchString(s.BotID) && manifestID.MatchString(s.ChannelID) && (s.ProjectID == nil || manifestID.MatchString(*s.ProjectID)) && s.AudienceRevision > 0 && s.AudienceRevision <= manifestSafeInteger
}
func manifestCopyScope(s ManifestScope) ManifestScope {
	if s.ProjectID != nil {
		p := *s.ProjectID
		s.ProjectID = &p
	}
	return s
}
func manifestRangeValid(text string, r ManifestRange) bool {
	return r.Offset >= 0 && r.End > r.Offset && r.End <= len(text) && r.End-r.Offset <= 8192 && utf8.ValidString(text[r.Offset:r.End])
}
func manifestSpan(text string, offset int) ManifestSpan {
	return ManifestSpan{Offset: offset, End: offset + len(text), SHA256: manifestHash(text), Text: text}
}
func manifestHash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
func manifestReaderMatches(w Work, reader *PluginRecord) bool {
	if (w.ToolPolicyVersion != 1 && w.ToolPolicyVersion != 2) || reader == nil || !reader.TrustedBuiltin || reader.ID != "neko-project-sources" || reader.Publisher != "original-host-implementation" || reader.Status != "ready_builtin_local" || reader.Version != "1.0.0" || len(reader.Tools) != 1 || reader.Tools[0].Name != "project.read_file" {
		return false
	}
	current := pinPlugin(*reader)
	known := pinPlugin(builtinPlugins()[1])
	if current.SchemaSHA256 != known.SchemaSHA256 {
		return false
	}
	for _, p := range w.PluginPins {
		if p.ID == current.ID {
			return reflect.DeepEqual(p, current)
		}
	}
	return false
}
func manifestFileMatches(f UploadedFile, w Work) bool {
	sha, size := fileTextIdentity(f)
	format := f.MIME == "text/plain; charset=utf-8" && f.ExtractionVersion == 0 && f.TextFormat == "" && f.TextSHA256 == "" && f.TextBytes == 0 || extractedDescriptor(f)
	return manifestID.MatchString(f.ID) && f.ProjectID == w.Resources.Project.ID && f.ChannelID == w.ConversationID && f.OwnerUserID == w.OwnerUserID && f.SpaceID == w.SpaceID && f.Granted && f.Status == "ready_text" && format && f.Version > 0 && f.Bytes > 0 && f.Bytes <= 1<<20 && f.AccessRevision <= manifestSafeInteger && lowerHex(f.SHA256, 64) && manifestRangeValid(f.Excerpt, ManifestRange{End: len(f.Excerpt)}) && len(f.Excerpt) <= size && (f.Truncated && len(f.Excerpt) < size || !f.Truncated && len(f.Excerpt) == size && sha == manifestHash(f.Excerpt))
}
func manifestValid(m ContextManifest) bool {
	if m.Version != 1 && m.Version != 2 || m.Version == 1 && m.Requester != nil || m.Version == 2 && (m.Requester == nil || !identity.ValidPrincipal(*m.Requester) || m.Requester.SpaceID != m.Scope.SpaceID || m.PrimaryMessage.SenderID != m.Requester.ActorID) {
		return false
	}
	if len(m.Sources) > 16 || len(m.FactCandidates) > 64 {
		return false
	}
	ids := map[string]ManifestSource{}
	for _, s := range m.Sources {
		if !manifestID.MatchString(s.ID) || !reflect.DeepEqual(s.Scope, m.Scope) || s.Version == 0 || s.Version > manifestSafeInteger || s.Bytes < 1 || s.Bytes > 1<<20 || s.Span.Offset < 0 || s.Span.End > s.Bytes || !manifestExactSpan(s.Span) || !lowerHex(s.SHA256, 64) || len(s.Entities) > 32 {
			return false
		}
		if _, exists := ids[s.ID]; exists {
			return false
		}
		ids[s.ID] = s
		for _, e := range s.Entities {
			if e.ID != nil && !manifestID.MatchString(*e.ID) || strings.TrimSpace(e.Label) == "" || len(e.Label) > 256 || !utf8.ValidString(e.Label) {
				return false
			}
		}
	}
	for _, c := range m.FactCandidates {
		s, ok := ids[c.SourceID]
		if !ok || !manifestID.MatchString(c.Key) || c.State != "unresolved" || !manifestExactSpan(c.Span) || c.Span.Offset < s.Span.Offset || c.Span.End > s.Span.End || s.Span.Text[c.Span.Offset-s.Span.Offset:c.Span.End-s.Span.Offset] != c.Span.Text {
			return false
		}
		if c.EntityID != nil {
			found := false
			for _, e := range s.Entities {
				if e.ID != nil && *e.ID == *c.EntityID {
					found = true
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}
func manifestExactSpan(s ManifestSpan) bool {
	return s.Offset >= 0 && s.End > s.Offset && s.End-s.Offset == len(s.Text) && len(s.Text) <= 8192 && utf8.ValidString(s.Text) && s.SHA256 == manifestHash(s.Text)
}

func manifestLiteralSafe(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return v.IsNil() || manifestLiteralSafe(v.Elem())
	case reflect.String:
		return utf8.ValidString(v.String()) && !hasSecret([]byte(v.String()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() <= manifestSafeInteger
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() >= -int64(manifestSafeInteger) && v.Int() <= int64(manifestSafeInteger)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !manifestLiteralSafe(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !manifestLiteralSafe(v.Index(i)) {
				return false
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if !manifestLiteralSafe(key) || !manifestLiteralSafe(v.MapIndex(key)) {
				return false
			}
		}
	}
	return true
}

// Core hashes ordinal JSON.stringify strings, including literal HTML characters and U+2028/U+2029.
func manifestCanonical(value any) ([]byte, error) {
	if !manifestLiteralSafe(reflect.ValueOf(value)) {
		return nil, ErrInvalid
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var tree any
	if decoder.Decode(&tree) != nil {
		return nil, ErrInvalid
	}
	var b bytes.Buffer
	if !manifestEncode(&b, tree) {
		return nil, ErrInvalid
	}
	return b.Bytes(), nil
}
func manifestEncode(b *bytes.Buffer, v any) bool {
	switch value := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(value))
	case json.Number:
		n, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil || n < -int64(manifestSafeInteger) || n > int64(manifestSafeInteger) {
			return false
		}
		b.WriteString(strconv.FormatInt(n, 10))
	case string:
		manifestQuote(b, value)
	case []any:
		b.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				b.WriteByte(',')
			}
			if !manifestEncode(b, item) {
				return false
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, z := utf16.Encode([]rune(keys[i])), utf16.Encode([]rune(keys[j]))
			for n := 0; n < len(a) && n < len(z); n++ {
				if a[n] != z[n] {
					return a[n] < z[n]
				}
			}
			return len(a) < len(z)
		})
		b.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			manifestQuote(b, key)
			b.WriteByte(':')
			if !manifestEncode(b, value[key]) {
				return false
			}
		}
		b.WriteByte('}')
	default:
		return false
	}
	return true
}
func manifestQuote(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				b.WriteByte("0123456789abcdef"[r>>4])
				b.WriteByte("0123456789abcdef"[r&15])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
