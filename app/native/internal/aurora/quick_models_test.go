package aurora

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestQuickAccountModelsRefreshSelectionAndFailure(t *testing.T) {
	s, srv := quickFixture(t)
	bot := firstBot(s)
	s.quick.ChatGPT = true
	be := s.ext.bot(bot)
	be.Model = "core-chatgpt:retired-fixture"
	if err := s.ext.setBot(bot, be); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.quick.runModels = func(context.Context) (quickModelCatalog, error) {
		calls++
		return quickModelCatalog{Models: []quickModelOption{
			{ID: "gpt-current", Label: "Current account model", Available: true, Transport: "codex"},
			{ID: "gpt-needs-update", Label: "Needs newer component", Available: false, Transport: "codex", Issue: "core_support_required"},
		}, CheckedAt: "2026-10-31T00:00:00Z"}, nil
	}
	in := map[string]any{"workspaceId": "wrong", "botId": bot}
	quickPost(t, srv, "/models", in, 409)
	if calls != 0 {
		t.Fatal("wrong workspace used account")
	}
	in["workspaceId"] = s.workspaceID
	data := quickPost(t, srv, "/models", in, 200)
	if calls != 1 || !strings.Contains(string(data), "Current account model") {
		t.Fatal("account catalog missing")
	}
	if s.quickSelection(bot) != be.Model || s.aiViewFor(bot)["issue"] != "core_model_unavailable" {
		t.Fatal("refresh replaced or concealed unavailable selection")
	}
	if !validModel(s.modelMenu(), "core-chatgpt:gpt-current") || validModel(s.modelMenu(), "core-chatgpt:gpt-needs-update") || validModel(s.modelMenu(), be.Model) {
		t.Fatal("menu does not match runnable account models")
	}
	save := map[string]any{"workspaceId": s.workspaceID, "botId": bot, "provider": "chatgpt", "model": "gpt-current", "expectedSelection": be.Model}
	quickPost(t, srv, "", save, 200)
	if s.aiViewFor(bot)["modelLabel"] != "Current account model" {
		t.Fatal("display label differs from chosen model")
	}
	s.quick.runModels = func(context.Context) (quickModelCatalog, error) {
		return quickModelCatalog{}, errors.New("private provider body must not escape")
	}
	data = quickPost(t, srv, "/models", in, 200)
	if strings.Contains(string(data), "private provider") || !strings.Contains(string(data), "core_catalog_unavailable") {
		t.Fatal("unsafe or missing error")
	}
	if s.quickSelection(bot) != "core-chatgpt:gpt-current" || !validModel(s.modelMenu(), "core-chatgpt:gpt-current") {
		t.Fatal("failed refresh lost prior selection/catalog")
	}
}

func TestQuickCatalogCannotPublishAcrossSignIn(t *testing.T) {
	q := &quickSetup{login: quickLogin{ID: "old", State: "connected"}}
	q.runModels = func(context.Context) (quickModelCatalog, error) {
		q.mu.Lock()
		q.login = quickLogin{ID: "new", State: "connected"}
		q.mu.Unlock()
		return quickModelCatalog{Models: []quickModelOption{{ID: "old-account-model", Available: true}}}, nil
	}
	if q.refreshModels(context.Background()) == nil || q.models != nil {
		t.Fatal("old account catalog published after sign-in changed")
	}
}
