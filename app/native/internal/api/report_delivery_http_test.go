package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nekobot.local/native/internal/ledger"
)

const apiReportCommand = "@Meiiie gửi báo cáo đã làm ở cuộc trò chuyện riêng vào kênh này"
const apiReportReply = "Báo cáo thị trường đã hoàn thành. SAFE-REPORT-RESULT"

func seedReportWork(t *testing.T, s *ledger.Store, bot, conversation, nonce, prompt, reply string, files bool) ledger.Work {
	t.Helper()
	w, _, err := s.AcceptScoped(ledger.Input{BotID: bot, ClientNonce: nonce, Text: prompt, Lane: "foreground"}, conversation, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.Claim(w.RequestID, w.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	b := ledger.CoreBinding{SessionID: strings.Repeat("a", 32), Task: json.RawMessage(`{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","root":"/fixture","activationEpoch":1,"activationId":"cccccccccccccccccccccccccccccccc"}`), Execution: json.RawMessage(`{"version":1}`), CoreSHA256: "ac643e3440ccf8a77e4451688f585643a646d4ba798b128a2c5c89a929c0dab5"}
	if err = s.Admit(w.RequestID, w.Epoch, b); err != nil {
		t.Fatal(err)
	}
	if files {
		for i, f := range []struct{ name, text string }{{"report.md", "SAFE-REPORT-FILE"}, {"private-notes-canary.txt", "PRIVATE-UNREQUESTED-ARTIFACT"}} {
			frame, _ := json.Marshal(map[string]any{"name": "execute_tool", "arguments": map[string]any{"name": "artifact.write_text", "arguments": map[string]string{"filename": f.name, "text": f.text}}, "_meta": map[string]any{"neko.operation": map[string]any{"version": 1, "operationId": strings.Repeat(string(rune('d'+i)), 32), "sessionId": strings.Repeat("a", 32), "taskId": strings.Repeat("b", 32), "root": "/fixture", "activationEpoch": 1, "activationId": strings.Repeat("c", 32)}}})
			if _, err = s.ExecuteTool(w.RequestID, w.Epoch, frame); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = s.AppendText(w.RequestID, w.Epoch, reply); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	w, err = s.Get(w.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func reportAPIFixture(t *testing.T, sources bool) (*ledger.Store, *httptest.Server, *http.Client, ledger.Channel, ledger.Work) {
	t.Helper()
	s, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.SaveChannel(ledger.ChannelInput{Name: "Report team", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "api-report-channel"})
	if err != nil {
		t.Fatal(err)
	}
	var source ledger.Work
	if sources {
		conversation, _, err := s.CreateConversation(ledger.ConversationCreateInput{BotID: "bot-meiiie", Title: "Private report fixture", ClientNonce: "api-report-conversation"})
		if err != nil {
			t.Fatal(err)
		}
		seedReportWork(t, s, "bot-meiiie", conversation.ID, "api-older-private", "PRIVATE-HISTORY-PROMPT-CANARY", "PRIVATE-HISTORY-REPLY-CANARY", false)
		source = seedReportWork(t, s, "bot-meiiie", conversation.ID, "api-report-private", "Báo cáo thị trường. PRIVATE-SOURCE-PROMPT-CANARY", apiReportReply, true)
	}
	server := httptest.NewUnstartedServer(nil)
	handler, err := New(s, server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(func() { server.Close(); s.Close() })
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	r, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return s, server, client, c, source
}

func apiReportInput(c ledger.Channel, nonce string) ledger.ReportDeliveryInput {
	return ledger.ReportDeliveryInput{BotID: "bot-meiiie", DestinationChannelID: c.ID, ExpectedDestinationRevision: c.Revision, ClientNonce: nonce, Text: apiReportCommand}
}

func TestReportDeliveryHTTPDurableClarification(t *testing.T) {
	s, server, client, c, _ := reportAPIFixture(t, false)
	in := apiReportInput(c, "report-delivery-no-source")
	body, _ := json.Marshal(in)
	before := s.Snapshot().Sequence
	r := post(t, client, server.URL, "/v1/report-deliveries", string(body), "")
	defer r.Body.Close()
	if r.StatusCode != 201 {
		t.Fatal("missing report route", r.StatusCode)
	}
	var response struct {
		Delivery  ledger.ReportDelivery `json:"delivery"`
		Duplicate bool                  `json:"duplicate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil || response.Duplicate || response.Delivery.Status != "needs_clarification" || response.Delivery.Source != nil || response.Delivery.Reply != "" {
		t.Fatal("durable clarification", err)
	}
	r = post(t, client, server.URL, "/v1/report-deliveries", string(body), "")
	r.Body.Close()
	if r.StatusCode != 200 || s.Snapshot().Sequence != before+1 {
		t.Fatal("clarification retry")
	}
}

func TestReportDeliveryHTTPAuthorityProjectionAndArtifactBoundary(t *testing.T) {
	s, server, client, c, source := reportAPIFixture(t, true)
	in := apiReportInput(c, "report-http-delivery")
	body, _ := json.Marshal(in)
	for _, check := range []struct {
		client       *http.Client
		origin, body string
		status       int
	}{{http.DefaultClient, "", string(body), 401}, {client, "https://untrusted.example", string(body), 403}, {client, "", strings.TrimSuffix(string(body), "}") + `,"ownerUserId":"forged"}`, 400}, {client, "", string(body) + ` {}`, 400}} {
		r := post(t, check.client, server.URL, "/v1/report-deliveries", check.body, check.origin)
		r.Body.Close()
		if r.StatusCode != check.status {
			t.Fatal("HTTP gate", r.StatusCode, check.status)
		}
	}
	req, _ := http.NewRequest("POST", server.URL+"/v1/report-deliveries", bytes.NewReader(body))
	req.Host = "untrusted.example"
	req.Header.Set("Content-Type", "application/json")
	r, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("host gate")
	}
	r = post(t, client, server.URL, "/v1/report-deliveries", string(body), "")
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 201 {
		t.Fatal("delivery", r.StatusCode, string(raw))
	}
	var response struct {
		Delivery ledger.ReportDelivery `json:"delivery"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	d := response.Delivery
	if d.Reply != apiReportReply || len(d.Artifacts) != 1 || d.Artifacts[0].Filename != "report.md" {
		t.Fatal("report output selection")
	}
	for _, path := range []string{"/v1/report-delivery-receipts?clientNonce=" + in.ClientNonce, "/v1/channels/" + c.ID + "/export?format=json"} {
		r, err = client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		projection, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatal("projection", path, r.StatusCode)
		}
		for _, canary := range []string{"PRIVATE-SOURCE-PROMPT-CANARY", "PRIVATE-HISTORY-PROMPT-CANARY", "PRIVATE-HISTORY-REPLY-CANARY", "private-notes-canary.txt", "PRIVATE-UNREQUESTED-ARTIFACT"} {
			if bytes.Contains(projection, []byte(canary)) {
				t.Fatal("private content leaked", path)
			}
		}
	}
	for _, projection := range []any{s.Snapshot().ReportDeliveries, func() []ledger.Event { e, _ := s.Events(d.CreatedSequence - 1); return e }()} {
		data, _ := json.Marshal(projection)
		if bytes.Contains(data, []byte("private-notes-canary")) || bytes.Contains(data, []byte("PRIVATE-HISTORY")) {
			t.Fatal("projection leaked unrelated result")
		}
	}
	artifactPath := "/v1/report-deliveries/" + d.ID + "/artifacts/" + d.Artifacts[0].ID
	r, err = client.Get(server.URL + artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	file, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || string(file) != "SAFE-REPORT-FILE" || r.Header.Get("X-Artifact-SHA256") != d.Artifacts[0].SHA256 {
		t.Fatal("report download")
	}
	for _, path := range []string{"/v1/report-deliveries/report-delivery-" + strings.Repeat("a", 32) + "/artifacts/" + d.Artifacts[0].ID, "/v1/report-deliveries/" + d.ID + "/artifacts/" + source.Artifacts[1].ID, "/v1/channels/" + c.ID + "/artifacts/" + d.Artifacts[0].ID} {
		r, err = client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 404 {
			t.Fatal("artifact binding", path, r.StatusCode)
		}
	}
	if _, _, err = s.SetSourceAccess(ledger.AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "api-report-revoke"}); err != nil {
		t.Fatal(err)
	}
	r, err = client.Get(server.URL + artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 404 {
		t.Fatal("revoked download")
	}
	r, err = client.Get(server.URL + "/v1/report-delivery-receipts?clientNonce=" + in.ClientNonce)
	if err != nil {
		t.Fatal(err)
	}
	var receipt ledger.ReportDeliveryReceipt
	err = json.NewDecoder(r.Body).Decode(&receipt)
	r.Body.Close()
	if err != nil || !receipt.Found || *receipt.Active || receipt.Delivery.Reply != "" || len(receipt.Delivery.Artifacts) != 0 {
		t.Fatal("retired receipt", err)
	}
}

func TestReportDeliveryHTTPStrictReadonlyQueryAndNonceComposition(t *testing.T) {
	s, server, client, c, _ := reportAPIFixture(t, false)
	before := s.Snapshot().Sequence
	for _, query := range []string{"", "?", "?clientNonce=unknown-report-query&clientNonce=other-report-query", "?clientNonce=unknown-report-query&extra=1", "?clientNonce=short", "?clientNonce=%zz"} {
		r, err := client.Get(server.URL + "/v1/report-delivery-receipts" + query)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatal("query accepted", query, r.StatusCode)
		}
	}
	r, err := client.Get(server.URL + "/v1/report-delivery-receipts?clientNonce=unknown-report-query")
	if err != nil {
		t.Fatal(err)
	}
	var receipt ledger.ReportDeliveryReceipt
	err = json.NewDecoder(r.Body).Decode(&receipt)
	r.Body.Close()
	if err != nil || r.StatusCode != 200 || receipt.Found || s.Snapshot().Sequence != before {
		t.Fatal("lookup mutated", err)
	}
	in := apiReportInput(c, "report-http-compose")
	body, _ := json.Marshal(in)
	r = post(t, client, server.URL, "/v1/report-deliveries", string(body), "")
	r.Body.Close()
	if r.StatusCode != 201 {
		t.Fatal("delivery", r.StatusCode)
	}
	r = post(t, client, server.URL, "/v1/messages", `{"botId":"bot-davin","clientNonce":"report-http-compose","text":"unrelated action","lane":"foreground"}`, "")
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("nonce became Work", r.StatusCode)
	}
	in.Text += " changed"
	body, _ = json.Marshal(in)
	r = post(t, client, server.URL, "/v1/report-deliveries", string(body), "")
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("changed nonce", r.StatusCode)
	}
}

func TestReportDeliveryBrowserSeed(t *testing.T) {
	dir := os.Getenv("NEKO_REPORT_SEED_DIR")
	if dir == "" {
		t.Skip("explicit isolated browser fixture seed only")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("absolute isolated seed directory required")
	}
	s, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Snapshot().Sequence != 0 {
		t.Fatal("empty isolated seed ledger required")
	}
	c, _, err := s.SaveChannel(ledger.ChannelInput{Name: "Report team", BotIDs: []string{"bot-meiiie", "bot-davin"}, ClientNonce: "browser-report-channel"})
	if err != nil {
		t.Fatal(err)
	}
	private, _, err := s.CreateConversation(ledger.ConversationCreateInput{BotID: "bot-meiiie", Title: "Meiiie private fixture", ClientNonce: "browser-private-conversation"})
	if err != nil {
		t.Fatal(err)
	}
	seedReportWork(t, s, "bot-meiiie", private.ID, "browser-private-history", "PRIVATE-HISTORY-PROMPT-CANARY", "PRIVATE-HISTORY-REPLY-CANARY", false)
	w := seedReportWork(t, s, "bot-meiiie", private.ID, "browser-report-source", "Báo cáo thị trường. PRIVATE-SOURCE-PROMPT-CANARY", apiReportReply, true)
	seedReportWork(t, s, "bot-davin", "conversation-bot-davin", "browser-davin-source-one", "Báo cáo Atlas", "Báo cáo Atlas hoàn thành", false)
	seedReportWork(t, s, "bot-davin", "conversation-bot-davin", "browser-davin-source-two", "Báo cáo Boreal", "Báo cáo Boreal hoàn thành", false)
	metadata, _ := json.Marshal(map[string]any{"fixtureOnly": true, "channel": c, "reportReply": apiReportReply, "reportRequestId": w.RequestID, "reportArtifactId": w.Artifacts[0].ID, "reportReplyForSubject": "Báo cáo Atlas hoàn thành", "privateCanaries": []string{"PRIVATE-SOURCE-PROMPT-CANARY", "PRIVATE-HISTORY-PROMPT-CANARY", "PRIVATE-HISTORY-REPLY-CANARY", "private-notes-canary.txt", "PRIVATE-UNREQUESTED-ARTIFACT"}})
	fmt.Println(string(metadata))
}
