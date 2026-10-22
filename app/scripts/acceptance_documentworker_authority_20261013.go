package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"nekoaurora.local/native/internal/documents"
)

type independentExtractor struct {
	extract func(string, []byte) (documents.Result, error)
}

func (worker *independentExtractor) Extract(format string, raw []byte) (documents.Result, error) {
	return worker.extract(format, raw)
}

func independentText(text string) documents.Result {
	sum := sha256.Sum256([]byte(text))
	return documents.Result{Text: text, TextBytes: len([]byte(text)), TextSHA256: hex.EncodeToString(sum[:])}
}

func independentDocumentStore(t *testing.T, worker *independentExtractor) (*Store, Project, string) {
	t.Helper()
	store, err := OpenWithDocumentWorker(t.TempDir(), worker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	channel, _, err := store.SaveChannel(ChannelInput{Name: "independent worker scope", BotIDs: []string{"bot-meiiie"}, ClientNonce: "independent-worker-channel"})
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := store.SaveProject(ProjectInput{Name: "independent worker project", ChannelIDs: []string{channel.ID}, ClientNonce: "independent-worker-project"})
	if err != nil {
		t.Fatal(err)
	}
	return store, project, channel.ID
}

func TestIndependentDocumentWorkerMandatoryConstructor(t *testing.T) {
	for name, worker := range map[string]DocumentExtractor{"nil-interface": nil, "typed-nil-worker": (*independentExtractor)(nil)} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "must-not-open")
			store, err := OpenWithDocumentWorker(dir, worker)
			if store != nil || err != ErrUnavailable {
				t.Fatal("mandatory worker constructor accepted absent extractor")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("rejected constructor created Store state")
			}
		})
	}
}

func TestIndependentDocumentWorkerParentRecomputesIdentity(t *testing.T) {
	correct := independentText("independent worker body")
	cases := map[string]documents.Result{
		"reported-foreign-hash":                {Text: correct.Text, TextBytes: correct.TextBytes, TextSHA256: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
		"reported-foreign-length":              {Text: correct.Text, TextBytes: correct.TextBytes + 1, TextSHA256: correct.TextSHA256},
		"same-length-changed-body-old-witness": {Text: "Independent worker body", TextBytes: correct.TextBytes, TextSHA256: correct.TextSHA256},
		"valid-digest-invalid-UTF8":            independentText(string([]byte{0xc0, 0xaf})),
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			worker := &independentExtractor{extract: func(string, []byte) (documents.Result, error) { return result, nil }}
			store, project, channel := independentDocumentStore(t, worker)
			if _, _, _, err := store.extractFileText("fixture.docx", []byte("published synthetic Office raw")); err == nil {
				t.Fatal("parent trusted supplied text/hash/size instead of recomputing identity")
			}
			in := UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel, Filename: "fixture.docx", Content: []byte("published synthetic Office raw"), ClientNonce: "independent-upload-lying-worker"}
			file, duplicate, err := store.UploadFile(in)
			if err != nil || duplicate || file.Status != "quarantined_unsupported" || file.Excerpt != "" || file.ExtractionVersion != 0 {
				t.Fatal("invalid worker result became ready or changed acknowledged quarantine policy")
			}
			before := store.Snapshot().Sequence
			worker.extract = func(string, []byte) (documents.Result, error) { return correct, nil }
			retry, duplicate, err := store.UploadFile(in)
			if err != nil || !duplicate || !reflect.DeepEqual(retry, file) || store.Snapshot().Sequence != before {
				t.Fatal("later worker success changed original quarantined upload identity or journal")
			}
		})
	}
}

func TestIndependentDocumentWorkerFailureNeverFallsBack(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "independent-valid.docx"))
	if err != nil {
		t.Fatal("retained synthetic DOCX fixture not supplied")
	}
	if result, err := documents.ExtractDOCX(raw); err != nil || result.TextBytes == 0 {
		t.Fatal("retained synthetic fixture is not a valid direct-parser control")
	}
	refusal := errors.New("authored worker refusal")
	worker := &independentExtractor{extract: func(string, []byte) (documents.Result, error) { return documents.Result{}, refusal }}
	store := &Store{documentExtractor: worker}
	result, _, _, err := store.extractFileText("fixture.docx", raw)
	if !errors.Is(err, refusal) || result.Text != "" || result.TextBytes != 0 {
		t.Fatal("configured worker failure fell back to the valid direct-parser control")
	}
}

func TestIndependentDocumentWorkerPreviewCannotTrustOldReportedWitness(t *testing.T) {
	correct := independentText("independent worker body")
	worker := &independentExtractor{extract: func(string, []byte) (documents.Result, error) { return correct, nil }}
	store, project, channel := independentDocumentStore(t, worker)
	file, _, err := store.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel, Filename: "fixture.docx", Content: []byte("published synthetic Office raw"), ClientNonce: "independent-preview-original"})
	if err != nil || file.Status != "ready_text" {
		t.Fatal("authored valid extractor fixture unavailable")
	}
	worker.extract = func(string, []byte) (documents.Result, error) {
		changed := correct
		changed.Text = "Independent worker body"
		return changed, nil
	}
	before := store.Snapshot().Sequence
	preview, err := store.PreviewProjectFile(project.ID, channel, file.ID, project.Revision, file.AccessRevision)
	if err == nil || preview.Text != "" || store.Snapshot().Sequence != before || !reflect.DeepEqual(publicFile(store.uploads[file.ID]), file) {
		t.Fatal("preview accepted same-size changed text with old reported witness or mutated original receipt")
	}
}

func TestIndependentDocumentWorkerHeldPreviewRevocation(t *testing.T) {
	correct := independentText("independent worker body")
	worker := &independentExtractor{extract: func(string, []byte) (documents.Result, error) { return correct, nil }}
	store, project, channel := independentDocumentStore(t, worker)
	file, _, err := store.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel, Filename: "fixture.docx", Content: []byte("published synthetic Office raw"), ClientNonce: "independent-revoke-original"})
	if err != nil || file.Status != "ready_text" {
		t.Fatal("authored extraction setup unavailable")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	worker.extract = func(string, []byte) (documents.Result, error) { close(entered); <-release; return correct, nil }
	finished := make(chan error, 1)
	go func() {
		preview, err := store.PreviewProjectFile(project.ID, channel, file.ID, project.Revision, file.AccessRevision)
		if preview.Text != "" {
			finished <- errors.New("revoked preview returned text")
			return
		}
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("preview did not enter held extractor")
	}
	accessDone := make(chan error, 1)
	go func() {
		accessDone <- store.SetFileAccess(FileAccessInput{FileID: file.ID, ExpectedRevision: file.AccessRevision, Granted: false, ClientNonce: "independent-revoke-during-extract"})
	}()
	select {
	case err := <-accessDone:
		if err != nil {
			close(release)
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("held extractor retained Store lock")
	}
	afterRevoke := store.Snapshot().Sequence
	close(release)
	select {
	case err := <-finished:
		if err != ErrConflict {
			t.Fatal("revoked preview did not fail closed at final descriptor check")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("released preview did not complete")
	}
	if store.Snapshot().Sequence != afterRevoke || store.uploads[file.ID].Granted {
		t.Fatal("late extraction changed revoked authority or journal")
	}
}

func TestIndependentDocumentWorkerHeldUploadProjectRevision(t *testing.T) {
	correct := independentText("independent worker body")
	entered, release := make(chan struct{}), make(chan struct{})
	worker := &independentExtractor{extract: func(string, []byte) (documents.Result, error) { close(entered); <-release; return correct, nil }}
	store, project, channel := independentDocumentStore(t, worker)
	finished := make(chan error, 1)
	go func() {
		file, _, err := store.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel, Filename: "fixture.docx", Content: []byte("published synthetic Office raw"), ClientNonce: "independent-project-race-upload"})
		if file.ID != "" {
			finished <- errors.New("stale upload acquired identity")
			return
		}
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("upload did not enter held extractor")
	}
	projectDone := make(chan error, 1)
	go func() {
		_, _, err := store.SaveProject(ProjectInput{ID: project.ID, Name: "changed independent project", ExpectedRevision: project.Revision, ChannelIDs: []string{channel}, ClientNonce: "independent-project-race-change"})
		projectDone <- err
	}()
	select {
	case err := <-projectDone:
		if err != nil {
			close(release)
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("upload extraction retained Store lock")
	}
	afterProject := store.Snapshot().Sequence
	close(release)
	select {
	case err := <-finished:
		if err != ErrConflict {
			t.Fatal("old project revision upload did not fail at final authority check")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("released upload did not complete")
	}
	if store.Snapshot().Sequence != afterProject || len(store.uploads) != 0 {
		t.Fatal("stale upload mutated receipts or journal after scope change")
	}
}
