package aurora

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

const historyForm = `FORM: {"title":"Details","fields":[{"name":"name","label":"Name","type":"text","required":true},{"name":"format","label":"Format","type":"select","options":["Table","Chart"]}]}`

type historyModel struct {
	replies []string
	calls   int
}

func (a *historyModel) Configured() bool  { return true }
func (a *historyModel) Runtime() string   { return "direct-http" }
func (a *historyModel) ModelName() string { return "form-fixture" }
func (a *historyModel) Stream(_ context.Context, _ []ChatMsg, on func(string)) (string, error) {
	i := a.calls
	a.calls++
	if i >= len(a.replies) {
		return "Unexpected extra call.", nil
	}
	on(a.replies[i])
	return a.replies[i], nil
}

func historyService(t *testing.T, dir string) (*Service, func()) {
	t.Helper()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		cancel()
		l.Close()
		t.Fatal(err)
	}
	return s, func() { cancel(); s.Wait(); l.Close() }
}

func TestFormHistorySurvivesEvictionAndRestart(t *testing.T) {
	dir := t.TempDir()
	s, closeService := historyService(t, dir)
	defer func() { closeService() }()
	bot := firstBot(s)
	s.AI = &historyModel{replies: []string{historyForm}}
	source, _, err := s.L.Accept(ledger.Input{BotID: bot, ClientNonce: "history-form-source", Text: "Collect details"})
	if err != nil {
		t.Fatal(err)
	}
	s.runWork(source)
	check := func(done bool) {
		t.Helper()
		m := s.decorate([]*Message{{Role: "agent", TaskID: source.RequestID}})[0]
		if m.Form == nil || m.Form.Title != "Details" || m.Form.Done != done || m.Form.Fields[1].Options[0] != "Table" {
			t.Fatal("form lost or changed", m.Form)
		}
		m.Form.Fields[1].Options[0] = "mutated presentation"
	}
	check(false)
	s.ext.mu.Lock()
	for i := 0; i <= maxSide; i++ {
		s.ext.touchLocked(fmt.Sprintf("history-evict-%d", i))
	}
	s.ext.saveLocked()
	s.ext.mu.Unlock()
	check(false)
	closeService()
	s, closeService = historyService(t, dir)
	check(false)
	srv := httptest.NewServer(s)
	defer srv.Close()
	path := srv.URL + "/app/api/agents/" + bot + "/messages"
	answer := map[string]string{"text": "Name: Aurora\nFormat: Table", "clientNonce": "form-" + source.RequestID, "formFor": source.RequestID, "workspaceId": s.workspaceID}
	var first string
	for i := 0; i < 2; i++ {
		r := post(t, path, mustJSON(answer))
		var receipt map[string]any
		json.NewDecoder(r.Body).Decode(&receipt)
		r.Body.Close()
		if r.StatusCode != 202 {
			t.Fatal("durable unanswered form cannot be submitted", r.StatusCode)
		}
		if i == 0 {
			first, _ = receipt["id"].(string)
		} else if receipt["id"] != first {
			t.Fatal("retry changed receipt")
		}
	}
	check(true)
	if len(s.L.Snapshot().Work) != 2 {
		t.Fatal("duplicate answer")
	}
}

func TestFormHistoryPausesPlan(t *testing.T) {
	for _, stage := range []string{"initial", "step"} {
		t.Run(stage, func(t *testing.T) {
			s, closeService := historyService(t, t.TempDir())
			defer closeService()
			plan := "Tôi sẽ chuẩn bị.\nPLAN: [\"Lấy thông tin\",\"Chuẩn bị kết quả\"]"
			replies := []string{plan + "\n" + historyForm}
			if stage == "step" {
				replies = []string{plan, historyForm}
			}
			model := &historyModel{replies: replies}
			s.AI = model
			w, _, err := s.L.Accept(ledger.Input{BotID: firstBot(s), ClientNonce: "history-pause-" + stage, Text: "Chuẩn bị báo cáo giúp mình"})
			if err != nil {
				t.Fatal(err)
			}
			s.runWork(w)
			done, _ := s.L.Get(w.RequestID)
			if done.Status != "completed" || model.calls != len(replies) {
				t.Fatal("plan continued past form", done.Status, model.calls)
			}
			if stage == "step" && (len(done.Direct.Plan) != 2 || done.Direct.Plan[0].Status != "waiting" || done.Direct.Plan[1].Status != "cancelled") {
				t.Fatal("plan did not wait")
			}
			if m := s.decorate([]*Message{{Role: "agent", TaskID: w.RequestID}})[0]; m.Form == nil {
				t.Fatal("form omitted")
			}
		})
	}
}

func TestFormCompletionRequiresJournalCommit(t *testing.T) {
	dir := t.TempDir()
	s, closeService := historyService(t, dir)
	defer func() { closeService() }()
	w, _, err := s.L.Accept(ledger.Input{BotID: firstBot(s), ClientNonce: "form-finish-failure", Text: "Collect"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.L.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 100)
	s.mu.Lock()
	s.subs[events] = struct{}{}
	s.mu.Unlock()
	s.L.Close()
	s.finishWork(w, nil, &questionSet{Form: parseForm(historyForm[len("FORM: "):])})
	got, _ := s.L.Get(w.RequestID)
	if got.Status != "running" || got.Direct.Form != nil {
		t.Fatal("failed finish committed")
	}
	if m := s.decorate([]*Message{{Role: "agent", TaskID: w.RequestID}})[0]; m.Form != nil {
		t.Fatal("uncommitted form visible")
	}
	foundError := false
	for len(events) > 0 {
		event := <-events
		if a, ok := event.Data.(AGUIEvent); ok {
			if a.Type == RUN_FINISHED {
				t.Fatal("false completion event")
			}
			if a.Type == RUN_ERROR && a.Message == "completion_not_recorded" {
				foundError = true
			}
		}
	}
	if !foundError {
		t.Fatal("missing persistence failure event")
	}
	closeService()
	s, closeService = historyService(t, dir)
	got, _ = s.L.Get(w.RequestID)
	if got.Direct.Form != nil {
		t.Fatal("failed schema appeared after restart")
	}
}

func TestFormHistoryIgnoresForgedCompletionNonce(t *testing.T) {
	s, closeService := historyService(t, t.TempDir())
	defer closeService()
	bot := firstBot(s)
	s.AI = &historyModel{replies: []string{historyForm}}
	w, _, err := s.L.Accept(ledger.Input{BotID: bot, ClientNonce: "forged-form-source", Text: "Collect"})
	if err != nil {
		t.Fatal(err)
	}
	s.runWork(w)
	if _, _, err = s.L.Accept(ledger.Input{BotID: bot, ClientNonce: "form-" + w.RequestID, Text: "legacy nonce without receipt"}); err != nil {
		t.Fatal(err)
	}
	m := s.decorate([]*Message{{Role: "agent", TaskID: w.RequestID}})[0]
	if m.Form == nil || m.Form.Done {
		t.Fatal("nonce falsely completed durable form")
	}
	srv := httptest.NewServer(s)
	defer srv.Close()
	r := post(t, srv.URL+"/app/api/forms/"+w.RequestID+"/done", "{}")
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("missing receipt accepted", r.StatusCode)
	}
	s.ext = nil
	if m = s.decorate([]*Message{{Role: "agent", TaskID: w.RequestID}})[0]; m.Form == nil {
		t.Fatal("ledger form depends on cache")
	}
}

func TestFormDefinitionNormalizationAndCopies(t *testing.T) {
	f := parseForm(`{"title":"T","done":true,"fields":[{"label":"Name"},{"name":"format","label":"Format","type":"select","options":[" ","Table","Table","Chart"]},{"name":"format","label":"Duplicate"},{"name":"note","label":"Note","options":["stray"]}]}`)
	if f == nil || !ledger.ValidDirectForm(f) || f.Done || len(f.Fields) != 3 || len(f.Fields[1].Options) != 2 || len(f.Fields[2].Options) != 0 {
		t.Fatal("invalid normalized definition", f)
	}
	e := openExt(t.TempDir())
	e.setForm("fixture", f)
	f.Fields[1].Options[0] = "mutated input"
	_, first := e.side("fixture")
	first.Fields[1].Options[0] = "mutated output"
	_, second := e.side("fixture")
	if second.Fields[1].Options[0] != "Table" {
		t.Fatal("legacy cache aliases fields")
	}
	if promiseOnly(directives{Text: "I will prepare this", Form: f}) {
		t.Fatal("form triggered extra inference")
	}
}

func TestFormHistoryGroupScopeAndPendingTurn(t *testing.T) {
	dir := t.TempDir()
	s, closeService := historyService(t, dir)
	defer func() { closeService() }()
	bots := s.L.Snapshot().Bots
	group, _, err := s.L.SaveChannel(ledger.ChannelInput{Name: "Details", BotIDs: []string{bots[0].ID, bots[1].ID}, ClientNonce: "history-group"})
	if err != nil {
		t.Fatal(err)
	}
	w, _, err := s.L.AcceptProduct(ledger.Input{BotID: bots[1].ID, ClientNonce: "history-group-source", Text: "Collect"}, ledger.SubmissionScope{ChannelID: group.ID, ExpectedChannelRevision: &group.Revision})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.L.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	s.AI = &historyModel{replies: []string{historyForm}}
	approval, pending, err := s.runTurn(s.ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.Form == nil {
		t.Fatal("missing pending form")
	}
	if m := s.decorate([]*Message{{Role: "agent", TaskID: w.RequestID}})[0]; m.Form != nil {
		t.Fatal("uncommitted turn exposed form")
	}
	s.finishWork(w, approval, pending)
	closeService()
	s, closeService = historyService(t, dir)
	srv := httptest.NewServer(s)
	defer srv.Close()
	answer := map[string]string{"text": "Name: Aurora", "clientNonce": "form-" + w.RequestID, "formFor": w.RequestID, "workspaceId": s.workspaceID}
	r := post(t, srv.URL+"/app/api/agents/"+w.BotID+"/messages", mustJSON(answer))
	r.Body.Close()
	if r.StatusCode != 400 {
		t.Fatal("group form accepted in bot chat", r.StatusCode)
	}
	answer["workspaceId"] = "wrong-workspace"
	r = post(t, srv.URL+"/app/api/agents/"+group.ID+"/messages", mustJSON(answer))
	r.Body.Close()
	if r.StatusCode != 409 {
		t.Fatal("wrong workspace accepted", r.StatusCode)
	}
	answer["workspaceId"] = s.workspaceID
	r = post(t, srv.URL+"/app/api/agents/"+group.ID+"/messages", mustJSON(answer))
	r.Body.Close()
	if r.StatusCode != 202 {
		t.Fatal("group answer rejected", r.StatusCode)
	}
	for _, work := range s.L.Snapshot().Work {
		if work.RequestID != w.RequestID && (work.BotID != w.BotID || work.ConversationID != group.ID) {
			t.Fatal("answer routed to wrong recipient")
		}
	}
	if len(s.L.Snapshot().Work) != 2 {
		t.Fatal("answer fanned out")
	}
	if m := s.decorate([]*Message{{Role: "agent", TaskID: w.RequestID}})[0]; m.Form == nil || !m.Form.Done {
		t.Fatal("group receipt missing")
	}
}
