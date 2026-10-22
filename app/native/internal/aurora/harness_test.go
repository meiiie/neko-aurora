package aurora

import (
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestParseMultiToolAndMemory(t *testing.T) {
	d := ParseDirectives("Mình tra ba nơi.\nTOOL: {\"name\":\"web_search\",\"q\":\"a\"}\nTOOL: {\"name\":\"web_search\",\"q\":\"b\"}\nTOOL: {\"name\":\"gold_price\"}\nTOOL: {\"name\":\"web_search\",\"q\":\"d\"}\nMEMORY: {\"text\":\"Darren thích bảng ngắn\"}")
	if len(d.Tools) != 3 || d.Tools[1].Q != "b" || d.Memory != "Darren thích bảng ngắn" || d.Text != "Mình tra ba nơi." {
		t.Fatalf("%+v", d)
	}
	if visibleText("ok\nMEMORY: {\"text\":\"x\"}") != "ok" {
		t.Fatal("memory line leaked into visible text")
	}
}

func TestTruncateAndBudget(t *testing.T) {
	long := strings.Repeat("á", 5000) // 2 bytes per rune
	if got := truncateResult(long, 1001); !strings.HasSuffix(got, "chỉ đọc phần đầu]") || !strings.HasPrefix(got, "á") {
		t.Fatal("truncate")
	}
	blocks := []string{strings.Repeat("x", 20000), strings.Repeat("y", 10000)}
	out := budgetBlocks(blocks, 24000)
	if total := len(out[0]) + len(out[1]); total > 24100 || out[1] != blocks[1] {
		t.Fatalf("budget total=%d newest kept=%v", total, out[1] == blocks[1])
	}
}

func TestCompactHistory(t *testing.T) {
	msgs := []ChatMsg{{Role: "system", Content: "sys"}}
	for i := 0; i < 20; i++ {
		msgs = append(msgs, ChatMsg{"user", "câu hỏi " + itoa(i)}, ChatMsg{"assistant", strings.Repeat("dài ", 800)})
	}
	out := compactHistory(msgs)
	users := 0
	for _, m := range out {
		if m.Role == "user" {
			users++
		}
	}
	if users != keepFullTurns || out[1].Role != "system" || !strings.Contains(out[1].Content, "câu hỏi 0") {
		t.Fatalf("users=%d first=%q", users, out[1].Content[:40])
	}
	if len(out[2+1].Content) > maxOldReply+100 {
		t.Fatal("old reply not capped")
	}
	if last := out[len(out)-1]; len(last.Content) < 3000 {
		t.Fatal("recent reply should stay whole")
	}
}

func TestMemoryStore(t *testing.T) {
	dir := t.TempDir()
	m := openMemory(dir)
	if !m.Remember("b1", "Darren ở Hải Phòng") || m.Remember("b1", "darren ở hải phòng") {
		t.Fatal("dedupe")
	}
	if m.Remember("b1", "mật khẩu gmail là abc") {
		t.Fatal("secrets must never be memorised")
	}
	m2 := openMemory(dir)
	if l := m2.List("b1"); len(l) != 1 || l[0].Text != "Darren ở Hải Phòng" {
		t.Fatalf("persist %+v", l)
	}
	if !strings.Contains(memoryPrompt(m2.List("b1")), "Hải Phòng") || memoryPrompt(nil) != "" {
		t.Fatal("prompt")
	}
	if !m2.Forget("b1", 0) || len(m2.List("b1")) != 0 || m2.Forget("b1", 3) {
		t.Fatal("forget")
	}
	var nilStore *memStore
	if nilStore.Remember("b", "x") || nilStore.List("b") != nil || nilStore.Forget("b", 0) {
		t.Fatal("nil store")
	}
}

func TestBackoffGrows(t *testing.T) {
	if b0, b1 := backoff(0), backoff(1); b0 < retryDelay || b1 < 2*retryDelay || b1 > 3*retryDelay {
		t.Fatalf("%v %v", b0, b1)
	}
}

func TestDoingShowsPlanStep(t *testing.T) {
	r := &ledger.DirectRun{Plan: []ledger.DirectStep{{Title: "Tra giá", Status: "done"}, {Title: "So sánh", Status: "running"}, {Title: "Viết", Status: "pending"}}}
	if got := doingOf(r); got != "Bước 2/3 · So sánh" {
		t.Fatal(got)
	}
}

func TestAbsorbedNoteOnce(t *testing.T) {
	s := &Service{steered: map[string]string{"q1": "r0"}}
	if s.absorbedNote("q1") == "" || s.absorbedNote("q1") != "" || s.absorbedNote("zz") != "" {
		t.Fatal("absorbed note must fire exactly once for a steered request")
	}
}
