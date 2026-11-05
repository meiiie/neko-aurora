package aurora

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQuickTurnKeepsAIUntilNextTurn(t *testing.T) {
	s, srv := quickFixture(t)
	bot := firstBot(s)
	in := map[string]any{"workspaceId": s.workspaceID, "provider": "zai", "model": "glm-fixture", "key": "fixture-turn-old", "expectedSelection": ""}
	quickPost(t, srv, "", in, 200)
	choice := s.selectTurnAI(bot)
	ctx := context.WithValue(context.Background(), turnAIKey{}, choice)
	in["expectedSelection"] = "core-zai:glm-fixture"
	in["provider"], in["model"], in["key"] = "zai-openai", "glm-api-fixture", "fixture-turn-new"
	quickPost(t, srv, "", in, 200)
	for _, tc := range []struct {
		ctx  context.Context
		want string
	}{{ctx, "Core fixture zai"}, {context.Background(), "Core fixture zai-openai"}} {
		out, used, err := s.streamExt(tc.ctx, bot, "chat", []ChatMsg{{"user", "fixture"}}, func(string) {})
		if err != nil || !used || out != tc.want {
			t.Fatal("turn route changed or next turn ignored updated settings", err)
		}
	}
	if runtime, model := choice.identity(s.AI); runtime != "neko-core-cli" || model != "glm-fixture" {
		t.Fatal("receipt identity differs from frozen route")
	}
	if choice.core.key != "fixture-turn-old" {
		t.Fatal("turn credential not retained")
	}
}

func TestQuickTurnKeepsCustomRouteAndRejectsMissingProvider(t *testing.T) {
	s, _ := quickFixture(t)
	bot := firstBot(s)
	seen := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			t.Error("invalid request")
		}
		seen <- in.Model
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"fixture reply\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	s.ext.mu.Lock()
	s.ext.d.Providers = append(s.ext.d.Providers, provider{ID: "fixture", Base: upstream.URL, Models: []string{"first", "second"}})
	s.ext.mu.Unlock()
	if err := s.ext.setBot(bot, botExt{Model: "fixture:first"}); err != nil {
		t.Fatal(err)
	}
	choice := s.selectTurnAI(bot)
	ctx := context.WithValue(context.Background(), turnAIKey{}, choice)
	if err := s.ext.setBot(bot, botExt{Model: "fixture:second"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ctx  context.Context
		want string
	}{{ctx, "first"}, {context.Background(), "second"}} {
		_, used, err := s.streamExt(tc.ctx, bot, "chat", nil, func(string) {})
		if err != nil || !used || <-seen != tc.want {
			t.Fatal("custom turn route changed", err)
		}
	}
	if runtime, model := choice.identity(s.AI); runtime != "direct-http" || model != "first" {
		t.Fatal("custom receipt identity incorrect")
	}
	if err := s.ext.setBot(bot, botExt{Model: "missing:model"}); err != nil {
		t.Fatal(err)
	}
	if _, used, err := s.streamExt(context.Background(), bot, "chat", nil, func(string) {}); !used || err != ErrNoAI {
		t.Fatal("missing provider silently fell back")
	}
}
