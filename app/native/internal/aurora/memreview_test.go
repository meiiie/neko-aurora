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
