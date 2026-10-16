package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"testing"
	"time"

	"nekobot.local/native/internal/syncstream"
)

func TestLocalSyncTwoAuthenticatedClientsReconcileWithoutReplayingMessages(t *testing.T) {
	server, first := setup(t)
	jar, _ := cookiejar.New(nil)
	second := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	r, err := second.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	fetch := func(client *http.Client, query string) syncstream.Envelope {
		r, err := client.Get(server.URL + "/v1/local-sync?botId=bot-meiiie" + query)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatalf("read-only local sync endpoint absent/rejected: %d", r.StatusCode)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range [][]byte{[]byte("contextPin"), []byte("accountPin"), []byte("runtimeContext"), []byte("operations"), []byte("excerpt"), []byte("clientNonce")} {
			if bytes.Contains(raw, private) {
				t.Fatal("sync leaked private execution/intent fields")
			}
		}
		e, err := syncstream.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	initial := fetch(first, "")
	a, err := syncstream.NewClient(initial.InstanceID, initial.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := syncstream.NewClient(initial.InstanceID, initial.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Apply(initial); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Apply(initial); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"botId":"bot-davin","clientNonce":"sync-unrelated-request","text":"private other bot canary","lane":"foreground"}`,
		`{"botId":"bot-meiiie","clientNonce":"sync-selected-request","text":"selected local request","lane":"foreground"}`,
	} {
		r := post(t, first, server.URL, "/v1/messages", body, "")
		r.Body.Close()
		if r.StatusCode != 201 {
			t.Fatal(r.StatusCode)
		}
	}
	token, err := a.Cursor().Encode()
	if err != nil {
		t.Fatal(err)
	}
	delta := fetch(first, "&cursor="+url.QueryEscape(token))
	if len(delta.Frames) != 2 || len(delta.Frames[0].Updates) != 0 || delta.HighWater != 2 {
		t.Fatal("global sequence/filter gap", delta.HighWater)
	}
	if _, err = a.Apply(delta); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Apply(delta); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Objects(), b.Objects()) || a.Cursor() != b.Cursor() {
		t.Fatal("two local clients diverged")
	}
	if outcome, err := b.Apply(delta); err != nil || outcome != syncstream.Ignored {
		t.Fatal("duplicate frame replay changed state", outcome, err)
	}
	latest := fetch(second, "")
	if _, err = b.Apply(latest); err != nil {
		t.Fatal("same-cursor snapshot conflicts", err)
	}
	if !reflect.DeepEqual(a.Objects(), b.Objects()) {
		t.Fatal("snapshot differs from journal increment")
	}
	for _, object := range a.Objects() {
		if object.Message != nil && object.Message.Text == "private other bot canary" {
			t.Fatal("private bot leaked across scope")
		}
	}
	r, err = http.Get(server.URL + "/v1/local-sync?botId=bot-meiiie")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatal("unauthenticated sync", r.StatusCode)
	}
}
