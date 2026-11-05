package aurora

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/vault"
)

func TestQuickAPIModelsBelongToCandidateKeyAndSaveRevalidates(t *testing.T) {
	s, srv := quickFixture(t)
	bot := firstBot(s)
	allowed := "glm-current"
	calls := 0
	s.quick.runAPIModels = func(_ context.Context, p quickProvider, key string) (quickModelCatalog, error) {
		calls++
		if p.ID != "zai" {
			t.Fatal("crossed billing route")
		}
		if key != "fixture-new-key" {
			return quickModelCatalog{}, errors.New("core_auth_required")
		}
		return quickModelCatalog{Models: []quickModelOption{{ID: allowed, Label: "GLM live label", Available: true, Transport: "anthropic"}}}, nil
	}
	in := map[string]any{"workspaceId": s.workspaceID, "botId": bot, "provider": "zai", "key": "fixture-new-key"}
	data := quickPost(t, srv, "/models", in, 200)
	if !strings.Contains(string(data), "GLM live label") || strings.Contains(string(data), "fixture-new-key") || s.V.Has(providerKey("core-zai")) || s.quickSelection(bot) != "" {
		t.Fatal("preview changed connection or leaked key")
	}
	p, _ := s.quickProvider("zai")
	if p.ModelSource == "account" {
		t.Fatal("candidate catalog published to other bots")
	}
	in["model"], in["expectedSelection"] = "glm-current", ""
	allowed = "glm-newer"
	quickPost(t, srv, "", in, 400)
	if s.V.Has(providerKey("core-zai")) || s.quickSelection(bot) != "" {
		t.Fatal("rejected selection saved a key or model")
	}
	in["model"] = "glm-newer"
	quickPost(t, srv, "", in, 200)
	if calls != 3 || s.aiViewFor(bot)["modelLabel"] != "GLM live label" {
		t.Fatal("save did not use the checked live catalog")
	}
	if _, err := s.V.Set(vault.Meta{Name: providerKey("core-zai"), Label: "fixture", Kind: "api_key"}, "fixture-other-key"); err != nil {
		t.Fatal(err)
	}
	p, _ = s.quickProvider("zai")
	if p.ModelSource == "account" {
		t.Fatal("old key's models leaked across credential change")
	}
	if s.quickSelection(bot) != "core-zai:glm-newer" {
		t.Fatal("credential change silently replaced selection")
	}
}

func TestQuickAPICatalogFailurePreservesSavedConnection(t *testing.T) {
	s, srv := quickFixture(t)
	bot := firstBot(s)
	quickPost(t, srv, "", map[string]any{"workspaceId": s.workspaceID, "botId": bot, "provider": "zai", "model": "glm-fixture", "key": "fixture-saved", "expectedSelection": ""}, 200)
	s.quick.runAPIModels = func(context.Context, quickProvider, string) (quickModelCatalog, error) {
		return quickModelCatalog{}, errors.New("private-key-and-provider-body")
	}
	in := map[string]any{"workspaceId": s.workspaceID, "botId": bot, "provider": "zai", "key": "fixture-candidate"}
	data := quickPost(t, srv, "/models", in, 200)
	if !strings.Contains(string(data), "core_catalog_unavailable") || strings.Contains(string(data), "private-key") {
		t.Fatal("unsafe or absent error")
	}
	in["model"], in["expectedSelection"] = "glm-fixture", "core-zai:glm-fixture"
	quickPost(t, srv, "", in, 400)
	if s.quickKey("zai") != "fixture-saved" || s.quickSelection(bot) != "core-zai:glm-fixture" {
		t.Fatal("failed key validation replaced connection")
	}
	in["workspaceId"] = "wrong"
	s.quick.runAPIModels = func(context.Context, quickProvider, string) (quickModelCatalog, error) {
		t.Fatal("cross-workspace catalog request")
		return quickModelCatalog{}, nil
	}
	delete(in, "model")
	delete(in, "expectedSelection")
	quickPost(t, srv, "/models", in, 409)
}
