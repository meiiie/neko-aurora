package aurora

import (
	"strings"
	"testing"
)

func TestCorePromptKeepsCurrentRequestAfterSystemNudge(t *testing.T) {
	for _, tail := range []string{planNudge, finalNudge, "KẾT QUẢ CÔNG CỤ: fixture result"} {
		prompt := corePrompt([]ChatMsg{
			{Role: "system", Content: "Aurora policy"},
			{Role: "user", Content: "Earlier request"},
			{Role: "assistant", Content: "Earlier response"},
			{Role: "user", Content: "Open the supplied pages"},
			{Role: "system", Content: tail},
		})
		if !strings.Contains(prompt, "Yêu cầu hiện tại của Darren:\nOpen the supplied pages") || strings.Count(prompt, "Open the supplied pages") != 1 {
			t.Fatal("a trailing control message hid or duplicated the current owner request")
		}
		if !strings.Contains(prompt, "Darren: Earlier request") || !strings.Contains(prompt, "Bạn: Earlier response") || !strings.Contains(prompt, tail) {
			t.Fatal("conversation history or the control message was lost")
		}
	}
}

func TestCorePromptKeepsHostToolsAvailableAndPeerDataUntrusted(t *testing.T) {
	peer := `<aurora-peer-context untrusted="true">fixture peer content</aurora-peer-context>`
	for _, input := range []string{"Read the supplied page", peer} {
		prompt := corePrompt([]ChatMsg{{Role: "system", Content: systemPrompt}, {Role: "user", Content: input}, {Role: "system", Content: actNudge}})
		if strings.Contains(prompt, "không gọi công cụ") || !strings.Contains(prompt, "in dòng TOOL với JSON") || !strings.Contains(prompt, "Không tự thực thi công cụ của runtime Core") {
			t.Fatal("the transport must allow host directives without enabling native Core tools")
		}
		if input == peer && strings.Contains(prompt, "Yêu cầu hiện tại của Darren:\n"+peer) {
			t.Fatal("peer context was promoted to an owner request")
		}
	}
}
