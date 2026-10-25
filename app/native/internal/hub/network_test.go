package hub

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNetworkReservationsSurviveRetryRestartAndQuota(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	s, err := openUsers(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	u, err := s.signIn("fixture@example.com", "email", "", "Fixture", "free", 3, now)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.reserveNetwork(u.ID, "mail-once", now, 1); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.usedToday(u.ID, now); got != 1 {
		t.Fatalf("retry quota = %d", got)
	}
	s, err = openUsers(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.reserveNetwork(u.ID, "mail-once", now, 1); err != nil {
		t.Fatalf("restart lost reservation: %v", err)
	}
	if err := s.reserveNetwork(u.ID, "mail-second", now, 1); err == nil {
		t.Fatal("new wake exceeded quota")
	}
	if got := s.usedToday(u.ID, now); got != 1 {
		t.Fatalf("restart quota = %d", got)
	}
	if err := s.reserveNetwork(u.ID, "mail-once", now.Add(24*time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	if got := s.usedToday(u.ID, now.Add(24*time.Hour)); got != 0 {
		t.Fatalf("retry charged next day: %d", got)
	}
}

func TestNetworkReservationSaveFailureDoesNotAdmit(t *testing.T) {
	s, err := openUsers(filepath.Join(t.TempDir(), "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	u, err := s.signIn("fixture@example.com", "email", "", "Fixture", "free", 3, now)
	if err != nil {
		t.Fatal(err)
	}
	good := s.path
	s.path = filepath.Join(t.TempDir(), "missing", "users.json")
	if err := s.reserveNetwork(u.ID, "mail-storage", now, 1); err == nil {
		t.Fatal("unpaid admission on failed save")
	}
	if got := s.usedToday(u.ID, now); got != 0 {
		t.Fatalf("failed save left phantom in-memory charge: %d", got)
	}
	s.path = good
	if err := s.reserveNetwork(u.ID, "mail-storage", now, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(good); err != nil {
		t.Fatal(err)
	}
	if got := s.usedToday(u.ID, now); got != 1 {
		t.Fatalf("successful retry charged %d", got)
	}
}

func TestNetworkHTTPPassiveMailQuotaIsolationAndActualWakeAdmission(t *testing.T) {
	h, srv, fm := newHub(t, func(c *Config) { c.Plans["free"] = 1 })
	a, b := newClient(t, srv), newClient(t, srv)
	signIn(t, a, fm, "mail-a@example.com")
	signIn(t, b, fm, "mail-b@example.com")
	code, directory := a.do("GET", "/app/api/network", nil)
	if code != 200 {
		t.Fatal(code, directory)
	}
	session := directory["sessions"].([]any)[0].(map[string]any)["id"].(string)
	code, directoryB := b.do("GET", "/app/api/network", nil)
	if code != 200 {
		t.Fatal(code, directoryB)
	}
	if session == directoryB["sessions"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("same local bot alias collided across tenants")
	}
	body := map[string]any{"toSessionId": session, "text": "Fixture FYI", "wake": false, "clientNonce": "hosted-fyi", "attachments": []map[string]string{{"name": "note.txt", "content": "private fixture note"}}}
	code, sent := a.do("POST", "/app/api/network/messages", body)
	if code != 202 {
		t.Fatal(code, sent)
	}
	messageID := sent["message"].(map[string]any)["id"].(string)
	code, plan := a.do("GET", "/app/api/plan", nil)
	if code != 200 || plan["quota"].(map[string]any)["used"] != float64(0) {
		t.Fatal("FYI spent quota", code, plan)
	}
	if code, out := b.do("GET", "/app/api/network/messages/"+messageID+"/files/0", nil); code != 404 {
		t.Fatal("foreign file accessible", code, out)
	}
	if code, out := a.do("GET", "/app/api/network/messages/"+messageID+"/files/0", nil); code != 200 || out["raw"] != "private fixture note" {
		t.Fatal("own file download failed", code, out)
	}
	if code, out := b.do("POST", "/app/api/network/messages", body); code != 404 {
		t.Fatal("foreign recipient accepted", code, out)
	}
	wake := map[string]any{"toSessionId": session, "text": "Answer briefly.", "wake": true, "clientNonce": "hosted-wake"}
	for i := 0; i < 2; i++ {
		if code, out := a.do("POST", "/app/api/network/messages", wake); code != 202 {
			t.Fatal(code, out)
		}
	}
	var uid string
	if _, p := a.do("GET", "/app/api/plan", nil); p["id"] != nil {
		uid = p["id"].(string)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if h.users.usedToday(uid, h.cfg.Now()) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := h.users.usedToday(uid, h.cfg.Now()); got != 1 {
		t.Fatalf("actual wake reserved %d", got)
	}
	tenant, err := h.tenant(uid)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(tenant.store.Snapshot().Work); got != 1 {
		t.Fatalf("retry created %d works", got)
	}
	wake["clientNonce"] = "hosted-overquota"
	if code, out := a.do("POST", "/app/api/network/messages", wake); code != http.StatusAccepted {
		t.Fatal(code, out)
	}
	deadline = time.Now().Add(3 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		_, v := a.do("GET", "/app/api/network", nil)
		for _, m := range v["messages"].([]any) {
			if m.(map[string]any)["taskState"] == "admission_blocked" {
				blocked = true
			}
		}
		if blocked {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !blocked || len(tenant.store.Snapshot().Work) != 1 {
		t.Fatalf("quota bypass: blocked=%v works=%d", blocked, len(tenant.store.Snapshot().Work))
	}
}
