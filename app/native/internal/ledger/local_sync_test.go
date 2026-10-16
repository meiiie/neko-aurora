package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"nekobot.local/native/internal/syncstream"
)

func TestLocalSyncIdentityReplayAudienceAndFileAccessFences(t *testing.T) {
	s, c, p := productFixture(t)
	scope := LocalSyncScope{BotID: "bot-meiiie", ChannelID: c.ID, ProjectID: p.ID}
	initial, err := s.LocalSync(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	client, err := syncstream.NewClient(initial.InstanceID, initial.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Apply(initial); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.UploadFile(UploadInput{ProjectID: p.ID, ProjectRevision: 1, ChannelID: c.ID, Filename: "scope.txt", Content: []byte("private file canary"), ClientNonce: "sync-file-upload-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	token, _ := client.Cursor().Encode()
	delta, err := s.LocalSync(scope, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Apply(delta); err != nil {
		t.Fatal(err)
	}
	for _, o := range client.Objects() {
		if o.File != nil && o.ID == f.ID && o.File.AccessRevision != 0 {
			t.Fatal("file access revision changed")
		}
	}
	dir := s.dir
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	replayed, err := s.LocalSync(scope, "")
	if err != nil || replayed.InstanceID != initial.InstanceID || replayed.ScopeID != initial.ScopeID || !reflect.DeepEqual(replayed.Snapshot, client.Objects()) {
		t.Fatal("restart changed cursor/projection", err)
	}
	token, _ = client.Cursor().Encode()
	if err = s.SetFileAccess(FileAccessInput{FileID: f.ID, ExpectedRevision: 0, Granted: false, ClientNonce: "sync-file-revoke-fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LocalSync(scope, token); !errors.Is(err, syncstream.ErrScope) {
		t.Fatal("revocation silently rebound scope", err)
	}
	fresh, err := s.LocalSync(scope, "")
	if err != nil || fresh.ScopeID == initial.ScopeID {
		t.Fatal("new authority fence not explicit", err)
	}
	oldCursor := syncstream.Cursor{Version: 1, InstanceID: fresh.InstanceID, ScopeID: fresh.ScopeID, Sequence: fresh.To}
	token, _ = oldCursor.Encode()
	if _, _, err = s.SaveChannel(ChannelInput{ID: c.ID, Name: c.Name, BotIDs: []string{"bot-meiiie"}, ExpectedRevision: 1, ClientNonce: "sync-channel-revoke-fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LocalSync(scope, token); !errors.Is(err, syncstream.ErrScope) {
		t.Fatal("audience silently rebound scope", err)
	}
	ahead := oldCursor
	ahead.Sequence = 999999
	token, _ = ahead.Encode()
	newBinding, _ := s.LocalSync(scope, "")
	ahead.ScopeID = newBinding.ScopeID
	token, _ = ahead.Encode()
	if _, err = s.LocalSync(scope, token); !errors.Is(err, syncstream.ErrAhead) {
		t.Fatal("ahead cursor accepted", err)
	}
}

func TestCorruptOptionalSyncIdentityDoesNotDisableUnrelatedLocalJournal(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err = os.WriteFile(filepath.Join(dir, "local-sync.identity"), []byte("partial metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal("optional replica metadata blocked local service", err)
	}
	defer s.Close()
	if _, _, err = s.Accept(input("sync-unrelated-journal-alive", "foreground")); err != nil {
		t.Fatal("unrelated intake unavailable", err)
	}
	if _, err = s.LocalSync(LocalSyncScope{BotID: "bot-meiiie"}, ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal("corrupt identity silently reset replica", err)
	}
	for _, cap := range s.Snapshot().Capabilities {
		if cap == "local_sync_simulation_v1" {
			t.Fatal("unavailable sync advertised ready")
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "local-sync.identity"))
	if string(raw) != "partial metadata" {
		t.Fatal("existing identity overwritten without explicit reset")
	}
}

func TestLocalSyncFirstRenameCannotCollideWithBuiltinRevision(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	scope := LocalSyncScope{BotID: "bot-meiiie"}
	initial, err := s.LocalSync(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := syncstream.NewClient(initial.InstanceID, initial.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	c.Apply(initial)
	if _, _, err = s.RenameBot("bot-meiiie", BotNameInput{Name: "Renamed", ClientNonce: "sync-first-rename-fixture"}); err != nil {
		t.Fatal(err)
	}
	token, _ := c.Cursor().Encode()
	delta, err := s.LocalSync(scope, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Apply(delta); err != nil {
		t.Fatal("sequence-one rename conflicts with builtin", err)
	}
	fresh, err := s.LocalSync(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Apply(fresh); err != nil {
		t.Fatal("same cursor snapshot", err)
	}
	if len(c.Objects()) != 1 || c.Objects()[0].Bot.Name != "Renamed" || c.Objects()[0].Revision != 2 {
		t.Fatal("rename projection not monotonic")
	}
}
