package aurora

import (
	"strings"
	"testing"
	"time"
)

func TestMemFact(t *testing.T) {
	for in, want := range map[string]string{"KHÔNG": "", " không. ": "", "`Chủ nhân muốn được gọi là anh Minh.`\nthêm": "Chủ nhân muốn được gọi là anh Minh.", "": ""} {
		if g := memFact(in); g != want {
			t.Fatalf("%q: %q", in, g)
		}
	}
}

func TestLastingGate(t *testing.T) {
	if !lasting.MatchString("Từ giờ gọi mình là Minh nhé") || !lasting.MatchString("I prefer short answers") {
		t.Fatal("missed lasting")
	}
	if lasting.MatchString("Tóm tắt bài báo này giúp mình") {
		t.Fatal("false gate")
	}
}

func TestClockNote(t *testing.T) {
	n := clockNote(time.Date(2026, 10, 23, 20, 0, 0, 0, time.UTC)) // 03:00 next day in VN
	if !strings.Contains(n, "24/10/2026") || !strings.Contains(n, "Thứ bảy") {
		t.Fatal(n)
	}
}

func TestMemoryJournalEditTeam(t *testing.T) {
	m := openMemory(t.TempDir())
	m.RememberBy("b1", "Darren thích bảng", "bot")
	if err := m.Edit("b1", 0, "Darren thích bảng ngắn", "owner"); err != nil {
		t.Fatal(err)
	}
	if m.Edit("b1", 0, "token là sk-123", "owner") == nil {
		t.Fatal("secret edit accepted")
	}
	m.RememberBy(teamKey, "Gọi chủ nhân là anh Darren", "review")
	m.ForgetBy("b1", 0, "owner")
	j := m.Journal("b1", 10)
	if len(j) != 4 || j[0].Op != "delete" || j[0].Before != "Darren thích bảng ngắn" || j[2].Before != "Darren thích bảng" || j[1].Bot != teamKey {
		t.Fatalf("%+v", j)
	}
	if len(m.Journal("b2", 10)) != 1 { // other bots see only team changes
		t.Fatal("journal scope")
	}
	if !strings.Contains(teamPrompt(m.List(teamKey)), "anh Darren") {
		t.Fatal("team prompt")
	}
}

func TestMemoryDirectiveTeam(t *testing.T) {
	d := ParseDirectives("ok\nMEMORY: {\"text\":\"Darren dùng tiếng Việt\",\"scope\":\"team\"}")
	if !d.MemoryTeam || d.Memory == "" {
		t.Fatal(d)
	}
}
