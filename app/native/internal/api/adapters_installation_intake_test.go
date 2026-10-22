package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"nekoaurora.local/native/internal/executors"
	"nekoaurora.local/native/internal/ledger"
)

type installationIntakeFixture struct {
	store      *ledger.Store
	server     *Server
	registry   *executors.Registry
	pin        executors.InstallPin
	descriptor executors.Descriptor
	path       string
	root       string
}

func newInstallationIntakeFixture(t *testing.T) *installationIntakeFixture {
	t.Helper()
	f := &installationIntakeFixture{root: t.TempDir()}
	var err error
	f.store, err = ledger.Open(filepath.Join(f.root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.store.Close() })
	f.server, err = New(f.store, "127.0.0.1:8765")
	if err != nil {
		t.Fatal(err)
	}
	f.descriptor = executors.Descriptor{HostSHA256: strings.Repeat("1", 64), CoreSHA256: strings.Repeat("2", 64), RootSHA256: strings.Repeat("3", 64), CapabilitiesSHA256: strings.Repeat("4", 64)}
	f.registry, err = executors.Open(filepath.Join(f.root, "installation"), f.descriptor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.registry.Close() })
	f.store.ConfigureExecutor(f.registry)
	f.pin, err = f.registry.ReadInstallation()
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := f.store.SaveChannel(ledger.ChannelInput{Name: "Wiii exact installation", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "installation-channel"})
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := f.store.RegisterAdapter(ledger.AdapterSessionInput{SourceApp: "wiii", SourceSessionID: "wiii-session", SourceThreadID: "wiii-thread", ChannelID: c.ID, ExpectedChannelRevision: c.Revision, ClientNonce: "installation-session"})
	if err != nil {
		t.Fatal(err)
	}
	f.path = "/v1/adapters/sessions/" + a.ID + "/messages"
	return f
}

func (f *installationIntakeFixture) message(nonce string) map[string]any {
	return map[string]any{"botId": "bot-meiiie", "text": "keep original installation", "lane": "foreground", "sourceMessageId": "source-" + nonce, "clientNonce": nonce, "expectedInstallationPin": f.pin}
}

func (f *installationIntakeFixture) post(body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "http://"+f.server.host+f.path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://"+f.server.host)
	r.AddCookie(&http.Cookie{Name: "neko_local", Value: f.server.session})
	w := httptest.NewRecorder()
	f.server.ServeHTTP(w, r)
	return w
}

func decodeInstallationWork(t *testing.T, w *httptest.ResponseRecorder, code int) ledger.Work {
	t.Helper()
	if w.Code != code {
		t.Fatalf("expected status %d, got %d: %s", code, w.Code, w.Body.String())
	}
	var out struct{ Work ledger.Work }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Work
}

func TestAdapterInstallationIntakeCASAndOriginalDuplicateAfterRotation(t *testing.T) {
	f := newInstallationIntakeFixture(t)
	body := f.message("installation-first")
	w := decodeInstallationWork(t, f.post(body), 201)
	if w.ExecutorPin == nil || *w.ExecutorPin != f.pin || w.Adapter == nil || w.Adapter.SourceThreadID != "wiii-thread" {
		t.Fatal("original installation or source provenance lost", w.RequestID)
	}
	rotated, _, err := f.registry.Configure(f.pin, f.descriptor, "installation-rotate")
	if err != nil {
		t.Fatal(err)
	}
	before := f.store.Snapshot().Sequence
	if got := f.post(f.message("installation-stale")); got.Code != 409 {
		t.Fatal("stale metadata admitted", got.Code, got.Body.String())
	}
	duplicate := decodeInstallationWork(t, f.post(body), 200)
	if duplicate.RequestID != w.RequestID || duplicate.ExecutorPin == nil || *duplicate.ExecutorPin != f.pin {
		t.Fatal("lost ACK rebound task")
	}
	body["expectedInstallationPin"] = rotated.Pin
	if got := f.post(body); got.Code != 409 {
		t.Fatal("changed installation reused nonce", got.Code)
	}
	if f.store.Snapshot().Sequence != before {
		t.Fatal("stale or duplicate call wrote journal")
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	f.store, err = ledger.Open(filepath.Join(f.root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	f.store.ConfigureExecutor(f.registry)
	f.server, err = New(f.store, "127.0.0.1:8765")
	if err != nil {
		t.Fatal(err)
	}
	body["expectedInstallationPin"] = f.pin
	duplicate = decodeInstallationWork(t, f.post(body), 200)
	if duplicate.RequestID != w.RequestID || *duplicate.ExecutorPin != f.pin {
		t.Fatal("replay changed original pin")
	}
}

func TestAdapterInstallationIntakeValidationRevocationAndLegacy(t *testing.T) {
	f := newInstallationIntakeFixture(t)
	before := f.store.Snapshot().Sequence
	for _, pin := range []any{map[string]any{}, map[string]any{"version": 1}, map[string]any{"version": 2, "executorId": f.pin.ExecutorID, "generation": f.pin.Generation}} {
		body := f.message("installation-invalid")
		body["expectedInstallationPin"] = pin
		if got := f.post(body); got.Code != 400 {
			t.Fatal("invalid installation pin", got.Code, got.Body.String())
		}
	}
	if f.store.Snapshot().Sequence != before {
		t.Fatal("invalid pin committed")
	}
	legacy := f.message("installation-legacy")
	delete(legacy, "expectedInstallationPin")
	decodeInstallationWork(t, f.post(legacy), 201)
	if _, _, err := f.registry.Revoke(f.pin, "installation-revoke"); err != nil {
		t.Fatal(err)
	}
	before = f.store.Snapshot().Sequence
	if got := f.post(f.message("installation-revoked")); got.Code != 409 {
		t.Fatal("revoked installation admitted", got.Code, got.Body.String())
	}
	if f.store.Snapshot().Sequence != before {
		t.Fatal("revoked intake wrote journal")
	}
	capability := false
	for _, c := range f.store.Snapshot().Capabilities {
		capability = capability || c == "local_adapter_installation_intake_cas_v1"
	}
	if !capability {
		t.Fatal("CAS capability absent")
	}
}

func TestAdapterInstallationIntakeConcurrentRotationNeverAcceptsNewPin(t *testing.T) {
	f := newInstallationIntakeFixture(t)
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results <- f.post(f.message(fmt.Sprintf("installation-race-%02d", i)))
		}(i)
	}
	wg.Add(1)
	rotateErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		<-start
		_, _, err := f.registry.Configure(f.pin, f.descriptor, "installation-race-rotate")
		rotateErr <- err
	}()
	close(start)
	wg.Wait()
	close(results)
	if err := <-rotateErr; err != nil {
		t.Fatal(err)
	}
	for r := range results {
		if r.Code == 201 {
			w := decodeInstallationWork(t, r, 201)
			if w.ExecutorPin == nil || *w.ExecutorPin != f.pin {
				t.Fatal("metadata-to-intake race committed different installation", w.RequestID)
			}
		} else if r.Code != 409 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
}

func TestAdapterCapabilitiesMetadataOnlyAuthenticatedReadonly(t *testing.T) {
	f := newInstallationIntakeFixture(t)
	call := func(path, host, origin string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://"+f.server.host+path, nil)
		r.Host = host
		r.Header.Set("Origin", origin)
		if auth {
			r.AddCookie(&http.Cookie{Name: "neko_local", Value: f.server.session})
		}
		w := httptest.NewRecorder()
		f.server.ServeHTTP(w, r)
		return w
	}
	path := "/v1/adapters/capabilities"
	before := f.store.Snapshot().Sequence
	w := call(path, f.server.host, "", true)
	if w.Code != 200 {
		t.Fatal("capability route missing", w.Code, w.Body.String())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || string(out["version"]) != "1" || string(out["capabilities"]) != `["local_adapter_installation_intake_cas_v1","local_adapter_history_repair_v1"]` || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("capabilities leaked unrelated metadata", w.Body.String())
	}
	if call(path, f.server.host, "", false).Code != 401 || call(path, "evil.invalid", "", true).Code != 403 || call(path, f.server.host, "http://evil.invalid", true).Code != 403 {
		t.Fatal("capabilities auth bypass")
	}
	for _, suffix := range []string{"?", "?projectId=anything", "?x=%zz", "?x=1&x=2"} {
		if got := call(path+suffix, f.server.host, "", true); got.Code != 400 {
			t.Fatal("query accepted", suffix, got.Code)
		}
	}
	if f.store.Snapshot().Sequence != before {
		t.Fatal("capabilities wrote journal")
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	if got := call(path, f.server.host, "", true); got.Code != 503 {
		t.Fatal("closed store returned metadata", got.Code)
	}
}
