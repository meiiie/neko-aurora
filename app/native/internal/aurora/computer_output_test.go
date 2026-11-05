package aurora

import (
	"strings"
	"testing"
	"unicode/utf8"

	"nekoaurora.local/native/internal/computer"
)

func TestComputerLongCommandPreservesResultWithinContextBudget(t *testing.T) {
	result := computer.Result{Cmd: "python3 - <<'PY'\n" + strings.Repeat("script\n", 2000), Output: "VALIDATION_PASS\n" + strings.Repeat("tiếng Việt 中文\n", 2000) + "FINAL_INTERACTION_CHECK", ExitCode: 0, Ms: 1200}
	text := untrustedOut + fmtRun(result)
	block := truncateResult(text, maxResultChars)
	for _, want := range []string{"mã thoát 0", "VALIDATION_PASS", "FINAL_INTERACTION_CHECK", "đã cắt bớt phần giữa"} {
		if !strings.Contains(block, want) {
			t.Fatalf("command echo hid %q from model context", want)
		}
	}
	if len(text) > maxResultChars-500 || !utf8.ValidString(text) {
		t.Fatal("result leaves no budget for tool metadata or breaks Unicode")
	}
}

func TestComputerShortCommandKeepsOutputAndFailure(t *testing.T) {
	text := fmtRun(computer.Result{Cmd: "node --check app.js", Output: "SyntaxError: unexpected token", ExitCode: 1})
	if !strings.Contains(text, "mã thoát 1") || !strings.Contains(text, "SyntaxError: unexpected token") || strings.Contains(text, "cắt bớt") {
		t.Fatal("short command lost its failure evidence")
	}
}
