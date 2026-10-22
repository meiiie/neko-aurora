package api

import (
	"encoding/json"
	"nekoaurora.local/native/internal/ledger"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureChallengeSimulatorDefaultOff(t *testing.T) {
	server, client := setup(t)
	r := post(t, client, server.URL, "/v1/secure-challenges", `{"requestId":"req-fixture","clientNonce":"challenge-default-off"}`, "")
	defer r.Body.Close()
	if r.StatusCode != 501 {
		t.Fatalf("explicit simulator capability missing: %d", r.StatusCode)
	}
}
func TestSecureChallengeSimulatorIsTaskBoundOneUse(t *testing.T) {
	dir := t.TempDir()
	store, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.ConfigureChallengeSimulator(true)
	server := httptest.NewUnstartedServer(nil)
	handler, err := New(store, server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	r, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	call := func(path, body string) (int, map[string]any) {
		r := post(t, client, server.URL, path, body, "")
		defer r.Body.Close()
		var v map[string]any
		json.NewDecoder(r.Body).Decode(&v)
		return r.StatusCode, v
	}
	_, v := call("/v1/messages", `{"botId":"bot-meiiie","clientNonce":"challenge-bound-task","text":"Synthetic secure input task","lane":"background"}`)
	id := v["work"].(map[string]any)["requestId"].(string)
	body := `{"action":"create","requestId":"` + id + `","expectedEpoch":1,"destination":"conversation-bot-meiiie","purpose":"secure_input_simulation","clientNonce":"challenge-create-original"}`
	status, out := call("/v1/secure-challenges", body)
	if status != 200 {
		t.Fatal("challenge create", status)
	}
	code := out["simulationCode"].(string)
	challenge := out["challenge"].(map[string]any)
	cid := challenge["id"].(string)
	fingerprint := challenge["fingerprint"].(string)
	status, duplicate := call("/v1/secure-challenges", body)
	if status != 200 || duplicate["simulationCode"] != "" || duplicate["duplicate"] != true {
		t.Fatal("private proof repeated")
	}
	resolve := `{"action":"approve_simulation","fingerprint":"` + fingerprint + `","clientNonce":"challenge-approve-once","simulationCode":"` + code + `"}`
	status, out = call("/v1/secure-challenges/"+cid+"/resolve", resolve)
	if status != 200 || out["challenge"].(map[string]any)["grantsExecution"] != false || out["challenge"].(map[string]any)["status"] != "approve_simulation" {
		t.Fatal("simulation granted execution", status)
	}
	status, _ = call("/v1/secure-challenges/"+cid+"/resolve", strings.ReplaceAll(resolve, "challenge-approve-once", "challenge-approve-again"))
	if status != 409 {
		t.Fatal("proof reusable", status)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "transactions.jsonl"))
	if err != nil || strings.Contains(string(raw), code) || strings.Contains(string(raw), "simulationCode") {
		t.Fatal("proof persisted")
	}
}
