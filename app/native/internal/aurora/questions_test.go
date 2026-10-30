package aurora

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

const questionsFixture = `[{"prompt":"When?","options":["Morning","Evening"]},{"prompt":"Outputs?","options":["Table","Chart","@all"],"multiple":true}]`

func TestStructuredQuestionsParseAndRuntime(t *testing.T) {
	d := ParseDirectives("Before.QUESTIONS: " + questionsFixture + "\nCHOICES: [\"old\",\"style\"]")
	if len(d.Questions) != 2 || len(d.Choices) != 0 || d.Questions[0].ID != "q1" || strings.Contains(d.Text, "QUESTIONS") {
		t.Fatal("directive parse")
	}
	if parseQuestions(`[{"prompt":"x","options":["same","same"]}]`) != nil {
		t.Fatal("duplicate options")
	}
	var decoded any
	json.Unmarshal([]byte(questionsFixture), &decoded)
	pretty, _ := json.MarshalIndent(decoded, "", "  ")
	parsed := ParseDirectives("Before.\nQUESTIONS: \n" + string(pretty) + "\nAfter.")
	if len(parsed.Questions) != 2 || strings.Contains(parsed.Text, "options") || !strings.Contains(parsed.Text, "After.") {
		t.Fatal("multiline card leaked or removed reply text")
	}
	fa := scriptedAI(t, "Bạn cho mình biết nhé?\nQUESTIONS: "+questionsFixture)
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "fixture-key")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	s, _ := newSvc(t)
	srv := httptest.NewServer(s)
	defer srv.Close()
	bot := firstBot(s)
	r := post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"Chuẩn bị bản tóm tắt giúp mình","clientNonce":"structured-runtime-fixture"}`)
	r.Body.Close()
	if r.StatusCode != 202 {
		t.Fatal(r.StatusCode)
	}
	waitTask(t, s, bot)
	found := false
	for _, m := range s.view().messages(bot, 0) {
		if len(m.Questions) == 2 {
			found = true
			if strings.Contains(m.Text, "QUESTIONS:") {
				t.Fatal("directive leaked")
			}
		}
	}
	if !found {
		t.Fatal("runtime did not journal question card")
	}
}

func TestStructuredQuestionsValidateReceiptAndGroupRecipient(t *testing.T) {
	for _, group := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "group"}[group], func(t *testing.T) {
			dir := t.TempDir()
			l, err := ledger.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
			if err != nil {
				t.Fatal(err)
			}
			bot, thread := "bot-meiiie", "bot-meiiie"
			scope := ledger.SubmissionScope{}
			if group {
				c, _, err := l.SaveChannel(ledger.ChannelInput{Name: "Questions", BotIDs: []string{bot, "bot-davin"}, ClientNonce: "questions-group-fixture"})
				if err != nil {
					t.Fatal(err)
				}
				thread = c.ID
				scope.ChannelID = c.ID
				scope.ExpectedChannelRevision = &c.Revision
			}
			w, _, err := l.AcceptProduct(ledger.Input{BotID: bot, Text: "source", ClientNonce: "question-source-fixture"}, scope)
			if err != nil {
				t.Fatal(err)
			}
			w, err = l.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http"})
			if err != nil {
				t.Fatal(err)
			}
			w, err = l.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *ledger.DirectRun) { d.Questions = parseQuestions(questionsFixture) })
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(s)
			defer func() { srv.Close(); l.Close() }()
			request := func(target string, as []questionAnswer) (int, map[string]any) {
				b, _ := json.Marshal(map[string]any{"text": "untrusted client label", "answerFor": w.RequestID, "clientNonce": "choice-" + w.RequestID, "questionAnswers": as})
				r := post(t, srv.URL+"/app/api/agents/"+target+"/messages", string(b))
				defer r.Body.Close()
				var v map[string]any
				json.NewDecoder(r.Body).Decode(&v)
				return r.StatusCode, v
			}
			good := []questionAnswer{{ID: "q1", Text: "My own time"}, {ID: "q2", Selected: []int{2, 0}}}
			bad := [][]questionAnswer{
				good[:1], {{ID: "wrong", Text: "x"}, good[1]},
				{{ID: "q1", Selected: []int{0, 1}}, good[1]},
				{{ID: "q1", Text: "x", Selected: []int{0}}, good[1]},
				{good[0], {ID: "q2", Selected: []int{-1}}},
				{good[0], {ID: "q2", Selected: []int{3}}},
				{good[0], {ID: "q2", Selected: []int{0, 0}}},
				{good[0], {ID: "q2", Text: strings.Repeat("x", 2001)}},
			}
			for _, a := range bad {
				if code, _ := request(thread, a); code != 400 {
					t.Fatal("invalid answer admitted", code)
				}
			}
			if code, _ := request("bot-davin", good); code != 400 {
				t.Fatal("cross-thread answer", code)
			}
			if len(l.Snapshot().Work) != 1 {
				t.Fatal("invalid input created work")
			}
			code, first := request(thread, good)
			if code != 202 || first["answerFor"] != w.RequestID {
				t.Fatal("receipt", code, first)
			}
			if len(l.Snapshot().Work) != 2 {
				t.Fatal("answer fanned out to other group members")
			}
			accepted, _ := l.Get(first["taskId"].(string))
			if accepted.BotID != bot || strings.Contains(accepted.Text, "untrusted") || !strings.Contains(accepted.Text, "Table; @all") {
				t.Fatal("answer context or recipient changed")
			}
			srv.Close()
			l.Close()
			l, err = ledger.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			s, err = New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
			if err != nil {
				t.Fatal(err)
			}
			srv = httptest.NewServer(s)
			code, again := request(thread, good)
			if code != 202 || again["id"] != first["id"] || len(l.Snapshot().Work) != 2 {
				t.Fatal("replayed answer changed", code)
			}
			good[0].Text = "different"
			if code, _ := request(thread, good); code != 409 {
				t.Fatal("second answer accepted", code)
			}
		})
	}
}
