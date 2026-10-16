package historyexport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func fixture() History {
	return History{Version: 1, ConversationID: "conversation-bot-meiiie", Title: "Lịch sử 猫", BotID: "bot-meiiie", Archived: true, AtSequence: 42, Messages: []Message{{RequestID: "request-one", BotID: "bot-meiiie", CreatedAt: "2026-10-12T01:00:00.123Z", Text: "Xin chào\n\t<script>literal</script> `ticks` \"quote\" \\ path", Reply: "Reply 猫\r\nsecond line", Status: "completed", Files: []File{{ID: "artifact-one", Name: "báo cáo.txt", SHA256: strings.Repeat("a", 64), Bytes: 12, DownloadPath: "/v1/channels/conversation-bot-meiiie/artifacts/artifact-one"}}}}}
}

func reject(t *testing.T, history History, format string, want ErrorKind) {
	t.Helper()
	encoded, err := Encode(history, format)
	var rejected *Error
	if !errors.As(err, &rejected) || rejected.Kind != want || encoded != nil {
		t.Fatalf("want typed %q with nil output; got bytes=%d err=%v", want, len(encoded), err)
	}
}

func TestEncodeJSONPreservesLiteralUnicodeAndEscapes(t *testing.T) {
	h := fixture()
	encoded, err := Encode(h, "json")
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(encoded) || !bytes.Contains(encoded, []byte(`\u003cscript\u003e`)) || !bytes.Contains(encoded, []byte(`\n\t`)) || !bytes.Contains(encoded, []byte(`\"quote\"`)) || bytes.Contains(encoded, []byte("<script>")) {
		t.Fatal("JSON output must preserve data through standard JSON escaping")
	}
	var roundTrip History
	if err := json.Unmarshal(encoded, &roundTrip); err != nil || !reflect.DeepEqual(h, roundTrip) {
		t.Fatalf("JSON round-trip differs: %v", err)
	}
	second, err := Encode(h, "json")
	if err != nil || !bytes.Equal(encoded, second) {
		t.Fatal("JSON output must be deterministic")
	}
}

func TestEncodeTextPreservesScriptAndBackticksAsPlainData(t *testing.T) {
	h := fixture()
	h.Messages[0].Text += "\n```literal code fence remains data```\n<!DOCTYPE html>"
	encoded, err := Encode(h, "text")
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{h.Title, h.Messages[0].Text, h.Messages[0].Reply, h.Messages[0].Files[0].Name, "SHA-256: " + h.Messages[0].Files[0].SHA256, "separate authenticated download"} {
		if !bytes.Contains(encoded, []byte(literal)) {
			t.Fatalf("plain text output lost literal value of length %d", len(literal))
		}
	}
	if !bytes.HasPrefix(encoded, []byte("Neko Bot conversation history\n")) {
		t.Fatal("plain text output must start with the literal text format header")
	}
}

func TestEncodeJSONHasOnlyExplicitPublicDTOFields(t *testing.T) {
	projected := struct {
		Public                                      History
		PrivateContext, Account, Nonce, Instruction string
	}{fixture(), "private-context-canary", "account-canary", "nonce-canary", "internal-stage-canary"}
	encoded, err := Encode(projected.Public, "json")
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{projected.PrivateContext, projected.Account, projected.Nonce, projected.Instruction} {
		if bytes.Contains(encoded, []byte(canary)) {
			t.Fatal("fields outside the supplied public DTO appeared in export")
		}
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, result, "version", "conversationId", "title", "botId", "archived", "atSequence", "messages")
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(result["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, messages[0], "requestId", "botId", "createdAt", "text", "reply", "status", "files")
	var files []map[string]json.RawMessage
	if err := json.Unmarshal(messages[0]["files"], &files); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, files[0], "id", "name", "sha256", "bytes", "downloadPath")
}

func assertKeys(t *testing.T, got map[string]json.RawMessage, names ...string) {
	t.Helper()
	if len(got) != len(names) {
		t.Fatalf("unexpected DTO field count: got %d want %d", len(got), len(names))
	}
	for _, name := range names {
		if _, exists := got[name]; !exists {
			t.Fatalf("DTO field %s missing", name)
		}
	}
}

func TestEncodeNormalizesEmptyCollectionsWithoutMutatingInput(t *testing.T) {
	h := fixture()
	h.Messages[0].Files = nil
	encoded, err := Encode(h, "json")
	if err != nil || !bytes.Contains(encoded, []byte(`"files": []`)) || h.Messages[0].Files != nil {
		t.Fatal("files must encode as [] without mutating input")
	}
	h.Messages = nil
	encoded, err = Encode(h, "json")
	if err != nil || !bytes.Contains(encoded, []byte(`"messages": []`)) || h.Messages != nil {
		t.Fatal("messages must encode as [] without mutating input")
	}
}

func TestEncodeRejectsInvalidFieldsAndControls(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*History)
	}{
		{"version", func(h *History) { h.Version = 2 }},
		{"conversation_id", func(h *History) { h.ConversationID = "../outside" }},
		{"bot_id", func(h *History) { h.BotID = "" }},
		{"request_id", func(h *History) { h.Messages[0].RequestID = "req\nheader" }},
		{"message_bot", func(h *History) { h.Messages[0].BotID = strings.Repeat("b", 129) }},
		{"timestamp", func(h *History) { h.Messages[0].CreatedAt = "tomorrow" }},
		{"status", func(h *History) { h.Messages[0].Status = "running\rheader" }},
		{"title_control", func(h *History) { h.Title = "title\nheader" }},
		{"invalid_utf8_title", func(h *History) { h.Title = string([]byte{0xff}) }},
		{"invalid_utf8_text", func(h *History) { h.Messages[0].Text = string([]byte{0xff}) }},
		{"nul", func(h *History) { h.Messages[0].Text = "nul\x00data" }},
		{"escape", func(h *History) { h.Messages[0].Reply = "\x1b[31m" }},
		{"c1_control", func(h *History) { h.Messages[0].Reply = "\u0085" }},
		{"file_id", func(h *History) { h.Messages[0].Files[0].ID = "" }},
		{"filename_control", func(h *History) { h.Messages[0].Files[0].Name = "name\tdata" }},
		{"hash", func(h *History) { h.Messages[0].Files[0].SHA256 = strings.Repeat("g", 64) }},
		{"uppercase_hash", func(h *History) { h.Messages[0].Files[0].SHA256 = strings.Repeat("A", 64) }},
		{"negative_size", func(h *History) { h.Messages[0].Files[0].Bytes = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := fixture()
			test.mutate(&h)
			for _, format := range []string{"json", "text"} {
				reject(t, h, format, InvalidHistory)
			}
		})
	}
}

func TestEncodeRejectsExternalOrAmbiguousDownloadPaths(t *testing.T) {
	for _, p := range []string{"https://example.invalid/file", "//example.invalid/file", "relative/file", "/v1/../private", "/v1//file", `/v1\file`, "/v1/file?token=literal", "/v1/file#frag", "/v1/%2e%2e/file", "/v1/C:/file", "/v1/file\nheader"} {
		h := fixture()
		h.Messages[0].Files[0].DownloadPath = p
		reject(t, h, "json", InvalidHistory)
	}
	h := fixture()
	h.Messages[0].Files[0].DownloadPath = ""
	if _, err := Encode(h, "json"); err != nil {
		t.Fatal("empty metadata-only download path must be allowed", err)
	}
}

func TestEncodeRejectsDuplicateRequestsAndFiles(t *testing.T) {
	h := fixture()
	h.Messages = append(h.Messages, h.Messages[0])
	reject(t, h, "json", InvalidHistory)
	h = fixture()
	h.Messages[0].Files = append(h.Messages[0].Files, h.Messages[0].Files[0])
	reject(t, h, "text", InvalidHistory)
	h = fixture()
	copyMessage := h.Messages[0]
	copyMessage.RequestID = "request-two"
	h.Messages = append(h.Messages, copyMessage)
	reject(t, h, "json", InvalidHistory)
}

func TestEncodeMessageQuota(t *testing.T) {
	h := fixture()
	h.Messages = nil
	for i := 0; i < 500; i++ {
		h.Messages = append(h.Messages, Message{RequestID: fmt.Sprintf("request-%d", i), BotID: "bot-meiiie", CreatedAt: "2026-10-12T00:00:00Z", Text: "猫", Status: "completed"})
	}
	for _, format := range []string{"json", "text"} {
		if _, err := Encode(h, format); err != nil {
			t.Fatal("500 messages must be supported", err)
		}
	}
	h.Messages = append(h.Messages, Message{RequestID: "request-500"})
	reject(t, h, "json", LimitExceeded)
}

func TestEncodeRejectsFieldAndFileCountBudgets(t *testing.T) {
	for _, mutate := range []func(*History){
		func(h *History) { h.Title = strings.Repeat("x", 257) },
		func(h *History) { h.Messages[0].Text = strings.Repeat("猫", 65536/3+1) },
		func(h *History) { h.Messages[0].Reply = strings.Repeat("x", 65537) },
		func(h *History) { h.Messages[0].Files[0].Name = strings.Repeat("x", 257) },
		func(h *History) { h.Messages[0].Files[0].Bytes = 1<<30 + 1 },
		func(h *History) { h.Messages[0].Files[0].DownloadPath = "/" + strings.Repeat("x", 512) },
		func(h *History) { h.Messages[0].Files = make([]File, 65) },
	} {
		h := fixture()
		mutate(&h)
		reject(t, h, "json", LimitExceeded)
	}
}

func TestEncodeEnforcesOutputQuotaAfterJSONEscaping(t *testing.T) {
	h := fixture()
	h.Messages = nil
	for i := 0; i < 9; i++ {
		h.Messages = append(h.Messages, Message{RequestID: fmt.Sprintf("request-%d", i), BotID: "bot-meiiie", CreatedAt: "2026-10-12T00:00:00Z", Text: strings.Repeat("\n", 65536), Status: "completed"})
	}
	reject(t, h, "json", LimitExceeded)
	encoded, err := Encode(h, "text")
	if err != nil || len(encoded) > 1<<20 {
		t.Fatal("plain text data under quota should remain accepted", err)
	}
	for i := 9; i < 18; i++ {
		h.Messages = append(h.Messages, Message{RequestID: fmt.Sprintf("request-%d", i), BotID: "bot-meiiie", CreatedAt: "2026-10-12T00:00:00Z", Text: strings.Repeat("x", 65536), Status: "completed"})
	}
	reject(t, h, "text", LimitExceeded)
}

func TestEncodeRejectsUnknownFormatWithoutPartialOutput(t *testing.T) {
	for _, format := range []string{"", "JSON", "markdown", "html"} {
		reject(t, fixture(), format, UnsupportedFormat)
	}
}

func TestEncodeAcceptsExactOutputQuotaAndRejectsOneByteMore(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		h := fixture()
		h.Messages = nil
		for i := 0; i < 16; i++ {
			text := strings.Repeat("x", 65536)
			if i == 15 {
				text = ""
			}
			h.Messages = append(h.Messages, Message{RequestID: fmt.Sprintf("request-%d", i), BotID: "bot-meiiie", CreatedAt: "2026-10-12T00:00:00Z", Text: text, Status: "completed"})
		}
		base, err := Encode(h, format)
		if err != nil {
			t.Fatal(err)
		}
		remaining := (1 << 20) - len(base)
		if remaining < 1 || remaining >= 65536 {
			t.Fatal("exact-quota fixture did not leave bounded text space")
		}
		h.Messages[15].Text = strings.Repeat("x", remaining)
		encoded, err := Encode(h, format)
		if err != nil || len(encoded) != 1<<20 {
			t.Fatalf("exact output quota rejected for %s: bytes=%d err=%v", format, len(encoded), err)
		}
		h.Messages[15].Text += "x"
		reject(t, h, format, LimitExceeded)
	}
}

func TestEncodeRejectsCrossConversationAndCrossFilePointers(t *testing.T) {
	for _, p := range []string{
		"/v1/channels/conversation-other/artifacts/artifact-one",
		"/v1/channels/conversation-bot-meiiie/artifacts/artifact-other",
		"/v1/artifacts/artifact-one",
	} {
		h := fixture()
		h.Messages[0].Files[0].DownloadPath = p
		reject(t, h, "json", InvalidHistory)
	}
}

func TestEncodeRejectsNonRFC3339TimestampsAndEmptyStatusIdentifiers(t *testing.T) {
	for _, timestamp := range []string{
		"2026-10-12T1:00:00Z",
		"2026-10-12T01:00:00,123Z",
		"2026-10-12T01:00:00+24:00",
		"2026-10-12T01:00:00+00:60",
	} {
		t.Run(timestamp, func(t *testing.T) {
			h := fixture()
			h.Messages[0].CreatedAt = timestamp
			reject(t, h, "json", InvalidHistory)
		})
	}
	h := fixture()
	h.Messages[0].Status = "___"
	reject(t, h, "text", InvalidHistory)
}
