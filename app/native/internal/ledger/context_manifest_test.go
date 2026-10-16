package ledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func manifestWork(text string) (Work, TrustedManifestIntake) {
	w := Work{RequestID: "request-one", SourceMessageID: "message-one", OwnerUserID: "local-owner", SpaceID: "local-space", BotID: "bot-meiiie", ConversationID: "conversation-bot-meiiie", BotConfigRevision: 1, Text: text, Epoch: 1}
	w.Publication = &Publication{Version: 1, ActorID: w.OwnerUserID, SourceChannelID: w.ConversationID, SourceMessageID: w.SourceMessageID, ReplyChannelID: w.ConversationID, AudienceRevision: 1}
	w.Context = &ContextPin{Version: 3, OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, BotID: w.BotID, ConversationID: w.ConversationID, BotConfigRevision: 1, Sources: []ContextSource{}}
	sealManifestWork(&w)
	in := TrustedManifestIntake{Scope: ManifestScope{OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, BotID: w.BotID, ChannelID: w.ConversationID, AudienceRevision: 1}, Primary: ManifestPrimary{ID: w.SourceMessageID, SenderID: w.OwnerUserID, Text: text}}
	return w, in
}

func sealManifestWork(w *Work) {
	w.Context.Resources = copyResources(w.Resources)
	w.Context.ToolPolicyVersion = w.ToolPolicyVersion
	w.Context.PluginPins = append([]PluginPin(nil), w.PluginPins...)
	w.Context.SHA256 = ""
	b, _ := json.Marshal(w.Context)
	w.Context.SHA256 = manifestHash(string(b))
}

func addManifestProject(w *Work, in *TrustedManifestIntake) {
	p := Project{ID: "project-one", Name: "Same name", Goal: "Read descriptive sources", Revision: 1, OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, ChannelIDs: []string{w.ConversationID}}
	w.Resources = &ResourcePin{Project: p, RequestedFileIDs: []string{}, Files: []UploadedFile{}}
	in.Scope.ProjectID = &p.ID
	sealManifestWork(w)
}

func addManifestFile(w *Work, in *TrustedManifestIntake) {
	addManifestProject(w, in)
	f := UploadedFile{ID: "file-one", ProjectID: w.Resources.Project.ID, ChannelID: w.ConversationID, OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, Filename: "note.txt", MIME: "text/plain; charset=utf-8", Excerpt: "Selected Tiếng Việt <>&\u2028\u2029", Version: 1, Status: "ready_text", Granted: true, AccessRevision: 2}
	f.Bytes, f.SHA256 = len(f.Excerpt), manifestHash(f.Excerpt)
	w.Resources.Files = []UploadedFile{f}
	reader := builtinPlugins()[1]
	in.Reader = &reader
	w.ToolPolicyVersion, w.PluginPins = 1, []PluginPin{pinPlugin(reader)}
	sealManifestWork(w)
}

func TestManifestOriginalAndCanonicalStrings(t *testing.T) {
	w, in := manifestWork("Owner <>& Tiếng Việt \u2028\u2029 literal \\u2028\n\t\"\\")
	out, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Manifest.Sources) != 1 || out.Manifest.Sources[0].Kind != "original-user" || out.Manifest.PrimaryMessage.Text != w.Text || out.Manifest.Sources[0].Span.Text != w.Text || len(out.Manifest.FactCandidates) != 0 {
		t.Fatal("primary authority or exact text changed")
	}
	if strings.Contains(string(out.JSON), `\u003c`) || strings.Contains(string(out.JSON), `\u2029`) || !strings.Contains(string(out.JSON), `\\u2028`) || !strings.Contains(string(out.JSON), "\u2028") {
		t.Fatal("canonical string escaping disagrees with JS")
	}
	if out.SHA256 != "5df116dbd75e0f0b177be980ca21e9381222e8dc0fc9b1b0af011a5b26b34558" {
		t.Fatal("frozen Core156 digest golden changed")
	}
	var decoded ContextManifest
	if json.Unmarshal(out.JSON, &decoded) != nil || !reflect.DeepEqual(decoded, out.Manifest) {
		t.Fatal("manifest JSON round trip changed literals/nulls")
	}
}

func TestManifestScopeAndPinNegatives(t *testing.T) {
	for _, name := range []string{"owner", "space", "bot", "channel", "audience", "primary-id", "primary-text", "publication", "pin-hash", "pin-bot", "pin-resources", "safe-integer", "invalid-utf8", "secret"} {
		t.Run(name, func(t *testing.T) {
			w, in := manifestWork("original owner instruction")
			switch name {
			case "owner":
				in.Scope.OwnerUserID = "other-owner"
			case "space":
				in.Scope.SpaceID = "other-space"
			case "bot":
				in.Scope.BotID = "bot-davin"
			case "channel":
				in.Scope.ChannelID = "other-channel"
			case "audience":
				in.Scope.AudienceRevision++
			case "primary-id":
				in.Primary.ID = "other-message"
			case "primary-text":
				in.Primary.Text = "wrapped quote"
			case "publication":
				w.Publication = nil
			case "pin-hash":
				w.Context.Sources = append(w.Context.Sources, ContextSource{Text: "tamper"})
			case "pin-bot":
				w.Context.BotID = "bot-davin"
				sealManifestWork(&w)
			case "pin-resources":
				addManifestProject(&w, &in)
				w.Resources.Project.Revision++
			case "safe-integer":
				in.Scope.AudienceRevision = 1 << 53
				w.Publication.AudienceRevision = in.Scope.AudienceRevision
			case "invalid-utf8":
				w.Text = string([]byte{0xff})
				in.Primary.Text = w.Text
			case "secret":
				w.Text = "api_key=\"" + strings.Repeat("x", 32) + "\""
				in.Primary.Text = w.Text
			}
			if _, err := BuildContextManifest(w, in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected local invalid, got %v", err)
			}
		})
	}
}

func TestManifestPrimaryRangeIsExplicitAndUTF8Exact(t *testing.T) {
	w, in := manifestWork(strings.Repeat("é", 5000))
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("implicitly selected/truncated a long request")
	}
	in.PrimaryRange = &ManifestRange{Offset: 0, End: 8192}
	out, err := BuildContextManifest(w, in)
	if err != nil || out.Manifest.Sources[0].Bytes != 10000 || out.Manifest.PrimaryMessage.Text != w.Text || len(out.Manifest.Sources[0].Span.Text) != 8192 {
		t.Fatalf("full primary not retained: %v", err)
	}
	in.PrimaryRange.End = 8191
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("split UTF-8 accepted")
	}
	in.PrimaryRange = &ManifestRange{Offset: 2, End: 4}
	if _, err := BuildContextManifest(w, in); err != nil {
		t.Fatal(err)
	}
	w.Text = strings.Repeat("x", 32769)
	in.Primary.Text = w.Text
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("oversized primary accepted")
	}
}

func TestManifestPreviousTurnsRemainDerivedWithRoles(t *testing.T) {
	w, in := manifestWork("Current request")
	w.Context.Sources = []ContextSource{{RequestID: "request-prior", MessageID: "message-prior", Sequence: 1, Epoch: 2, Text: "Previous owner says A", Reply: "Assistant says B", BotID: "bot-davin", AccessRevision: 3}}
	sealManifestWork(&w)
	out, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal(err)
	}
	s := out.Manifest.Sources[1]
	if s.Kind != "derived" || s.ObservedAt != nil || !strings.Contains(s.Span.Text, `"role":"user"`) || !strings.Contains(s.Span.Text, `"role":"assistant"`) || !strings.Contains(s.Span.Text, `"botId":"bot-davin"`) || !strings.Contains(s.Span.Text, `"senderId":null`) || !strings.Contains(s.Span.Text, "original authorship not attested") {
		t.Fatal("prior roles/provenance changed")
	}
	w.Context.Sources[0].Reply = strings.Repeat("x", 8192)
	sealManifestWork(&w)
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("oversized wrapper silently pruned")
	}
}

func TestManifestCollaborationNeedsAuthenticatedGoal(t *testing.T) {
	w, in := manifestWork("Original group goal")
	g := Collaboration{ID: "collab-one", CollaborationInput: CollaborationInput{Goal: in.Primary.Text}}
	in.Primary.ID = g.ID
	w.Collaboration = &CollaborationBinding{ID: g.ID, Stage: 2}
	w.Text, w.SourceMessageID = stageText(g, 2), "message-stage-two"
	w.Publication.SourceMessageID = w.SourceMessageID
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("group owner goal silently inferred")
	}
	in.CollaborationID = g.ID
	out, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Manifest.PrimaryMessage.Text != g.Goal || out.Manifest.Sources[1].Kind != "derived" || !strings.Contains(out.Manifest.Sources[1].Span.Text, "Finite collaboration") {
		t.Fatal("generated stage minted user authority")
	}
	in.Primary.Text = w.Text
	in.Primary.ID = w.SourceMessageID
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("generated stage accepted as owner goal")
	}
}

func TestManifestFilesNeedExactExplicitLiveReaderPin(t *testing.T) {
	w, in := manifestWork("Read file")
	addManifestFile(&w, &in)
	out, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal(err)
	}
	f := w.Resources.Files[0]
	s := out.Manifest.Sources[2]
	if s.ID != f.ID || s.Kind != "retrieved" || s.SHA256 != f.SHA256 || s.Version != uint64(f.Version) || s.AccessRevision != f.AccessRevision || s.Span.Text != f.Excerpt {
		t.Fatal("file provenance changed")
	}
	for _, name := range []string{"missing-reader", "disabled", "reader-revision", "untrusted", "wrong-schema", "legacy", "foreign-file", "revoked", "bad-hash", "extent", "version", "redacted-excerpt"} {
		t.Run(name, func(t *testing.T) {
			w, in := manifestWork("Read file")
			addManifestFile(&w, &in)
			switch name {
			case "missing-reader":
				in.Reader = nil
			case "disabled":
				in.Reader.Status = "disabled"
			case "reader-revision":
				in.Reader.Revision++
			case "untrusted":
				in.Reader.TrustedBuiltin = false
			case "wrong-schema":
				in.Reader.Tools[0].InputSchema = json.RawMessage(`{}`)
			case "legacy":
				w.ToolPolicyVersion = 0
				w.PluginPins = nil
			case "foreign-file":
				w.Resources.Files[0].ChannelID = "private-channel"
			case "revoked":
				w.Resources.Files[0].Granted = false
			case "bad-hash":
				w.Resources.Files[0].SHA256 = strings.Repeat("a", 64)
			case "extent":
				w.Resources.Files[0].Bytes = 2
			case "version":
				w.Resources.Files[0].Version = 0
			case "redacted-excerpt":
				w.Resources.Files[0].Excerpt = ""
			}
			sealManifestWork(&w)
			if _, err := BuildContextManifest(w, in); err == nil {
				t.Fatal("unsafe retrieval admitted")
			}
		})
	}
}

func TestManifestContinuityPreservesLiteralProvenanceWithoutGrant(t *testing.T) {
	w, in := manifestWork("Summarize recorded project notes")
	addManifestProject(&w, &in)
	c := ProjectContinuity{Version: 1, ProjectID: w.Resources.Project.ID, ChannelID: w.ConversationID, ProjectRevision: 1, ChannelRevision: 1, Revision: 1, OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, AuthorType: "owner", RecordedAt: "2026-10-11T00:00:00Z", Instructions: "Literal owner instructions", Decisions: []ProjectDecision{{ID: "decision-one", Text: "Explicitly contested", Status: "contested", EffectiveDate: "2026-10-10", EntityID: "person-a", ReferenceIDs: []string{"reference-one"}}}, References: []ProjectReference{{ID: "reference-one", Title: "Alex", SourceDate: "2026-10-09", EntityID: "person-a", URL: "https://example.org/a"}, {ID: "reference-two", Title: "Alex", EntityID: "person-b", File: &ProjectFileReference{FileID: "file-" + strings.Repeat("b", 32), SHA256: strings.Repeat("a", 64), AccessRevision: 2}}}}
	c.SHA256 = continuityDigest(c)
	w.Resources.Continuity = &ContinuityPin{Revision: 1, SHA256: c.SHA256, Metadata: &c}
	sealManifestWork(&w)
	out, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal(err)
	}
	s := out.Manifest.Sources[2]
	if s.Kind != "derived" || s.EffectiveFrom != nil || s.EffectiveTo != nil || s.ObservedAt != nil || len(out.Manifest.Sources) != 3 || len(s.Entities) != 3 || len(out.Manifest.FactCandidates) != 0 || !strings.Contains(s.Span.Text, `"status":"contested"`) || !strings.Contains(s.Span.Text, "2026-10-09") {
		t.Fatal("metadata invented inference or read grant")
	}
	for _, entity := range s.Entities {
		if entity.ID == nil {
			t.Fatal("explicit conflicting entity ID dropped")
		}
	}
	w.Resources.Continuity.Metadata = nil
	sealManifestWork(&w)
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("public redaction used as private continuity")
	}
}

func TestManifestUnresolvedCandidatesAndBudgets(t *testing.T) {
	w, in := manifestWork("hello world")
	in.Candidates = []ManifestCandidate{{Key: "candidate-one", SourceID: w.SourceMessageID, State: "unresolved", Span: ManifestSpan{Offset: 0, End: 5, SHA256: manifestHash("hello"), Text: "hello"}}}
	out, err := BuildContextManifest(w, in)
	if err != nil || len(out.Manifest.FactCandidates) != 1 {
		t.Fatal(err)
	}
	in.Candidates[0].State = "confirmed"
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("resolved fact accepted")
	}
	in.Candidates[0].State = "unresolved"
	in.Candidates[0].Span.Text = "other"
	in.Candidates[0].Span.SHA256 = manifestHash("other")
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("candidate quote changed")
	}
	in.Candidates = make([]ManifestCandidate, 65)
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("candidate budget exceeded")
	}
	w, in = manifestWork("Small")
	for i := 0; i < 16; i++ {
		w.Context.Sources = append(w.Context.Sources, ContextSource{RequestID: "history-" + strings.Repeat("a", i+1), MessageID: "message-prior", Epoch: 1, Text: "x", Reply: "y"})
	}
	sealManifestWork(&w)
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("source budget exceeded")
	}
	w, in = manifestWork("Small")
	for i := 0; i < 8; i++ {
		w.Context.Sources = append(w.Context.Sources, ContextSource{RequestID: "history-" + strings.Repeat("a", i+1), MessageID: "message-prior", Epoch: 1, Text: strings.Repeat("x", 6000), Reply: "y"})
	}
	sealManifestWork(&w)
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("wire budget exceeded")
	}
}

func TestManifestExportCoreFixtures(t *testing.T) {
	dir := os.Getenv("NEKO_CONTEXT_MANIFEST_FIXTURE_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unicode-primary", "project-file", "explicit-primary-range", "derived-history", "group-goal", "unresolved-candidate"} {
		w, in := manifestWork("Owner <>& Tiếng Việt \u2028\u2029 literal \\u2028\n\t\"\\")
		switch name {
		case "project-file":
			addManifestFile(&w, &in)
		case "explicit-primary-range":
			w.Text = strings.Repeat("é", 5000)
			in.Primary.Text = w.Text
			in.PrimaryRange = &ManifestRange{Offset: 0, End: 8192}
		case "derived-history":
			w.Context.Sources = []ContextSource{{RequestID: "prior-one", MessageID: "prior-message", Epoch: 1, Text: "user literal", Reply: "assistant literal"}}
			sealManifestWork(&w)
		case "group-goal":
			g := Collaboration{ID: "collab-one", CollaborationInput: CollaborationInput{Goal: in.Primary.Text}}
			in.Primary.ID = g.ID
			w.Collaboration = &CollaborationBinding{ID: g.ID, Stage: 2}
			w.Text = stageText(g, 2)
			w.SourceMessageID = "stage-message"
			w.Publication.SourceMessageID = w.SourceMessageID
			in.CollaborationID = g.ID
		case "unresolved-candidate":
			in.Candidates = []ManifestCandidate{{Key: "candidate-literal", SourceID: in.Primary.ID, State: "unresolved", Span: manifestSpan("Owner", 0)}}
		}
		out, err := BuildContextManifest(w, in)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), out.JSON, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".sha256"), []byte(out.SHA256), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManifestGenuineStorePinsReplayAndReadBoundary(t *testing.T) {
	s, c, p := productFixture(t)
	if _, _, err := s.SaveProjectContinuity(continuityInput(p, c, "manifest-record-nonce")); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID, ClientNonce: "manifest-file-nonce", Filename: "quote.txt", Content: []byte(strings.Repeat("é", 3000))})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "manifest-real-intake", Text: "@davin compare selected quote", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision, FileIDs: []string{f.ID}})
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := s.plugin("neko-project-sources")
	if !ok {
		t.Fatal("reader absent")
	}
	in := TrustedManifestIntake{Scope: ManifestScope{OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, BotID: w.BotID, ProjectID: &p.ID, ChannelID: c.ID, AudienceRevision: c.Revision}, Primary: ManifestPrimary{ID: w.SourceMessageID, SenderID: w.OwnerUserID, Text: w.Text}, Reader: &reader}
	before := s.Snapshot().Sequence
	out, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal("genuine pin rejected", err)
	}
	if s.Snapshot().Sequence != before || out.Manifest.Scope.BotID != "bot-davin" || len(out.Manifest.Sources) != 4 || out.Manifest.Sources[3].Bytes != f.Bytes || len(out.Manifest.Sources[3].Span.Text) != 4000 || out.Manifest.Sources[3].SHA256 != f.SHA256 {
		t.Fatal("actual routing/resource span changed or journal mutated")
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, _ := reopened.Get(w.RequestID)
	replayed, err := BuildContextManifest(saved, in)
	if err != nil || replayed.SHA256 != out.SHA256 {
		t.Fatal("replay altered manifest", err)
	}
	if _, err := BuildContextManifest(PublicContinuityWork(saved), in); err == nil {
		t.Fatal("redacted public work admitted")
	}
	if target := os.Getenv("NEKO_CONTEXT_MANIFEST_FIXTURE_DIR"); target != "" {
		if err := os.MkdirAll(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "genuine-store-replay.json"), out.JSON, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "genuine-store-replay.sha256"), []byte(out.SHA256), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManifestCanonicalOrdinalAndGoPinRemainDistinct(t *testing.T) {
	value := map[string]any{"\ue000": "BMP", "😀": "astral", "a": []any{"<>&\u2028\u2029", "\\u2028", 9007199254740991}, "z": nil}
	b, err := manifestCanonical(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(b), "😀") > strings.Index(string(b), "\ue000") {
		t.Fatal("keys sorted UTF8 rather than JS UTF16 ordinal")
	}
	if manifestHash(string(b)) != "a215be35b5665a92c0742a8ff34ef9cb69dfee74dba76d1d59eaa83cc79259ad" {
		t.Fatal("frozen Core156 ordinal golden changed")
	}
	if target := os.Getenv("NEKO_CONTEXT_MANIFEST_FIXTURE_DIR"); target != "" {
		if err := os.MkdirAll(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "canonical-ordinal.value.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "canonical-ordinal.value.sha256"), []byte(manifestHash(string(b))), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w, in := manifestWork("Current <>&")
	w.Context.BotName = "Name <>&\u2028"
	sealManifestWork(&w)
	if _, err := BuildContextManifest(w, in); err != nil {
		t.Fatal(err)
	}
	pin := *w.Context
	pin.SHA256 = ""
	canonical, _ := manifestCanonical(pin)
	w.Context.SHA256 = manifestHash(string(canonical))
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("accepted Core canonical hash in place of existing Go pin digest")
	}
}

func TestManifestAdditionalBoundsAndNoInputAliasing(t *testing.T) {
	w, in := manifestWork(strings.Repeat("x", 32768))
	in.PrimaryRange = &ManifestRange{End: 8192}
	if _, err := BuildContextManifest(w, in); err != nil {
		t.Fatal("maximum full primary with selected exact span", err)
	}
	w, in = manifestWork("request")
	addManifestFile(&w, &in)
	out, err := BuildContextManifest(w, in)
	if err != nil {
		t.Fatal(err)
	}
	*in.Scope.ProjectID = "changed-project"
	in.Reader.Revision++
	w.Resources.Files[0].Excerpt = "changed"
	if *out.Manifest.Scope.ProjectID != "project-one" || out.Manifest.Sources[2].Span.Text == "changed" {
		t.Fatal("manifest aliased caller inputs")
	}
	w, in = manifestWork("request")
	addManifestFile(&w, &in)
	w.Resources.Files[0].ID = w.SourceMessageID
	sealManifestWork(&w)
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("duplicate source ID accepted")
	}
	w, in = manifestWork("request")
	addManifestFile(&w, &in)
	w.Resources.Files[0].Excerpt = string([]byte{0xff})
	sealManifestWork(&w)
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("file invalid UTF8 replacement admitted")
	}
	w, in = manifestWork("request")
	in.Candidates = []ManifestCandidate{{Key: "candidate", SourceID: w.SourceMessageID, State: "unresolved", EntityID: func() *string { id := "person-unknown"; return &id }(), Span: manifestSpan("request", 0)}}
	if _, err := BuildContextManifest(w, in); err == nil {
		t.Fatal("unknown entity candidate admitted")
	}
}

func TestManifestContinuityUnknownsAndTamperStayLocal(t *testing.T) {
	for _, name := range []string{"unknown-literal", "foreign-author", "bad-record-hash", "invalid-entity", "raw-metadata-secret"} {
		t.Run(name, func(t *testing.T) {
			w, in := manifestWork("Read recorded note")
			addManifestProject(&w, &in)
			c := ProjectContinuity{Version: 1, ProjectID: w.Resources.Project.ID, ChannelID: w.ConversationID, ProjectRevision: 1, ChannelRevision: 1, Revision: 1, OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, AuthorType: "owner", Instructions: "Unresolved literal", References: []ProjectReference{{ID: "unknown-ref", Title: "Alex", URL: "https://example.org/note"}}, Decisions: []ProjectDecision{}}
			switch name {
			case "foreign-author":
				c.OwnerUserID = "other-owner"
			case "invalid-entity":
				c.References[0].EntityID = "Alex with spaces"
			case "raw-metadata-secret":
				c.Instructions = "password = \"" + strings.Repeat("x", 32) + "\""
			}
			c.SHA256 = continuityDigest(c)
			if name == "bad-record-hash" {
				c.SHA256 = strings.Repeat("a", 64)
			}
			w.Resources.Continuity = &ContinuityPin{Revision: 1, SHA256: c.SHA256, Metadata: &c}
			sealManifestWork(&w)
			out, err := BuildContextManifest(w, in)
			if name != "unknown-literal" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("wanted task-local invalid: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			s := out.Manifest.Sources[2]
			if len(s.Entities) != 1 || s.Entities[0].ID != nil || s.Entities[0].Label != "Alex" || s.ObservedAt != nil || s.EffectiveFrom != nil || s.EffectiveTo != nil || len(out.Manifest.Sources) != 3 {
				t.Fatal("unknown identity/date inferred or URL fetched")
			}
		})
	}
}

func TestManifestRecordedContinuityRevisionsRemainHistorical(t *testing.T) {
	for _, mode := range []string{"project", "channel", "both"} {
		t.Run(mode, func(t *testing.T) {
			s, c, p := productFixture(t)
			record, _, err := s.SaveProjectContinuity(continuityInput(p, c, "record-before-rename"))
			if err != nil {
				t.Fatal(err)
			}
			old, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "before-rename", Text: "Use recorded continuity", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "project" || mode == "both" {
				p, _, err = s.SaveProject(ProjectInput{ID: p.ID, Name: "Renamed project", Goal: p.Goal, ExpectedRevision: p.Revision, ChannelIDs: p.ChannelIDs, ClientNonce: "rename-project"})
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "channel" || mode == "both" {
				c, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: "Renamed channel", ExpectedRevision: c.Revision, BotIDs: c.BotIDs, ClientNonce: "rename-channel"})
				if err != nil {
					t.Fatal(err)
				}
			}
			view, err := s.GetProjectContinuity(p.ID, c.ID)
			if err != nil || !view.Usable || !reflect.DeepEqual(view.Continuity, record) {
				t.Fatal("unchanged recorded provenance lost", err)
			}
			if s.RuntimeAllowed(old.RequestID, old.Epoch) {
				t.Fatal("old project/audience task fence weakened")
			}
			w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "after-rename", Text: "Use recorded continuity", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision, ProjectID: p.ID, ProjectRevision: p.Revision})
			if err != nil {
				t.Fatal(err)
			}
			in := TrustedManifestIntake{Scope: ManifestScope{OwnerUserID: w.OwnerUserID, SpaceID: w.SpaceID, BotID: w.BotID, ProjectID: &p.ID, ChannelID: c.ID, AudienceRevision: c.Revision}, Primary: ManifestPrimary{ID: w.SourceMessageID, SenderID: w.OwnerUserID, Text: w.Text}}
			out, err := BuildContextManifest(w, in)
			if err != nil {
				t.Fatal("current authorized request rejected historical continuity admission revisions", err)
			}
			if out.Manifest.Scope.AudienceRevision != c.Revision || w.Resources.Project.Revision != p.Revision || !reflect.DeepEqual(*w.Resources.Continuity.Metadata, record) || !strings.Contains(out.Manifest.Sources[2].Span.Text, `"projectRevision":1`) || !strings.Contains(out.Manifest.Sources[2].Span.Text, `"channelRevision":1`) {
				t.Fatal("recorded counters rewritten into current selection")
			}
			if target := os.Getenv("NEKO_CONTEXT_MANIFEST_FIXTURE_DIR"); target != "" {
				if err := os.MkdirAll(target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "historical-continuity-"+mode+".json"), out.JSON, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "historical-continuity-"+mode+".sha256"), []byte(out.SHA256), 0600); err != nil {
					t.Fatal(err)
				}
			}
			other, _, err := s.SaveChannel(ChannelInput{Name: "Other scope", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "other-scope-channel"})
			if err != nil {
				t.Fatal(err)
			}
			p, _, err = s.SaveProject(ProjectInput{ID: p.ID, Name: p.Name, Goal: p.Goal, ExpectedRevision: p.Revision, ChannelIDs: []string{other.ID}, ClientNonce: "remove-project-channel"})
			if err != nil {
				t.Fatal(err)
			}
			if s.RuntimeAllowed(w.RequestID, w.Epoch) {
				t.Fatal("scope-removed task still live")
			}
			if _, err = s.GetProjectContinuity(p.ID, c.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("removed scope readable", err)
			}
			seq := s.Snapshot().Sequence
			if _, _, err = s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "removed-scope-intake", Text: "Read continuity", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision}); err == nil || s.Snapshot().Sequence != seq {
				t.Fatal("removed scope accepted/committed")
			}
		})
	}
}
