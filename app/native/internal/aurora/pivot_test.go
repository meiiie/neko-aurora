package aurora

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const fontTB = `Traceback (most recent call last):
  File "/home/bot/proj/make_video.py", line %d, in <module>
    frame = slide_image(slide, i, len(prs.slides))
  File "/usr/lib/python3/dist-packages/PIL/ImageFont.py", line 249, in __init__
OSError: cannot open resource '/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf'
[mã thoát 1, 3.3s]`

func TestFailSigIgnoresLineNumbersAndPaths(t *testing.T) {
	a := failSig("run", strings.Replace(fontTB, "%d", "80", 1))
	b := failSig("run", strings.Replace(fontTB, "%d", "112", 1))
	if a != b || !strings.Contains(a, "oserror: cannot open resource") {
		t.Fatalf("%q vs %q", a, b)
	}
	if failSig("run", "ModuleNotFoundError: No module named 'pptx'") == a {
		t.Fatal("different errors share a fingerprint")
	}
}

// Runs 9/10: the same font error was "fixed" over and over.
func TestRepeatedFailurePivotsThenStops(t *testing.T) {
	svc := quietSvc(t)
	if svc.PC == nil {
		t.Skip()
	}
	bot := firstBot(svc)
	svc.newRequest(bot, "quay video xem trước", true)
	if n := svc.failNote(bot, "run", strings.Replace(fontTB, "%d", "1", 1)); n != "" {
		t.Fatal("first failure must not nag:", n)
	}
	if n := svc.failNote(bot, "run", strings.Replace(fontTB, "%d", "2", 1)); !strings.Contains(n, "giả định") || !strings.Contains(n, "ĐỔI cách làm") {
		t.Fatal("2nd repeat: premise nudge expected:", n)
	}
	if svc.buildExhausted(bot) {
		t.Fatal("stopped too early")
	}
	n := svc.failNote(bot, "run", strings.Replace(fontTB, "%d", "3", 1))
	if !strings.Contains(n, "DỪNG") || !strings.Contains(n, "CHOICES") || !svc.buildExhausted(bot) {
		t.Fatal("3rd repeat must stop with options:", n)
	}
	w := startWorkLite(bot, "quay video trên máy tính")
	old := turnBudget
	turnBudget = 0
	defer func() { turnBudget = old }()
	if _, ok := svc.checkpoint(w, time.Now()); ok {
		t.Fatal("a stuck request must not auto-continue")
	}
	svc.newRequest(bot, "thử cách khác cho video", true)
	if svc.buildExhausted(bot) {
		t.Fatal("a new owner message resets the breaker")
	}
}

func TestProbeMP4(t *testing.T) {
	b, err := os.ReadFile("testdata/run8_preview.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if v := checkVideo(bytes.NewReader(b), int64(len(b)), ".mp4"); !v.ok || !strings.Contains(v.why, "324s") {
		t.Fatalf("%+v", v)
	}
	cut := b[:len(b)/2]
	if v := checkVideo(bytes.NewReader(cut), int64(len(cut)), ".mp4"); v.ok {
		t.Fatal("half-written mp4 passed")
	}
	junk := bytes.Repeat([]byte("x"), 5000)
	if v := checkVideo(bytes.NewReader(junk), 5000, ".mp4"); v.ok {
		t.Fatal("junk passed")
	}
}

func xlsx(t *testing.T, rows int) []byte {
	t.Helper()
	var body strings.Builder
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&body, `<row r="%d"><c r="A%d"><v>1</v></c></row>`, i, i)
	}
	return sheetArchive(t, sheetPackage(body.String()))
}

func TestCheckSheet(t *testing.T) {
	ok := func(b []byte, ext string) bool { return checkSheet(bytes.NewReader(b), int64(len(b)), ext).ok }
	if !ok(xlsx(t, 5), ".xlsx") || ok(xlsx(t, 1), ".xlsx") || ok([]byte("PK junk"), ".xlsx") {
		t.Fatal("xlsx")
	}
	if !ok([]byte("a,b\n1,2\n3,4\n"), ".csv") || ok([]byte("a,b\n"), ".csv") || ok([]byte("a,b\n1\n2\n3\n4,5\n"), ".csv") {
		t.Fatal("csv")
	}
}

// A file with the right name is not enough: the item closes on a real check.
func TestChecklistNeedsRealArtifacts(t *testing.T) {
	svc := quietSvc(t)
	if svc.PC == nil {
		t.Skip()
	}
	bot := firstBot(svc)
	svc.newRequest(bot, "viết script python xuất bảng tính xlsx và quay video mp4", true)
	svc.PC.WriteFile(bot, "out/preview.mp4", strings.Repeat("x", 4000))
	svc.PC.WriteFile(bot, "out/data.xlsx", "not a zip")
	svc.PC.WriteFile(bot, "make.py", "print(1)")
	svc.noteWrite(bot, "make.py")
	svc.refreshChecklist(bot)
	if n := len(svc.openItems(bot)); n != 3 {
		t.Fatalf("fake artifacts closed items: open=%d %+v", n, svc.pcSnap(bot).checklist)
	}
	if !strings.Contains(checklistText(svc.pcSnap(bot).checklist), "MP4 hỏng") {
		t.Fatal("reason not shown to the bot")
	}
	mp4, _ := os.ReadFile("testdata/run8_preview.mp4")
	svc.PC.WriteFile(bot, "out/preview.mp4", string(mp4))
	svc.PC.WriteFile(bot, "out/data.xlsx", string(xlsx(t, 3)))
	svc.noteRun(bot, "python3 make.py", 0)
	svc.refreshChecklist(bot)
	if open := svc.openItems(bot); len(open) != 0 {
		t.Fatalf("%+v", open)
	}
	v, _, err := svc.PC.ReadFile(bot, "verify.txt")
	if err != nil || !strings.Contains(v, "✓ video") && !strings.Contains(v, "MP4 324s") {
		t.Fatalf("verify.txt: %q %v", v, err)
	}
}

// Live run 11: a queued hidden follow-up (validator report, checklist,
// premise nudge) was rendered as the owner's own message.
func TestQueuedHiddenTurnNotShownAsUser(t *testing.T) {
	svc := quietSvc(t)
	bot := firstBot(svc)
	if _, _, err := svc.L.Accept(ledgerInput(bot, "computer-resume-x", "[Hệ thống] Lượt trước hết thời gian")); err != nil {
		t.Fatal(err)
	}
	for _, m := range svc.view().messages(bot, 0) {
		if m.Role == "user" && strings.Contains(m.Text, "[Hệ thống]") {
			t.Fatal("hidden turn shown as a user bubble")
		}
	}
}
