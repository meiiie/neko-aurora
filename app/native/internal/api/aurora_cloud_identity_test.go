package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nekoaurora.local/native/internal/identity"
	"nekoaurora.local/native/internal/ledger"
)

func TestPublicAuroraOwnerIdentityFollowsVerifiedCookie(t *testing.T) {
	st, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s, err := New(st, "127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.EnableAurora(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		a, ok := identity.RequestIdentityFrom(r.Context())
		if !ok || a.ActorID != "local-owner" || a.Principal != nil {
			t.Error("verified owner cookie did not establish direct owner identity")
		}
		w.WriteHeader(200)
	}))
	s.EnableAuroraPublic("fixture.example", "fixture-owner-code")
	for _, authenticated := range []bool{false, true} {
		r := httptest.NewRequest("GET", "https://fixture.example/app/api/agents/bot-davin/cloud", nil)
		r.Host = "fixture.example"
		if authenticated {
			r.AddCookie(&http.Cookie{Name: auroraCookie, Value: s.auroraPub.session})
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, r)
		want := 401
		if authenticated {
			want = 200
		}
		if rec.Code != want {
			t.Fatalf("got %d want %d", rec.Code, want)
		}
	}
	if calls != 1 {
		t.Fatal("unauthenticated request reached owner handler")
	}
}
