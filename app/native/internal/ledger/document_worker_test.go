package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nekobot.local/native/internal/documents"
	"nekobot.local/native/internal/documentworker"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--document-worker-v1" {
		os.Exit(documentworker.Run(os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

type checkedDocumentExtractor struct {
	calls   int
	format  string
	failure bool
	held    chan struct{}
	started chan struct{}
}

func (extractor *checkedDocumentExtractor) Extract(format string, raw []byte) (documents.Result, error) {
	extractor.calls++
	extractor.format = format
	if extractor.started != nil {
		close(extractor.started)
		<-extractor.held
	}
	if extractor.failure {
		return documents.Result{}, ErrUnavailable
	}
	switch format {
	case "xlsx":
		return documents.ExtractXLSX(raw)
	case "pptx":
		return documents.ExtractPPTX(raw)
	case "docx":
		return documents.ExtractDOCX(raw)
	}
	return documents.Result{}, ErrUnsupported
}

func documentWorkerFixture(t *testing.T, extractor DocumentExtractor) (*Store, Channel, Project) {
	t.Helper()
	old, c, p := productFixture(t)
	dir := old.dir
	old.Close()
	store, err := OpenWithDocumentWorker(dir, extractor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, c, p
}

func TestDocumentWorkerConstructorRequiresExtractor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-created")
	if store, err := OpenWithDocumentWorker(dir, nil); err == nil || store != nil {
		t.Fatal("nil worker accepted")
	}
	var extractor *checkedDocumentExtractor
	if store, err := OpenWithDocumentWorker(dir, extractor); err == nil || store != nil {
		t.Fatal("typed nil worker accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("invalid constructor created state")
	}
}

func TestDocumentWorkerUploadPreviewToolPreparation(t *testing.T) {
	for _, ext := range []string{".xlsx", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			extractor := &checkedDocumentExtractor{}
			store, channel, project := documentWorkerFixture(t, extractor)
			raw := officeUploadFixture(t, ext, "Worker text tiếng Việt")
			file, _, err := store.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "worker-configured-upload", Filename: "quote" + ext, Content: raw})
			if err != nil || file.Status != "ready_text" || extractor.calls != 1 || extractor.format != strings.TrimPrefix(ext, ".") {
				t.Fatal("configured upload did not extract", err)
			}
			preview, err := store.PreviewProjectFile(project.ID, channel.ID, file.ID, project.Revision, 0)
			if err != nil || extractor.calls != 2 || preview.TextSHA256 != file.TextSHA256 {
				t.Fatal("configured preview did not reparse", err)
			}
			work, _, err := store.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "worker-tool-source", Text: "Inspect quote", Lane: "background"}, SubmissionScope{ChannelID: channel.ID, ProjectID: project.ID, ProjectRevision: project.Revision, FileIDs: []string{file.ID}})
			if err != nil {
				t.Fatal(err)
			}
			store.mu.Lock()
			active := store.work[work.RequestID]
			active.Status = "running"
			store.work[work.RequestID] = active
			store.mu.Unlock()
			source, text, err := store.preparePinnedFileText(work.RequestID, work.Epoch, file.ID)
			if err != nil || extractor.calls != 3 || source.TextSHA256 != file.TextSHA256 || !strings.Contains(string(text), "Worker text") {
				t.Fatal("configured tool preparation did not reparse", err)
			}
			extractor.failure = true
			if _, err = store.PreviewProjectFile(project.ID, channel.ID, file.ID, project.Revision, 0); err == nil {
				t.Fatal("preview fell back to direct parser")
			}
			if _, _, err = store.preparePinnedFileText(work.RequestID, work.Epoch, file.ID); err == nil {
				t.Fatal("tool preparation fell back to direct parser")
			}
		})
	}
}

func TestDocumentWorkerFailureQuarantineDedup(t *testing.T) {
	extractor := &checkedDocumentExtractor{failure: true}
	store, channel, project := documentWorkerFixture(t, extractor)
	raw := officeUploadFixture(t, ".xlsx", "Valid ordinary text")
	input := UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "worker-failed-upload", Filename: "quote.xlsx", Content: raw}
	file, duplicate, err := store.UploadFile(input)
	if err != nil || duplicate || file.Status != "quarantined_unsupported" || file.ExtractionVersion != 0 || extractor.calls != 1 {
		t.Fatal("failed extraction promoted or lost raw upload", err)
	}
	extractor.failure = false
	again, duplicate, err := store.UploadFile(input)
	if err != nil || !duplicate || again.ID != file.ID || again.SHA256 != file.SHA256 || again.Status != file.Status {
		t.Fatal("nonce retry upgraded quarantine", err)
	}
	input.ClientNonce = "worker-ready-upload"
	ready, _, err := store.UploadFile(input)
	if err != nil || ready.Status != "ready_text" {
		t.Fatal("positive extraction", err)
	}
	extractor.failure = true
	again, duplicate, err = store.UploadFile(input)
	if err != nil || !duplicate || again.ID != ready.ID || again.Status != ready.Status || again.TextSHA256 != ready.TextSHA256 {
		t.Fatal("failed retry changed accepted receipt", err)
	}
	_, bytes, err := store.ReadProjectFile(project.ID, channel.ID, file.ID)
	if err != nil || string(bytes) != string(raw) {
		t.Fatal("quarantine lost original bytes", err)
	}
}

func TestDocumentWorkerPreviewRevokeWhileParsing(t *testing.T) {
	extractor := &checkedDocumentExtractor{}
	store, channel, project := documentWorkerFixture(t, extractor)
	file, _, err := store.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "worker-race-upload", Filename: "quote.xlsx", Content: officeUploadFixture(t, ".xlsx", "Ordinary text")})
	if err != nil {
		t.Fatal(err)
	}
	extractor.held = make(chan struct{})
	extractor.started = make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := store.PreviewProjectFile(project.ID, channel.ID, file.ID, project.Revision, 0)
		done <- err
	}()
	select {
	case <-extractor.started:
	case <-time.After(time.Second):
		t.Fatal("extractor not reached")
	}
	if err = store.SetFileAccess(FileAccessInput{FileID: file.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "worker-race-revoke"}); err != nil {
		t.Fatal(err)
	}
	close(extractor.held)
	select {
	case err = <-done:
		if !errors.Is(err, ErrConflict) {
			t.Fatal("late parser bypassed final authority", err)
		}
	case <-time.After(time.Second):
		t.Fatal("preview blocked")
	}
}

func TestDocumentWorkerSelfExecutableLedgerRoutes(t *testing.T) {
	client, err := documentworker.NewSelf()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	store, channel, project := documentWorkerFixture(t, client)
	raw := officeUploadFixture(t, ".xlsx", "Self executable literal text")
	input := UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "worker-self-upload", Filename: "quote.xlsx", Content: raw}
	file, _, err := store.UploadFile(input)
	if err != nil || file.Status != "ready_text" {
		t.Fatal("real worker upload", err)
	}
	preview, err := store.PreviewProjectFile(project.ID, channel.ID, file.ID, project.Revision, 0)
	if err != nil || preview.TextSHA256 != file.TextSHA256 {
		t.Fatal("real worker preview", err)
	}
	work, _, err := store.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "worker-self-task", Text: "Inspect quote", Lane: "background"}, SubmissionScope{ChannelID: channel.ID, ProjectID: project.ID, ProjectRevision: project.Revision, FileIDs: []string{file.ID}})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	active := store.work[work.RequestID]
	active.Status = "running"
	store.work[work.RequestID] = active
	store.mu.Unlock()
	_, text, err := store.preparePinnedFileText(work.RequestID, work.Epoch, file.ID)
	if err != nil || !strings.Contains(string(text), "Self executable") {
		t.Fatal("real worker tool preparation", err)
	}
	client.Close()
	if _, err = store.PreviewProjectFile(project.ID, channel.ID, file.ID, project.Revision, 0); err == nil {
		t.Fatal("closed worker preview fell back")
	}
	input.ClientNonce = "worker-self-closed-upload"
	quarantined, _, err := store.UploadFile(input)
	if err != nil || quarantined.Status != "quarantined_unsupported" {
		t.Fatal("closed worker upload fell back", err)
	}
}

type lyingDocumentExtractor struct{ result documents.Result }

func (extractor *lyingDocumentExtractor) Extract(format string, raw []byte) (documents.Result, error) {
	return extractor.result, nil
}

func TestDocumentWorkerRejectsLyingResultIdentity(t *testing.T) {
	raw := officeUploadFixture(t, ".xlsx", strings.Repeat("m", 17000)+" original ordinary text")
	valid, err := documents.ExtractXLSX(raw)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]documents.Result{}
	bad := valid
	bad.TextSHA256 = strings.Repeat("a", 64)
	cases["false_hash"] = bad
	bad = valid
	bad.TextBytes++
	cases["false_size"] = bad
	bad = valid
	bad.TextSHA256 = strings.ToUpper(valid.TextSHA256)
	cases["uppercase_hash"] = bad
	cases["zero_metadata"] = documents.Result{Text: valid.Text}
	invalid := string([]byte{0xff})
	sum := sha256.Sum256([]byte(invalid))
	cases["invalid_utf8"] = documents.Result{Text: invalid, TextBytes: 1, TextSHA256: hex.EncodeToString(sum[:])}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			extractor := &lyingDocumentExtractor{result: result}
			store, channel, project := documentWorkerFixture(t, extractor)
			file, _, err := store.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "worker-lying-upload", Filename: "quote.xlsx", Content: raw})
			if err != nil || file.Status != "quarantined_unsupported" || file.ExtractionVersion != 0 {
				t.Fatal("lying extractor promoted text or lost quarantine", err)
			}
			_, body, err := store.ReadProjectFile(project.ID, channel.ID, file.ID)
			if err != nil || string(body) != string(raw) {
				t.Fatal("lying result lost original bytes", err)
			}
		})
	}
}

func TestDocumentWorkerRejectsChangedSuffixOnReread(t *testing.T) {
	raw := officeUploadFixture(t, ".xlsx", strings.Repeat("q", 17000)+" original tail")
	valid, err := documents.ExtractXLSX(raw)
	if err != nil {
		t.Fatal(err)
	}
	extractor := &lyingDocumentExtractor{result: valid}
	store, channel, project := documentWorkerFixture(t, extractor)
	file, _, err := store.UploadFile(UploadInput{ProjectID: project.ID, ProjectRevision: project.Revision, ChannelID: channel.ID, ClientNonce: "worker-suffix-upload", Filename: "quote.xlsx", Content: raw})
	if err != nil || file.Status != "ready_text" {
		t.Fatal("positive source", err)
	}
	if _, err = store.PreviewProjectFile(project.ID, channel.ID, file.ID, project.Revision, 0); err != nil {
		t.Fatal("positive reread", err)
	}
	work, _, err := store.AcceptProduct(Input{BotID: "bot-meiiie", ClientNonce: "worker-suffix-task", Text: "Inspect quote", Lane: "background"}, SubmissionScope{ChannelID: channel.ID, ProjectID: project.ID, ProjectRevision: project.Revision, FileIDs: []string{file.ID}})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	active := store.work[work.RequestID]
	active.Status = "running"
	store.work[work.RequestID] = active
	store.mu.Unlock()
	extractor.result.Text = valid.Text[:len(valid.Text)-1] + "Z"
	if _, err = store.PreviewProjectFile(project.ID, channel.ID, file.ID, project.Revision, 0); err == nil {
		t.Fatal("changed suffix bypassed parent digest")
	}
	if _, _, err = store.preparePinnedFileText(work.RequestID, work.Epoch, file.ID); err == nil {
		t.Fatal("changed suffix reached tool preparation")
	}
}
