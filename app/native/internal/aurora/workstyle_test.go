package aurora

import (
	"context"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/tools"
)

func workstyleService(t *testing.T) (*Service, ledger.Work) {
	t.Helper()
	s, closeService := historyService(t, t.TempDir())
	t.Cleanup(closeService)
	s.Tools = tools.New()
	w, _, err := s.L.Accept(ledgerInput(firstBot(s), "workstyle-fixture", "Review this result"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.L.DirectStart(w.RequestID, w.Epoch, ledger.DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	return s, w
}

func TestWorkSkillsRespectSelectionAndDisable(t *testing.T) {
	s, w := workstyleService(t)
	if err := s.ext.setBot(w.BotID, botExt{Skills: []string{"pstack-debug"}}); err != nil {
		t.Fatal(err)
	}
	note := s.extNote(w.BotID)
	if !strings.Contains(note, "pstack-debug") || strings.Contains(note, "pstack-build") {
		t.Fatal("the prompt advertised a skill outside the owner's selection")
	}
	for _, name := range []string{"pstack-build", "Build and deliver", "missing"} {
		_, err := s.runExt(context.Background(), w, &toolCtx{}, callsOf([]toolReq{{Name: "skill_load", Q: name}})[0])
		if err == nil {
			t.Fatalf("unselected skill %q loaded", name)
		}
	}
	call := callsOf([]toolReq{{Name: "skill_load", Q: "pstack-debug"}})[0]
	result, err := s.runExt(context.Background(), w, &toolCtx{}, call)
	if err != nil || !strings.Contains(result.Text, workDebug) {
		t.Fatal("selected bundled skill could not be read", err)
	}
	if err := s.ext.setBot(w.BotID, botExt{Off: map[string]bool{"skills": true}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.extNote(w.BotID), "pstack-") {
		t.Fatal("disabled skills still instructed the model")
	}
	if _, err := s.runExt(context.Background(), w, &toolCtx{}, call); err == nil {
		t.Fatal("disabled skill still loaded")
	}
}

func TestWorkSkillSurvivesToolBudgetWithoutDuplicateOrCustomPolicy(t *testing.T) {
	s, w := workstyleService(t)
	tc := &toolCtx{}
	call := callsOf([]toolReq{{Name: "skill_load", Q: "pstack-build"}})[0]
	s.runTools(context.Background(), w, tc, []toolCall{call, call})
	s.runTools(context.Background(), w, tc, []toolCall{call})
	cur, _ := s.L.Get(w.RequestID)
	if cur.Direct == nil || len(cur.Direct.Tools) != 1 || cur.Direct.Tools[0].Status != "done" {
		t.Fatal("skill loading must have one durable successful tool receipt")
	}
	for i := 0; i < 12; i++ {
		tc.note(strings.Repeat("untrusted page text ", 400))
	}
	custom := skill{ID: "sk-custom", Name: "Custom review", Body: "custom policy sentinel"}
	s.ext.d.Skills = append(s.ext.d.Skills, custom)
	s.runTools(context.Background(), w, tc, callsOf([]toolReq{{Name: "skill_load", Q: custom.ID}}))
	msgs := tc.inject([]ChatMsg{{Role: "system", Content: "owner authority"}, {Role: "user", Content: "current request"}})
	if msgs[0].Content != "owner authority" || msgs[len(msgs)-2].Content != "current request" {
		t.Fatal("skill loading changed the authority or current request")
	}
	count := 0
	for _, msg := range msgs {
		if strings.HasPrefix(msg.Content, "Aurora bundled playbook ") {
			count++
			if !strings.HasSuffix(msg.Content, workBuild) || strings.Contains(msg.Content, custom.Body) {
				t.Fatal("bundled guidance was truncated or replaced by custom data")
			}
		}
	}
	if count != 1 || !strings.Contains(msgs[len(msgs)-1].Content, custom.Body) || msgs[len(msgs)-1].Role != "user" {
		t.Fatal("custom guidance must remain a tool result, not bundled policy")
	}
	resumed := &toolCtx{}
	s.runTools(context.Background(), w, resumed, []toolCall{call})
	if len(resumed.workSkills) != 1 || resumed.workSkills[0].Body != workBuild {
		t.Fatal("reusing a durable load receipt lost the guide in a rebuilt context")
	}
	other := (&toolCtx{}).inject([]ChatMsg{{Role: "user", Content: "another run"}})
	if len(other) != 1 {
		t.Fatal("a loaded skill leaked into another run")
	}
}

func TestWorkSkillMissingOrPeerDeniedCannotPinPolicy(t *testing.T) {
	s, w := workstyleService(t)
	tc := &toolCtx{}
	s.runTools(context.Background(), w, tc, callsOf([]toolReq{{Name: "skill_load", Q: "pstack-missing"}}))
	if len(tc.workSkills) != 0 {
		t.Fatal("failed load installed policy")
	}
	w.PeerMessageID = "peer-request"
	s.runTools(context.Background(), w, tc, callsOf([]toolReq{{Name: "skill_load", Q: "pstack-review"}}))
	if len(tc.workSkills) != 0 {
		t.Fatal("peer context installed owner workflow policy")
	}
}

func TestWorkSkillLibraryIsBoundedAndBundled(t *testing.T) {
	e := openExt(t.TempDir())
	ids := map[string]bool{}
	for _, sk := range e.skills() {
		if ids[sk.ID] {
			t.Fatal("duplicate bundled skill ID", sk.ID)
		}
		ids[sk.ID] = true
		if strings.HasPrefix(sk.ID, "pstack-") {
			if len(sk.Body) == 0 || len(sk.Body) > maxResultChars {
				t.Fatal("playbook cannot fit in a normal skill read", sk.ID)
			}
		}
	}
	if !ids["csv-report"] || !ids["email-draft"] || len(ids) != 6 {
		t.Fatal("existing skills were lost or the new library was not bundled")
	}
}
