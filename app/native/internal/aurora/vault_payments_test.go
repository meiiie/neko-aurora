package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func paymentCards(t *testing.T, s *Service) (string, string) {
	t.Helper()
	a, err := s.V.SaveItem("", "card", "Fixture travel", map[string]string{"number": "4111111111111111", "name": "Fixture Owner"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.V.SaveItem("", "card", "Fixture spare", map[string]string{"number": "5555555555554444", "name": "Fixture Owner"})
	if err != nil {
		t.Fatal(err)
	}
	return a.ID, b.ID
}

func TestVaultPaymentPreferenceMetadata(t *testing.T) {
	s, _ := newSvc(t)
	a, b := paymentCards(t, s)
	for _, id := range []string{a, b, ""} {
		w := hintRequest(t, s, "POST", "/app/api/vault/payments", map[string]string{"workspaceId": s.workspaceID, "preferredItem": id})
		var result struct{ WorkspaceID, PreferredItem string }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.WorkspaceID != s.workspaceID || result.PreferredItem != id || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("payment selection HTTP%d", w.Code)
		}
		w = hintRequest(t, s, "GET", "/app/api/vault?workspaceId="+s.workspaceID, nil)
		var listed struct {
			Items []struct {
				ID               string
				PreferredPayment bool
			}
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &listed) != nil {
			t.Fatal("list failed")
		}
		count := 0
		for _, it := range listed.Items {
			if it.PreferredPayment {
				count++
				if it.ID != id {
					t.Fatal("wrong preferred item")
				}
			}
		}
		want := 1
		if id == "" {
			want = 0
		}
		if count != want {
			t.Fatal("selection not reflected in metadata")
		}
		if strings.Contains(w.Body.String(), "4111111111111111") || strings.Contains(w.Body.String(), "5555555555554444") {
			t.Fatal("list exposed card number")
		}
		model, err := s.runVault(ledger.Work{}, &toolCtx{}, toolCall{name: "vault_list"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(model.Text, "ưu tiên thanh toán") != (id != "") {
			t.Fatal("model metadata did not reflect owner preference")
		}
	}
	hintRequest(t, s, "POST", "/app/api/vault/payments", map[string]string{"workspaceId": s.workspaceID, "preferredItem": a})
	old := fillURLOK
	fillURLOK = func(string) bool { return true }
	defer func() { fillURLOK = old }()
	tc := &toolCtx{}
	_, err := s.runVault(ledger.Work{}, tc, toolCall{name: "vault_use", arg: `{"item":"` + a + `","url":"https://example.test/checkout"}`})
	if err != nil || !strings.HasPrefix(tc.pendingCmd, parkedVault) {
		t.Fatal("preferred card bypassed approval parking")
	}
	if ap := s.vaultApproval(strings.TrimPrefix(tc.pendingCmd, parkedVault)); ap == nil || ap.State != "pending" {
		t.Fatal("preferred card did not require approval")
	}
	raw := strings.TrimPrefix(tc.pendingCmd, parkedVault)
	if w := hintRequest(t, s, "POST", "/app/api/vault/payments", map[string]string{"workspaceId": s.workspaceID, "preferredItem": b}); w.Code != 200 {
		t.Fatal("replacement preference failed")
	}
	var use vaultUse
	ap := s.vaultApproval(raw)
	if ap == nil || json.Unmarshal([]byte(ap.Use), &use) != nil || use.Item != a || ap.State != "pending" {
		t.Fatal("changing preference retargeted an existing approval")
	}
	if w := hintRequest(t, s, "POST", "/app/api/vault/"+b+"/delete", map[string]string{"workspaceId": s.workspaceID}); w.Code != 200 {
		t.Fatal("preferred item deletion failed")
	}
	w := hintRequest(t, s, "GET", "/app/api/vault?workspaceId="+s.workspaceID, nil)
	if strings.Contains(w.Body.String(), `"preferredPayment":true`) {
		t.Fatal("delete selected another card automatically")
	}
}

func TestVaultPaymentPreferenceValidation(t *testing.T) {
	s, _ := newSvc(t)
	a, _ := paymentCards(t, s)
	login, err := s.V.SaveItem("", "login", "Fixture login", map[string]string{"password": "fixture-private"})
	if err != nil {
		t.Fatal(err)
	}
	valid := func(workspace, id string) string {
		b, _ := json.Marshal(map[string]string{"workspaceId": workspace, "preferredItem": id})
		return string(b)
	}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"wrong-workspace", valid("other", a), 409}, {"empty-workspace", valid("", a), 400},
		{"missing-workspace", `{"preferredItem":"` + a + `"}`, 400}, {"missing-item", `{"workspaceId":"` + s.workspaceID + `"}`, 400},
		{"null-item", `{"workspaceId":"` + s.workspaceID + `","preferredItem":null}`, 400}, {"wrong-type", `{"workspaceId":42,"preferredItem":""}`, 400},
		{"unknown", `{"extra":true}`, 400}, {"trailing", valid(s.workspaceID, a) + ` {}`, 400}, {"oversized", valid(s.workspaceID, strings.Repeat("x", 17<<10)), 400},
		{"login", valid(s.workspaceID, login.ID), 400}, {"missing", valid(s.workspaceID, "deadbeef"), 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("POST", "/app/api/vault/payments", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("invalid preference HTTP%d, want%d", w.Code, tc.status)
			}
		})
	}
	w := hintRequest(t, s, "GET", "/app/api/vault?workspaceId="+s.workspaceID, nil)
	if strings.Contains(w.Body.String(), `"preferredPayment":true`) {
		t.Fatal("invalid request changed preference")
	}
}

func TestVaultPaymentPreferenceReadOnly(t *testing.T) {
	s, _ := newSvc(t)
	a, _ := paymentCards(t, s)
	dir := t.TempDir()
	w, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	ro, err := ledger.OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	s.L = ro
	r := hintRequest(t, s, "POST", "/app/api/vault/payments", map[string]string{"workspaceId": s.workspaceID, "preferredItem": a})
	if r.Code != 409 {
		t.Fatalf("read-only preference HTTP%d", r.Code)
	}
	r = hintRequest(t, s, "GET", "/app/api/vault?workspaceId="+s.workspaceID, nil)
	if strings.Contains(r.Body.String(), `"preferredPayment":true`) {
		t.Fatal("read-only selection changed metadata")
	}
}
