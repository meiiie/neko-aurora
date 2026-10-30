package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

const valuesForm = `FORM: {"title":"Details","fields":[{"name":"name","label":"Name","type":"text","required":true},{"name":"amount","label":"Amount","type":"number"},{"name":"date","label":"Date","type":"date"},{"name":"format","label":"Format","type":"select","options":["Table","Chart"]}]}`

func TestFormValuesServerValidationAndReceipt(t *testing.T) {
	for name, values := range map[string]any{
		"missing": nil, "empty-required": []string{" ", "1", "2026-10-28", "Table"},
		"wrong-count": []string{"Aurora"}, "wrong-type": []any{10, "1", "2026-10-28", "Table"},
		"oversized":       []string{strings.Repeat("x", 2001), "1", "2026-10-28", "Table"},
		"nan":             []string{"Aurora", "NaN", "2026-10-28", "Table"},
		"infinite":        []string{"Aurora", "1e999", "2026-10-28", "Table"},
		"invalid-date":    []string{"Aurora", "1", "2026-02-30", "Table"},
		"wrong-option":    []string{"Aurora", "1", "2026-10-28", "Other"},
		"zero-year":       []string{"Aurora", "1", "0000-01-01", "Table"},
		"missing-version": []string{"Aurora", "1", "2026-10-28", "Table"},
	} {
		t.Run(name, func(t *testing.T) {
			s, closeService := historyService(t, t.TempDir())
			defer closeService()
			s.AI = &historyModel{replies: []string{valuesForm}}
			w, _, err := s.L.Accept(ledger.Input{BotID: firstBot(s), ClientNonce: "values-source-" + name, Text: "Collect"})
			if err != nil {
				t.Fatal(err)
			}
			s.runWork(w)
			srv := httptest.NewServer(s)
			defer srv.Close()
			body := map[string]any{"text": "forged summary", "clientNonce": "form-" + w.RequestID, "formFor": w.RequestID, "workspaceId": s.workspaceID, "formAnswerVersion": 1, "formValues": values}
			if name == "missing-version" {
				delete(body, "formAnswerVersion")
			}
			r := post(t, srv.URL+"/app/api/agents/"+w.BotID+"/messages", mustJSON(body))
			r.Body.Close()
			if r.StatusCode != 400 || len(s.L.Snapshot().Work) != 1 {
				t.Fatal("invalid fields admitted", r.StatusCode)
			}
		})
	}
	t.Run("canonical-replay", func(t *testing.T) {
		dir := t.TempDir()
		s, closeService := historyService(t, dir)
		defer func() { closeService() }()
		s.AI = &historyModel{replies: []string{valuesForm}}
		w, _, err := s.L.Accept(ledger.Input{BotID: firstBot(s), ClientNonce: "values-source-valid", Text: "Collect"})
		if err != nil {
			t.Fatal(err)
		}
		s.runWork(w)
		srv := httptest.NewServer(s)
		defer func() { srv.Close() }()
		body := map[string]any{"text": "forged summary", "clientNonce": "form-" + w.RequestID, "formFor": w.RequestID, "workspaceId": s.workspaceID, "formAnswerVersion": 1, "formValues": []string{" Aurora ", "-12.5", "2024-02-29", "Chart"}}
		request := func() (int, map[string]any) {
			r := post(t, srv.URL+"/app/api/agents/"+w.BotID+"/messages", mustJSON(body))
			defer r.Body.Close()
			var out map[string]any
			json.NewDecoder(r.Body).Decode(&out)
			return r.StatusCode, out
		}
		code, first := request()
		expected := "📋 Details\n- Name: Aurora\n- Amount: -12.5\n- Date: 2024-02-29\n- Format: Chart"
		if code != 202 || first["text"] != expected {
			t.Fatal("server did not render schema values", code, first["text"])
		}
		if mustJSON(first["formValues"]) != mustJSON(body["formValues"]) {
			t.Fatal("receipt omitted structured values")
		}
		srv.Close()
		closeService()
		s, closeService = historyService(t, dir)
		srv = httptest.NewServer(s)
		if code, replay := request(); code != 202 || replay["id"] != first["id"] {
			t.Fatal("restart replay failed", code)
		}
		body["formValues"] = []string{"changed", "-12.5", "2024-02-29", "Chart"}
		if code, _ := request(); code != 409 {
			t.Fatal("changed fields changed receipt", code)
		}
		if len(s.L.Snapshot().Work) != 2 {
			t.Fatal("extra work")
		}
	})
}

func TestFormValuesLegacyJournalStillAcceptsText(t *testing.T) {
	s, closeService := historyService(t, t.TempDir())
	defer closeService()
	w, _, err := s.L.Accept(ledger.Input{BotID: firstBot(s), ClientNonce: "legacy-journal-form", Text: "Collect"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.L.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	f := parseForm(historyForm[len("FORM: "):])
	f.AnswerVersion = 0
	_, err = s.L.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *ledger.DirectRun) { d.Form = f })
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	defer srv.Close()
	r := post(t, srv.URL+"/app/api/agents/"+w.BotID+"/messages", mustJSON(map[string]string{"text": "Legacy reply", "clientNonce": "form-" + w.RequestID, "formFor": w.RequestID}))
	defer r.Body.Close()
	if r.StatusCode != 202 {
		t.Fatal("legacy payload rejected", r.StatusCode)
	}
}

func TestFormValuesReceiptBindsFieldsBeyondRenderedText(t *testing.T) {
	for name, pair := range map[string][2][]string{
		"multiline":    {{"x\n- B: y", "z"}, {"x", "y\n- B: z"}},
		"empty-marker": {{"(bỏ trống)", ""}, {"", "(bỏ trống)"}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s, closeService := historyService(t, dir)
			defer func() { closeService() }()
			s.AI = &historyModel{replies: []string{`FORM: {"title":"Fields","fields":[{"name":"a","label":"A","type":"textarea"},{"name":"b","label":"B","type":"textarea"}]}`}}
			w, _, err := s.L.Accept(ledger.Input{BotID: firstBot(s), ClientNonce: "field-collision-" + name, Text: "Collect"})
			if err != nil {
				t.Fatal(err)
			}
			s.runWork(w)
			srv := httptest.NewServer(s)
			defer func() { srv.Close() }()
			body := map[string]any{"text": "Preview", "clientNonce": "form-" + w.RequestID, "formFor": w.RequestID, "workspaceId": s.workspaceID, "formAnswerVersion": 1, "formValues": pair[0]}
			request := func() int {
				r := post(t, srv.URL+"/app/api/agents/"+w.BotID+"/messages", mustJSON(body))
				defer r.Body.Close()
				return r.StatusCode
			}
			if code := request(); code != 202 {
				t.Fatal(code)
			}
			srv.Close()
			closeService()
			s, closeService = historyService(t, dir)
			srv = httptest.NewServer(s)
			if code := request(); code != 202 {
				t.Fatal("original lost after reopen", code)
			}
			body["formValues"] = pair[1]
			if code := request(); code != 409 {
				t.Fatal("different fields replayed as the same answer", code)
			}
		})
	}
}
