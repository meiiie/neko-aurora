package aurora

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Vault end to end with fake data (4242 test card): the model lists and
// uses an item, never sees a value; the owner approves; Chrome fills a local
// checkout form; the result, card, logs and model context stay masked.
func TestVaultListUseApproveFill(t *testing.T) {
	if browserBin() == "" {
		t.Skip("no chrome")
	}
	var mu sync.Mutex
	var bodies []string
	var replies []string
	n := 0
	fa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		rep := replies[min(n, len(replies)-1)]
		n++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", rep)
	}))
	defer fa.Close()
	var paid string
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_ = r.ParseForm()
			mu.Lock()
			paid = r.Form.Get("cardnumber") + "|" + r.Form.Get("cvc") + "|" + r.Form.Get("exp")
			mu.Unlock()
			fmt.Fprintf(w, "<title>Paid</title><p>Charged card %s, CVC: %s</p>", r.Form.Get("cardnumber"), r.Form.Get("cvc"))
			return
		}
		fmt.Fprint(w, `<title>Checkout</title><form method=post action=/pay><label>Card number <input name=cardnumber autocomplete=cc-number></label><input name=ccname autocomplete=cc-name><input name=exp placeholder="MM/YY"><input name=cvc autocomplete=cc-csc><button>Pay</button></form>`)
	}))
	defer shop.Close()
	old := fillURLOK
	fillURLOK = func(u string) bool { return strings.HasPrefix(u, shop.URL) }
	defer func() { fillURLOK = old }()
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	t.Setenv("ZAI_API_KEY", "fake")
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	res := post(t, srv.URL+"/app/api/vault", `{"type":"card","label":"Thẻ test","fields":{"number":"4242 4242 4242 4242","name":"Test Owner","exp":"12/34","cvc":"987"}}`)
	var it struct{ ID string }
	_ = json.NewDecoder(res.Body).Decode(&it)
	if res.StatusCode != 200 || it.ID == "" {
		t.Fatalf("save %d", res.StatusCode)
	}
	mu.Lock()
	replies = []string{
		"TOOL: {\"name\":\"vault_list\"}",
		"TOOL: {\"name\":\"vault_use\",\"item\":\"" + it.ID + "\",\"url\":\"" + shop.URL + "/checkout\"}",
		"Xong, mình đã thanh toán bằng thẻ đuôi 4242.",
	}
	mu.Unlock()
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"thanh toán đơn hàng bằng thẻ trong Vault, thẻ của mình là 4242 4242 4242 4242"}`)
	waitTask(t, svc, bot)
	p := svc.view().pending()
	if len(p) != 1 || p[0].Approval.Kind != "vault" || strings.Contains(p[0].Approval.Body, "4242 4242") || !strings.Contains(p[0].Approval.Title, "Thẻ test") {
		t.Fatalf("pending %+v", p)
	}
	if r := post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved","body":"edited"}`); r.StatusCode < 400 {
		t.Fatal("a vault use must not be editable")
	}
	post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved"}`)
	var out string
	for i := 0; i < 300; i++ {
		w, _ := svc.L.Get(p[0].TaskID)
		if d := w.Direct.Approval.Delivery; d != nil && d.Status != "sending" {
			out = d.Status + " " + w.Direct.Approval.Outcome
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	got := paid
	mu.Unlock()
	if got != "4242424242424242|987|12/34" {
		t.Fatalf("form got %q (card: %s)", got, out)
	}
	if !strings.HasPrefix(out, "sent") || strings.Contains(out, "4242424242424242") || strings.Contains(out, "987") {
		t.Fatalf("outcome not masked: %s", out)
	}
	waitTask(t, svc, bot)
	time.Sleep(300 * time.Millisecond)
	waitTask(t, svc, bot)
	mu.Lock()
	all := strings.Join(bodies, "\n")
	mu.Unlock()
	if strings.Contains(all, "4242424242424242") || strings.Contains(all, "4242 4242 4242 4242") || strings.Contains(all, "987") {
		t.Fatal("a raw value reached the model")
	}
	if !strings.Contains(all, "Visa •••• 4242") {
		t.Fatal("vault_list metadata missing from the model context")
	}
	var lst struct {
		Uses []struct{ Action, Result string }
	}
	r, _ := http.Get(srv.URL + "/app/api/vault")
	_ = json.NewDecoder(r.Body).Decode(&lst)
	if len(lst.Uses) < 2 || lst.Uses[0].Action != "fill" || strings.Contains(lst.Uses[0].Result, "4242424242424242") {
		t.Fatalf("uses %+v", lst.Uses)
	}
	// reveal is owner-only (same session as every /app/api route) and logged
	r = post(t, srv.URL+"/app/api/vault/"+it.ID+"/reveal", `{}`)
	b, _ := io.ReadAll(r.Body)
	if !strings.Contains(string(b), "4242424242424242") {
		t.Fatal("reveal")
	}
	if svc.Mem.Remember(bot, "Darren dùng thẻ 4242 4242 4242 4242") {
		t.Fatal("memory accepted a stored card number")
	}
}

func TestVaultSecretDirectiveCard(t *testing.T) {
	d := ParseDirectives("Bạn lưu thẻ vào Vault giúp mình nhé.\nSECRET: {\"vault\":\"card\",\"label\":\"Thẻ công ty\",\"desc\":\"để thanh toán\"}")
	if d.Secret == nil || d.Secret.Vault != "card" || d.Secret.Name != "VAULT_CARD" {
		t.Fatalf("%+v", d.Secret)
	}
}

func TestVaultEnvOneCommand(t *testing.T) {
	t.Setenv("AURORA_AI_BASE_URL", "http://127.0.0.1:1")
	svc, _ := newSvc(t)
	if svc.PC == nil || svc.PC.CanRun() != nil {
		t.Skip("no sandbox")
	}
	if _, err := svc.PC.Run(t.Context(), "probe", "true"); err != nil {
		t.Skipf("sandbox unusable: %v", err)
	}
	it, err := svc.V.SaveItem("", "login", "API test", map[string]string{"site": "api.example", "username": "bot", "password": "fake-pass-1234"})
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := svc.fillValues(it.ID)
	env := map[string]string{}
	for k, v := range vals {
		env[vaultEnvName[k]] = v
	}
	out, err := svc.vaultExec(t.Context(), "b1", `echo "len=${#VAULT_PASSWORD} $VAULT_PASSWORD"`, env)
	if err != nil {
		t.Fatal(err)
	}
	if out = svc.V.Redact(out); !strings.Contains(out, "len=14") || strings.Contains(out, "fake-pass-1234") {
		t.Fatalf("%s", out)
	}
	st, _ := svc.PC.Status("b1")
	for _, r := range st.Log {
		if strings.Contains(r.Cmd, "fake-pass") {
			t.Fatal("secret in the command log")
		}
	}
	if out2, _ := svc.vaultExec(t.Context(), "b1", `echo "[$VAULT_PASSWORD]"`, nil); !strings.Contains(out2, "[]") {
		t.Fatalf("env leaked into the next command: %s", out2)
	}
}
