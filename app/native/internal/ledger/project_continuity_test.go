package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func continuityInput(p Project, c Channel, nonce string) ProjectContinuityInput {
	return ProjectContinuityInput{ProjectID: p.ID, ChannelID: c.ID, ExpectedProjectRevision: p.Revision, ExpectedChannelRevision: c.Revision, ClientNonce: nonce, Instructions: "Compare dated evidence; do not merge people by name.", Decisions: []ProjectDecision{{ID: "choice-1", Text: "The price is disputed", Status: "contested", EffectiveDate: "2026-10-11", EntityID: "vendor-a", ReferenceIDs: []string{"quote-old", "quote-new"}}}, References: []ProjectReference{{ID: "quote-old", Title: "Alex quote", SourceDate: "2026-10-01", EntityID: "person-a", Version: "1", URL: "https://example.invalid/quote-old"}, {ID: "quote-new", Title: "Alex quote", SourceDate: "2026-10-10", EntityID: "person-b", Version: "2", URL: "https://example.invalid/quote-new"}}}
}

func TestProjectContinuityEmptyRecordReplayAndRawSecretRejection(t *testing.T) {
	t.Run("empty-replay", func(t *testing.T) {
		s, c, p := productFixture(t)
		in := ProjectContinuityInput{ProjectID: p.ID, ChannelID: c.ID, ExpectedProjectRevision: 1, ExpectedChannelRevision: 1, ClientNonce: "continuity-empty-nonce"}
		if _, _, err := s.SaveProjectContinuity(in); err != nil {
			t.Fatal(err)
		}
		w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "New request", Lane: "foreground", ClientNonce: "continuity-empty-work"}, SubmissionScope{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID})
		if err != nil {
			t.Fatal(err)
		}
		dir := s.dir
		s.Close()
		s, err = Open(dir)
		if err != nil {
			t.Fatal("valid empty record made journal corrupt", err)
		}
		defer s.Close()
		saved, _ := s.Get(w.RequestID)
		if saved.Resources.Continuity.Revision != 1 {
			t.Fatal("empty metadata revision lost")
		}
	})
	t.Run("quoted-secret", func(t *testing.T) {
		s, c, p := productFixture(t)
		in := continuityInput(p, c, "continuity-secret-nonce")
		in.Instructions = "password = \"" + strings.Repeat("x", 24) + "\""
		before := s.Snapshot().Sequence
		if _, _, err := s.SaveProjectContinuity(in); !errors.Is(err, ErrInvalid) {
			t.Fatal("raw credential assignment accepted", err)
		}
		if s.Snapshot().Sequence != before {
			t.Fatal("secret committed")
		}
	})
}

func TestProjectContinuityCommitCrashGapLegacyPinsAndGroupFence(t *testing.T) {
	s, c, p := productFixture(t)
	scope := SubmissionScope{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID}
	legacy, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "Continue", Lane: "foreground", ClientNonce: "legacy-continuity-work"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("legacy nil pin invalid", err)
	}
	if !s.RuntimeAllowed(legacy.RequestID, legacy.Epoch) {
		t.Fatal("legacy nil pin lost before metadata")
	}
	in := continuityInput(p, c, "continuity-crash-gap")
	record, _, err := s.SaveProjectContinuity(in)
	if err != nil {
		t.Fatal(err)
	}
	group, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: 1, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare dated quotes", MaxStages: 3, ClientNonce: "continuity-group-nonce", ProjectID: p.ID, ProjectRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceCollaborations(); err != nil {
		t.Fatal(err)
	}
	group = s.collaborations[group.ID]
	if len(group.RequestIDs) != 1 {
		t.Fatal("no first stage")
	}
	firstStage, _ := s.Get(group.RequestIDs[0])
	frozen := copyResources(firstStage.Resources)
	in.ExpectedRevision = record.Revision
	in.ClientNonce = "continuity-commit-only"
	in.Instructions = "Revised owner instructions"
	changed := inputContinuity(in)
	changed.OwnerUserID = "local-owner"
	changed.SpaceID = "local-space"
	changed.AuthorType = "owner"
	changed.RecordedAt = s.clock().Format("2006-01-02T15:04:05.999999999Z07:00")
	changed.SHA256 = continuityDigest(changed)
	s.mu.Lock()
	e := Event{Type: "project.continuity_saved", ProjectContinuity: &changed, Nonce: in.ClientNonce}
	if !s.validContinuityEvent(e) {
		t.Fatal("fixture event invalid")
	}
	err = s.commit(e)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for attempt := 0; attempt < 3; attempt++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal("metadata crash gap caused global corruption", err)
		}
		if s.RuntimeAllowed(firstStage.RequestID, firstStage.Epoch) {
			t.Fatal("dispatch before reconciliation escaped fence")
		}
		if err = s.RecoverInterrupted(); err != nil {
			t.Fatal(err)
		}
		if err = s.AdvanceCollaborations(); err != nil {
			t.Fatal(err)
		}
		g := s.collaborations[group.ID]
		stage, _ := s.Get(firstStage.RequestID)
		if len(g.RequestIDs) != 1 || g.Status != "suspended" || stage.Status != "suspended" || !reflect.DeepEqual(stage.Resources, frozen) || !reflect.DeepEqual(g.Resources, group.Resources) {
			t.Fatal("group repinned or advanced", g.Status)
		}
		old, _ := s.Get(legacy.RequestID)
		if old.Status != "suspended" {
			t.Fatal("legacy nil pin reused new continuity")
		}
		s.Close()
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, err = s.Accept(Input{BotID: "bot-davin", Text: "Unrelated private request", Lane: "foreground", ClientNonce: "continuity-unrelated-work"}); err != nil {
		t.Fatal("unrelated intake blocked", err)
	}
}

func TestProjectContinuityFileReferencesExactScopeAndNoRegrant(t *testing.T) {
	s, c, p := productFixture(t)
	file, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ClientNonce: "continuity-file-upload", Filename: "evidence.md", Content: []byte("Evidence dated 2026-10-01")})
	if err != nil {
		t.Fatal(err)
	}
	in := continuityInput(p, c, "continuity-file-first")
	in.References[0].URL = ""
	in.References[0].File = &ProjectFileReference{FileID: file.ID, SHA256: file.SHA256}
	if _, _, err = s.SaveProjectContinuity(in); err != nil {
		t.Fatal(err)
	}
	scope := SubmissionScope{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "Evaluate context", Lane: "foreground", ClientNonce: "continuity-file-work"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Resources.Files) != 0 {
		t.Fatal("metadata reference expanded selected read boundary")
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: file.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "continuity-file-revoke"}); err != nil {
		t.Fatal(err)
	}
	if err = s.SetFileAccess(FileAccessInput{FileID: file.ID, ExpectedRevision: 1, Granted: true, ClientNonce: "continuity-file-regrant"}); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	view, _ := s.GetProjectContinuity(p.ID, c.ID)
	if current.Status != "suspended" || s.RuntimeAllowed(current.RequestID, current.Epoch) || view.Usable {
		t.Fatal("regrant restored old authority")
	}
	if _, _, err = s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "New", Lane: "foreground", ClientNonce: "continuity-invalid-reference"}, scope); !errors.Is(err, ErrConflict) {
		t.Fatal("stale metadata ref entered fresh context", err)
	}
	in.ExpectedRevision = 1
	in.ClientNonce = "continuity-file-reconfirm"
	in.References[0].File.AccessRevision = 2
	if _, _, err = s.SaveProjectContinuity(in); err != nil {
		t.Fatal(err)
	}
	fresh, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "New", Lane: "foreground", ClientNonce: "continuity-reconfirmed-work"}, scope)
	if err != nil || fresh.Resources.Continuity.Revision != 2 {
		t.Fatal(err)
	}
	cross := in
	cross.ExpectedRevision = 2
	cross.ClientNonce = "continuity-cross-ref"
	cross.References = append([]ProjectReference{}, in.References...)
	copyFile := *cross.References[0].File
	cross.References[0].File = &copyFile
	cross.References[0].File.SHA256 = strings.Repeat("a", 64)
	before := s.Snapshot().Sequence
	if _, _, err = s.SaveProjectContinuity(cross); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong hash accepted", err)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("invalid reference committed")
	}
	other, _, err := s.SaveProject(ProjectInput{Name: "Other", ChannelIDs: []string{c.ID}, ClientNonce: "continuity-other-project"})
	if err != nil {
		t.Fatal(err)
	}
	cross.ProjectID = other.ID
	cross.References[0].File.SHA256 = file.SHA256
	cross.ExpectedRevision = 0
	cross.ClientNonce = "continuity-cross-project"
	if _, _, err = s.SaveProjectContinuity(cross); !errors.Is(err, ErrInvalid) {
		t.Fatal("cross project reference accepted", err)
	}
}

func TestProjectContinuityChannelIsolationOutputAndPublicationFences(t *testing.T) {
	s, c, p := productFixture(t)
	other, _, err := s.SaveChannel(ChannelInput{Name: "Private parallel channel", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "continuity-other-channel"})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err = s.SaveProject(ProjectInput{ID: p.ID, Name: p.Name, Goal: p.Goal, ExpectedRevision: 1, ChannelIDs: []string{c.ID, other.ID}, ClientNonce: "continuity-two-channel-project"})
	if err != nil {
		t.Fatal(err)
	}
	metadata := continuityInput(p, c, "continuity-channel-record")
	if _, _, err = s.SaveProjectContinuity(metadata); err != nil {
		t.Fatal(err)
	}
	scope := SubmissionScope{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: c.ID}
	prior, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "Produce report", Lane: "foreground", ClientNonce: "continuity-completed-source"}, scope)
	if err != nil {
		t.Fatal(err)
	}
	prior = startPermissionWork(t, s, prior)
	if err = s.AppendText(prior.RequestID, prior.Epoch, "Restricted prior conclusion"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteTool(prior.RequestID, prior.Epoch, invocation("Restricted artifact")); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(prior.RequestID, prior.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	prior, _ = s.Get(prior.RequestID)
	consumer, _, err := s.AcceptProduct(Input{BotID: "bot-davin", Text: "Review conclusion", Lane: "background", ClientNonce: "continuity-derived-consumer"}, scope)
	if err != nil || len(consumer.Context.Sources) != 1 {
		t.Fatal("shared completed source absent", err)
	}
	consumer = startPermissionWork(t, s, consumer)
	otherScope := scope
	otherScope.ChannelID = other.ID
	isolated, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: "Continue in separate lane", Lane: "foreground", ClientNonce: "continuity-isolated-work"}, otherScope)
	if err != nil {
		t.Fatal(err)
	}
	if isolated.Resources.Continuity != nil || strings.Contains(isolated.RuntimePrompt(), metadata.Instructions) || len(isolated.Context.Sources) != 0 {
		t.Fatal("metadata/history crossed channel")
	}
	file, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: p.Revision, ChannelID: other.ID, ClientNonce: "continuity-other-channel-file", Filename: "private.md", Content: []byte("Private source")})
	if err != nil {
		t.Fatal(err)
	}
	cross := continuityInput(p, c, "continuity-cross-channel-source")
	cross.ExpectedRevision = 1
	cross.References[0].URL = ""
	cross.References[0].File = &ProjectFileReference{FileID: file.ID, SHA256: file.SHA256}
	if _, _, err = s.SaveProjectContinuity(cross); !errors.Is(err, ErrInvalid) {
		t.Fatal("other channel reference accepted", err)
	}
	metadata.ExpectedRevision = 1
	metadata.ClientNonce = "continuity-channel-change"
	metadata.Instructions = "Updated scoped requirement"
	if _, _, err = s.SaveProjectContinuity(metadata); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(consumer.RequestID)
	if current.Status != "suspending" || current.Epoch != consumer.Epoch+1 || s.RuntimeAllowed(current.RequestID, current.Epoch) {
		t.Fatal("active derived task unfenced")
	}
	if err = s.AppendText(consumer.RequestID, consumer.Epoch, "late output"); !errors.Is(err, ErrConflict) {
		t.Fatal("late text accepted", err)
	}
	if _, err = s.ExecuteTool(consumer.RequestID, consumer.Epoch, invocation("late file")); !errors.Is(err, ErrConflict) {
		t.Fatal("late effect accepted", err)
	}
	if err = s.Finish(consumer.RequestID, consumer.Epoch, "completed", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("late completion accepted", err)
	}
	if _, _, err = s.ReadChannelArtifact(c.ID, prior.Artifacts[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("old continuity artifact published", err)
	}
	if _, bytes, err := s.ReadArtifact(prior.Artifacts[0].ID); !errors.Is(err, ErrNotFound) || len(bytes) != 0 {
		t.Fatal("unscoped continuity artifact published", err)
	}
	if !s.RuntimeAllowed(isolated.RequestID, isolated.Epoch) {
		t.Fatal("unrelated channel fenced")
	}
	fresh, _, err := s.AcceptProduct(Input{BotID: "bot-davin", Text: "Fresh context", Lane: "foreground", ClientNonce: "continuity-no-old-result"}, scope)
	if err != nil || len(fresh.Context.Sources) != 0 || strings.Contains(fresh.RuntimePrompt(), prior.Reply) {
		t.Fatal("revoked derived result entered context", err)
	}
}

func TestProjectContinuityConcurrentCASAndCopyIsolation(t *testing.T) {
	s, c, p := productFixture(t)
	in := continuityInput(p, c, "continuity-concurrent-first")
	if _, _, err := s.SaveProjectContinuity(in); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			candidate := continuityInput(p, c, "continuity-cas-writer-"+string(rune('a'+i)))
			candidate.ExpectedRevision = 1
			_, _, err := s.SaveProjectContinuity(candidate)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("CAS allowed multiple writes", success)
	}
	view, err := s.GetProjectContinuity(p.ID, c.ID)
	if err != nil || view.Continuity.Revision != 2 {
		t.Fatal(err)
	}
	view.Continuity.Decisions[0].ReferenceIDs[0] = "caller-mutated"
	view.Continuity.References[0].Title = "caller-mutated"
	view, err = s.GetProjectContinuity(p.ID, c.ID)
	if err != nil || view.Continuity.Decisions[0].ReferenceIDs[0] == "caller-mutated" || view.Continuity.References[0].Title == "caller-mutated" {
		t.Fatal("mutable getter alias escaped")
	}
}

func TestProjectContinuityBoundsAndStrictReplayPayload(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProjectContinuityInput)
	}{
		{"invalid-date", func(in *ProjectContinuityInput) { in.References[0].SourceDate = "2026-02-30" }},
		{"unknown-status", func(in *ProjectContinuityInput) { in.Decisions[0].Status = "inferred" }},
		{"missing-source", func(in *ProjectContinuityInput) { in.Decisions[0].ReferenceIDs = []string{"missing"} }},
		{"duplicate-reference", func(in *ProjectContinuityInput) { in.References = append(in.References, in.References[0]) }},
		{"url-secret-userinfo", func(in *ProjectContinuityInput) { in.References[0].URL = "https://user:fixture@example.invalid/source" }},
		{"url-token-query", func(in *ProjectContinuityInput) {
			in.References[0].URL = "https://example.invalid/source?token=fixture"
		}},
		{"encoded-bound", func(in *ProjectContinuityInput) {
			in.Instructions = strings.Repeat("\\", 2000)
			in.Decisions = append(in.Decisions, ProjectDecision{ID: "large", Text: strings.Repeat("\\", 1000), Status: "confirmed"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, c, p := productFixture(t)
			in := continuityInput(p, c, "continuity-negative-case")
			tc.mutate(&in)
			before := s.Snapshot().Sequence
			if _, _, err := s.SaveProjectContinuity(in); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid metadata", err)
			}
			if before != s.Snapshot().Sequence {
				t.Fatal("invalid data committed")
			}
		})
	}
	t.Run("shared-prompt-bound", func(t *testing.T) {
		s, c, p := productFixture(t)
		in := continuityInput(p, c, "continuity-bound-record")
		in.Instructions = strings.Repeat("\\", 1500)
		if _, _, err := s.SaveProjectContinuity(in); err != nil {
			t.Fatal(err)
		}
		before := s.Snapshot().Sequence
		if _, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", Text: strings.Repeat("\\", 30000), Lane: "foreground", ClientNonce: "continuity-oversized-work"}, SubmissionScope{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID}); !errors.Is(err, ErrInvalid) {
			t.Fatal("encoded prompt bound bypassed", err)
		}
		if before != s.Snapshot().Sequence {
			t.Fatal("oversized context committed")
		}
	})
	t.Run("mixed-payload", func(t *testing.T) {
		s, c, p := productFixture(t)
		in := continuityInput(p, c, "continuity-replay-event")
		if _, _, err := s.SaveProjectContinuity(in); err != nil {
			t.Fatal(err)
		}
		dir := s.dir
		s.Close()
		path := filepath.Join(dir, "transactions.jsonl")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		previous := ""
		for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
			var r record
			if err = json.Unmarshal(line, &r); err != nil {
				t.Fatal(err)
			}
			r.Previous = previous
			if r.Event.ProjectContinuity != nil {
				r.Event.CompletionVersion = 1
			}
			r.Digest, err = digestRecord(r)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(r)
			out.Write(b)
			out.WriteByte('\n')
			previous = r.Digest
		}
		if err = os.WriteFile(path, out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(dir)
		if reopened != nil {
			reopened.Close()
		}
		if !errors.Is(err, ErrCorrupt) {
			t.Fatal("mixed typed payload replayed", err)
		}
	})
}

func TestProjectContinuityIntakeCASOriginalNonceAndFrozenGroup(t *testing.T) {
	s, c, p := productFixture(t)
	zero := uint64(0)
	one := uint64(1)
	scope := SubmissionScope{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, ExpectedContinuityRevision: &zero}
	in := Input{BotID: "bot-meiiie", Text: "Continue with exact metadata", Lane: "foreground", ClientNonce: "continuity-cas-original"}
	old, _, err := s.AcceptProduct(in, scope)
	if err != nil {
		t.Fatal(err)
	}
	metadata := continuityInput(p, c, "continuity-intake-metadata")
	if _, _, err = s.SaveProjectContinuity(metadata); err != nil {
		t.Fatal(err)
	}
	duplicate, dup, err := s.AcceptProduct(in, scope)
	if err != nil || !dup || duplicate.RequestID != old.RequestID || duplicate.Resources.Continuity != nil || duplicate.Status != "suspended" {
		t.Fatal("nonce repinned or denied original", err)
	}
	scope.ExpectedContinuityRevision = &one
	if _, _, err = s.AcceptProduct(in, scope); !errors.Is(err, ErrConflict) {
		t.Fatal("same nonce changed metadata selection", err)
	}
	scope.ExpectedContinuityRevision = &zero
	in.ClientNonce = "continuity-cas-stale"
	before := s.Snapshot().Sequence
	if _, _, err = s.AcceptProduct(in, scope); !errors.Is(err, ErrConflict) {
		t.Fatal("stale metadata intake", err)
	}
	if before != s.Snapshot().Sequence {
		t.Fatal("stale intake committed")
	}
	groupInput := CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: 1, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare quotes", MaxStages: 3, ClientNonce: "continuity-cas-group", ProjectID: p.ID, ProjectRevision: 1, ExpectedContinuityRevision: &zero}
	if _, _, err = s.CreateCollaboration(groupInput); !errors.Is(err, ErrConflict) {
		t.Fatal("stale group metadata intake", err)
	}
	groupInput.ExpectedContinuityRevision = &one
	g, _, err := s.CreateCollaboration(groupInput)
	if err != nil {
		t.Fatal(err)
	}
	one = 99
	saved := s.collaborations[g.ID]
	if *saved.ExpectedContinuityRevision != 1 || saved.Resources.Continuity.Revision != 1 {
		t.Fatal("caller mutated durable expected revision")
	}
}

func TestProjectContinuityDurableNonceCASAndVersionedProvenance(t *testing.T) {
	s, c, p := productFixture(t)
	in := continuityInput(p, c, "continuity-first-nonce")
	first, dup, err := s.SaveProjectContinuity(in)
	if err != nil || dup || first.Revision != 1 || first.AuthorType != "owner" || first.OwnerUserID != "local-owner" || first.RecordedAt == "" || first.SHA256 == "" {
		t.Fatal("owner provenance", err)
	}
	again, dup, err := s.SaveProjectContinuity(in)
	if err != nil || !dup || !reflect.DeepEqual(first, again) {
		t.Fatal("lost ack", err)
	}
	before := s.Snapshot().Sequence
	changed := in
	changed.Instructions = "changed"
	if _, _, err = s.SaveProjectContinuity(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce payload changed", err)
	}
	changed.ClientNonce = "continuity-stale-nonce"
	if _, _, err = s.SaveProjectContinuity(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("stale record CAS", err)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("rejection committed")
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	view, err := s.GetProjectContinuity(p.ID, c.ID)
	if err != nil || !view.Usable || !reflect.DeepEqual(view.Continuity, first) {
		t.Fatal("durable record", err)
	}
	if view.Continuity.References[0].SourceDate != "2026-10-01" || view.Continuity.References[1].EntityID != "person-b" || view.Continuity.Decisions[0].Status != "contested" {
		t.Fatal("date/entity/status merged")
	}
	changed.ExpectedRevision = 1
	second, _, err := s.SaveProjectContinuity(changed)
	if err != nil || second.Revision != 2 {
		t.Fatal(err)
	}
	again, dup, err = s.SaveProjectContinuity(in)
	if err != nil || !dup || again.Revision != 1 {
		t.Fatal("nonce rebound to latest", err)
	}
}

func TestProjectContinuityPinnedScopedRedactedAndFenced(t *testing.T) {
	s, c, p := productFixture(t)
	in := continuityInput(p, c, "continuity-scope-nonce")
	first, _, err := s.SaveProjectContinuity(in)
	if err != nil {
		t.Fatal(err)
	}
	scope := SubmissionScope{ChannelID: c.ID, ProjectID: p.ID, ProjectRevision: p.Revision}
	w, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "continuity-work-nonce", Text: "Compare evidence", Lane: "foreground"}, scope)
	if err != nil || w.Resources.Continuity == nil || w.Resources.Continuity.SHA256 != first.SHA256 || !strings.Contains(w.RuntimePrompt(), in.Instructions) {
		t.Fatal("pin absent", err)
	}
	snapshot, _ := json.Marshal(s.PublicSnapshot())
	events, _ := s.Events(0)
	published, _ := json.Marshal(events)
	if strings.Contains(string(snapshot), in.Instructions) || strings.Contains(string(published), in.Instructions) || strings.Contains(string(published), "person-b") {
		t.Fatal("unscoped projection disclosed metadata")
	}
	if _, err = s.GetProjectContinuity(p.ID, "conversation-bot-davin"); !errors.Is(err, ErrNotFound) {
		t.Fatal("wrong channel getter", err)
	}
	frozen := copyResources(w.Resources)
	in.ExpectedRevision = 1
	in.ClientNonce = "continuity-scope-update"
	in.Instructions = "New owner choice"
	if _, _, err = s.SaveProjectContinuity(in); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	if current.Status != "suspended" || s.RuntimeAllowed(current.RequestID, current.Epoch) || !reflect.DeepEqual(frozen, current.Resources) {
		t.Fatal("old task repinned or unfenced")
	}
	before := s.Snapshot().Sequence
	if _, err = s.RefreshContext(RefreshInput{RequestID: current.RequestID, ExpectedEpoch: current.Epoch}); !errors.Is(err, ErrConflict) {
		t.Fatal("refresh silently retained stale continuity", err)
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("stale refresh committed")
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("journal corrupted after metadata update", err)
	}
	defer s.Close()
	if err = s.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	fresh, _, err := s.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "continuity-fresh-work", Text: "Compare evidence", Lane: "foreground"}, scope)
	if err != nil || fresh.Resources.Continuity.Revision != 2 || len(fresh.Context.Sources) != 0 {
		t.Fatal("fresh pin reused fenced history", err)
	}
}
