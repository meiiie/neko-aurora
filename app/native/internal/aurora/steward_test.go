package aurora

import (
	"strings"
	"testing"
	"time"
)

func longChat(n int) []ChatMsg {
	msgs := []ChatMsg{{Role: "system", Content: "sys"}}
	for i := 0; i < n; i++ {
		msgs = append(msgs, ChatMsg{"user", "yêu cầu số " + itoa(i) + " — giữ nguyên văn dòng này\nchi tiết hai"}, ChatMsg{"assistant", "đã làm " + itoa(i) + strings.Repeat(" x", 900)})
	}
	return msgs
}

func TestCheckpointKeepsOwnerVerbatim(t *testing.T) {
	out := compactWith(longChat(20), nil)
	cp := out[1].Content
	if !strings.Contains(cp, "yêu cầu số 0 — giữ nguyên văn dòng này\n  chi tiết hai") || !strings.Contains(cp, "yêu cầu số 11") {
		t.Fatal(cp)
	}
	if strings.Contains(cp, strings.Repeat(" x", 100)) {
		t.Fatal("old replies not digested")
	}
	out = compactWith(longChat(20), &checkpoint{State: "ongoing", Summary: "Đã tạo deck.pptx; còn slide 5"})
	if !strings.Contains(out[1].Content, "CÒN DỞ") || !strings.Contains(out[1].Content, "deck.pptx") || strings.Contains(out[1].Content, "rút gọn tự động") {
		t.Fatal(out[1].Content)
	}
}

func TestCutMovesInBlocks(t *testing.T) {
	// 14..19 turns share one cut (12 old turns), so one summary serves them
	k := func(n int) string { m := longChat(n); c := cutPoint(m[1:]); return checkpointKey("b", m[1:1+c]) }
	if k(14) != k(19) || k(19) == k(20) {
		t.Fatal("cut not block aligned")
	}
	if cutPoint(longChat(13)[1:]) != -1 { // 5 old turns: not a full block yet
		t.Fatal("cut before a full block")
	}
}

func TestParseCheckpoint(t *testing.T) {
	c, ok := parseCheckpoint("```json\n{\"state\":\"done\",\"summary\":\"xong\"}\n```")
	if !ok || c.State != "done" {
		t.Fatal(c)
	}
	if _, ok := parseCheckpoint("không biết"); ok {
		t.Fatal("garbage accepted")
	}
	if c, _ := parseCheckpoint(`{"state":"weird","summary":"a"}`); c.State != "ongoing" {
		t.Fatal(c)
	}
}

func TestUsageReport(t *testing.T) {
	u := openUsage(t.TempDir())
	u.add("bot-a", "chat", 3200, 0)
	u.add("bot-a", "memory", 320, 0)
	u.add("", "brief", 32, 0)
	r := u.report(7, time.Now())
	if tot := r["total"].(usageSum); tot.Calls != 3 || tot.Tokens != 1110 {
		t.Fatal(tot)
	}
	if b := r["bots"].([]usageSum); b[0].Key != "bot-a" || len(b) != 2 {
		t.Fatal(b)
	}
	u2 := openUsage(u.path[:len(u.path)-len("/usage.json")])
	if u2.report(1, time.Now())["total"].(usageSum).Calls != 3 {
		t.Fatal("not persisted")
	}
}
