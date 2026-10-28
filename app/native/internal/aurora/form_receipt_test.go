package aurora

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestFormReceiptRequiresDurableAnswer(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	defer srv.Close()
	bot := l.Snapshot().Bots[0].ID
	work, _, err := l.Accept(ledger.Input{BotID: bot, ClientNonce: "form-fixture-source", Text: "collect details"})
	if err != nil {
		t.Fatal(err)
	}
	s.ext.setForm(work.RequestID, &form{Title: "Details", Fields: []formField{{Name: "name", Label: "Name", Type: "text", Required: true}}})
	request := func(path string, body any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		r := post(t, srv.URL+"/app/api"+path, string(b))
		defer r.Body.Close()
		var v map[string]any
		json.NewDecoder(r.Body).Decode(&v)
		return r.StatusCode, v
	}
	done := "/forms/" + work.RequestID + "/done"
	if code, _ := request(done, map[string]string{}); code != 409 {
		t.Fatal("unacknowledged form closed", code)
	}
	path := "/agents/" + bot + "/messages"
	answer := map[string]string{"text": "Name: Aurora", "clientNonce": "form-" + work.RequestID, "formFor": work.RequestID, "workspaceId": s.workspaceID}
	answer["formFor"] = "missing"
	if code, _ := request(path, answer); code != 400 {
		t.Fatal("unknown form", code)
	}
	answer["formFor"] = work.RequestID
	answer["ref"] = "other-chat"
	if code, _ := request(path, answer); code != 400 {
		t.Fatal("mutable context on answer", code)
	}
	delete(answer, "ref")
	code, first := request(path, answer)
	if code != 202 || first["formFor"] != work.RequestID {
		t.Fatal("receipt", code)
	}
	if code, replay := request(path, answer); code != 202 || replay["id"] != first["id"] {
		t.Fatal("receipt replay", code)
	}
	answer["text"] = "replacement"
	if code, _ := request(path, answer); code != 409 {
		t.Fatal("conflicting answer", code)
	}
	if code, _ := request(done, map[string]string{}); code != 200 {
		t.Fatal("accepted form not complete", code)
	}
	_, cached := s.ext.side(work.RequestID)
	if cached.Done {
		t.Fatal("completion must derive from durable receipt")
	}
	s.ext = openExt(filepath.Join(dir, "aurora"))
	m := s.decorate([]*Message{{Role: "agent", TaskID: work.RequestID}})[0]
	if m.Form == nil || !m.Form.Done {
		t.Fatal("receipt lost on side-store reload")
	}
	if len(l.Snapshot().Work) != 2 {
		t.Fatal("duplicate work")
	}

	other, _, err := l.Accept(ledger.Input{BotID: bot, ClientNonce: "form-fixture-failure", Text: "another form"})
	if err != nil {
		t.Fatal(err)
	}
	s.ext.setForm(other.RequestID, &form{Title: "Failure", Fields: cached.Fields})
	l.Close()
	answer["formFor"], answer["clientNonce"] = other.RequestID, "form-"+other.RequestID
	if code, _ := request(path, answer); code < 400 {
		t.Fatal("failed admission acknowledged", code)
	}
	if s.decorate([]*Message{{Role: "agent", TaskID: other.RequestID}})[0].Form.Done {
		t.Fatal("failed admission completed form")
	}
}
