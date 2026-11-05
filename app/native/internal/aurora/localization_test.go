package aurora

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestLocalizedImageMarkersMatchCatalogueContract(t *testing.T) {
	for _, lang := range []string{"vi", "en", "zh-Hans", "zh-Hant"} {
		t.Run(lang, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "..", "locales", lang+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var catalogue struct {
				UI map[string]string `json:"ui"`
			}
			if err := json.Unmarshal(data, &catalogue); err != nil {
				t.Fatal(err)
			}
			path := "uploads/Dự-án.jpg"
			marker := strings.ReplaceAll(catalogue.UI["chat.imageAttachment"], "{{name}}", path)
			calls := imageAuto(catalogue.UI["chat.imagePrompt"] + marker)
			if len(calls) != 1 {
				t.Fatalf("localized marker produced %d image calls", len(calls))
			}
			var request toolReq
			if err := json.Unmarshal([]byte(calls[0].arg), &request); err != nil || request.Path != path {
				t.Fatal("localized marker changed the image path")
			}
			if len(imageAuto(strings.ReplaceAll(marker, path, "outside.jpg"))) != 0 {
				t.Fatal("marker accepted a path outside uploads")
			}
		})
	}
}

func TestSnippetLocalizationKeepsModelTextLiteral(t *testing.T) {
	bot := ledger.Bot{ID: "bot-locale", Name: "Cài đặt"}
	for _, tc := range []struct{ status, reply, failure, key string }{
		{"", "", "", "agent.ready"},
		{"completed", "", "", "agent.done"},
		{"completed", "Sẵn sàng nhận việc.", "", ""},
		{"completed", "Đã xong", "", ""},
		{"cancelled", "", "", "agent.stopped"},
		{"failed", "", "ai_not_configured", "agent.setup"},
		{"failed", "", "core_failed", "agent.failed"},
		{"waiting_host", "", "", "agent.queued"},
	} {
		t.Run(tc.status+tc.reply+tc.failure, func(t *testing.T) {
			v := newView(ledger.Snapshot{})
			if tc.status != "" {
				v.byBot[bot.ID] = []ledger.Work{{Status: tc.status, Reply: tc.reply, Failure: tc.failure}}
			}
			a := v.agent(bot, botPrefs{})
			if a.SnipKey != tc.key || a.Name != bot.Name {
				t.Fatalf("snippet key = %q, want %q; name preserved = %v", a.SnipKey, tc.key, a.Name == bot.Name)
			}
			if tc.reply != "" && a.Snip != tc.reply {
				t.Fatal("model-authored snippet changed")
			}
		})
	}
}
