package aurora

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/cloudbrowser"
)

type deleteCloudTransport struct {
	fail   bool
	purges int
	opens  int
}

func (f *deleteCloudTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == "/v1/purge" {
		f.purges++
		if f.fail {
			return nil, errors.New("fixture cleanup unavailable")
		}
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		body, _ := json.Marshal(map[string]string{"namespace": in["namespace"], "nonce": in["nonce"], "status": "purged"})
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	f.opens++
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"profileId":"bot-davin","status":"ready","control":"paused","epoch":2,"authState":"unknown","protected":true}`))}, nil
}

func TestCloudAccountDeleteRequiresConfirmedSuccessfulPurge(t *testing.T) {
	dir := t.TempDir()
	s, _, _ := networkManualService(t, dir)
	f := &deleteCloudTransport{fail: true}
	c, err := cloudbrowser.New(cloudbrowser.Config{URL: "https://cloud.example.test", Key: []byte(strings.Repeat("fixture!", 4)), StateDir: filepath.Join(dir, "aurora"), HTTP: &http.Client{Transport: f}})
	if err != nil {
		t.Fatal(err)
	}
	s.Cloud = c
	if _, err = c.Do(context.Background(), cloudbrowser.Command{Profile: "bot-davin", Kind: "open", Actor: "owner", ActorID: "fixture-owner", Nonce: "fixture-delete-open"}); err != nil {
		t.Fatal(err)
	}
	hostCalls := 0
	s.OnDeleteAccount = func() error { hostCalls++; return nil }
	for _, body := range []string{`{}`, `{"confirm":"no"}`} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, cloudOwnerRequest("POST", "/app/api/account/delete", body))
		if rec.Code != 400 {
			t.Fatal("delete missing confirmation accepted")
		}
	}
	if f.purges != 0 || hostCalls != 0 {
		t.Fatal("unconfirmed request purged namespace")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, cloudOwnerRequest("POST", "/app/api/account/delete", `{"confirm":"DELETE"}`))
	if rec.Code != 503 || hostCalls != 0 || f.purges != 1 {
		t.Fatalf("failed purge lost account: status=%d host=%d purges=%d", rec.Code, hostCalls, f.purges)
	}
	if _, err = os.Stat(filepath.Join(dir, "aurora", cloudbrowser.NamespaceFile)); err != nil {
		t.Fatal("failed purge lost namespace locator")
	}
	f.fail = false
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, cloudOwnerRequest("POST", "/app/api/account/delete", `{"confirm":"DELETE"}`))
	if rec.Code != 202 || hostCalls != 1 || f.purges != 2 {
		t.Fatalf("safe deletion retry failed: %d host=%d purge=%d", rec.Code, hostCalls, f.purges)
	}
}

func TestCloudAccountDeleteRetainsUsedStateWhenCleanupUnconfigured(t *testing.T) {
	t.Setenv("AURORA_CLOUD_BROWSER_URL", "")
	t.Setenv("AURORA_CLOUD_BROWSER_KEY", "")
	t.Setenv("AURORA_CLOUD_BROWSER_DOCKER", "")
	dir := t.TempDir()
	s, _, _ := networkManualService(t, dir)
	f := &deleteCloudTransport{}
	c, err := cloudbrowser.New(cloudbrowser.Config{URL: "https://cloud.example.test", Key: []byte(strings.Repeat("fixture!", 4)), StateDir: filepath.Join(dir, "aurora"), HTTP: &http.Client{Transport: f}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Do(context.Background(), cloudbrowser.Command{Profile: "bot-davin", Kind: "open", Actor: "owner", ActorID: "fixture-owner", Nonce: "fixture-delete-open"}); err != nil {
		t.Fatal(err)
	}
	s.Cloud = nil
	hostCalls := 0
	s.OnDeleteAccount = func() error { hostCalls++; return nil }
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, cloudOwnerRequest("POST", "/app/api/account/delete", `{"confirm":"DELETE"}`))
	if rec.Code != 503 || hostCalls != 0 || f.purges != 0 {
		t.Fatal("unconfigured cleanup erased used namespace")
	}
	reconfigured, err := cloudbrowser.New(cloudbrowser.Config{URL: "https://cloud.example.test", Key: []byte(strings.Repeat("fixture!", 4)), StateDir: filepath.Join(dir, "aurora"), HTTP: &http.Client{Transport: f}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reconfigured.Do(context.Background(), cloudbrowser.Command{Profile: "bot-davin", Kind: "open", Actor: "owner", ActorID: "fixture-owner", Nonce: "fixture-reconfigured-open"}); !errors.Is(err, cloudbrowser.ErrCleanupPending) {
		t.Fatal("restored worker configuration resumed a namespace pending account deletion")
	}
}
