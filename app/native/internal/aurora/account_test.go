package aurora

import (
	"net/http/httptest"
	"testing"
)

func TestAccountDeleteNeedsConfirmation(t *testing.T) {
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	u := srv.URL + "/app/api/account/delete"
	if r := post(t, u, `{"confirm":"XOÁ"}`); r.StatusCode != 501 {
		t.Fatalf("no host hook: got %d", r.StatusCode)
	}
	calls := 0
	svc.OnDeleteAccount = func() error { calls++; return nil }
	for _, b := range []string{`{}`, `{"confirm":"ok"}`, `not json`} {
		if r := post(t, u, b); r.StatusCode != 400 {
			t.Fatalf("%s: got %d", b, r.StatusCode)
		}
	}
	if calls != 0 {
		t.Fatal("deleted without confirmation")
	}
	if r := post(t, u, `{"confirm":" xoá "}`); r.StatusCode != 202 || calls != 1 {
		t.Fatalf("confirmed delete: %d calls=%d", r.StatusCode, calls)
	}
}
