package aurora

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nekoaurora.local/native/internal/computer"
	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/vault"
)

func quietSvc(t *testing.T) *Service {
	t.Helper()
	st, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	v, _ := vault.Open(t.TempDir())
	pc, err := computer.New(t.TempDir(), computer.NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	return &Service{L: st, V: v, PC: pc, running: map[string]bool{}, Now: time.Now} // no turn runner: the test drives the ledger
}

func TestVerifyClaimsRewritesMissingFiles(t *testing.T) {
	svc := quietSvc(t)
	if svc.PC == nil {
		t.Skip("computers off")
	}
	bot := firstBot(svc)
	if err := svc.PC.WriteFile(bot, "deck/talk.pptx", "PK-fake"); err != nil {
		t.Fatal(err)
	}
	link := func(n, p string) string {
		return fmt.Sprintf("[%s](/app/api/agents/%s/computer/download?path=%s)", n, bot, p)
	}
	txt := "Xong! Link tải: " + link("deck", "deck/talk.pptx") + " và " + link("video", "deck/preview.mp4")
	out, bad := svc.verifyClaims(bot, txt, true)
	if len(bad) != 1 || bad[0] != "deck/preview.mp4" || !strings.Contains(out, link("deck", "deck/talk.pptx")) || strings.Contains(out, "preview.mp4)") {
		t.Fatalf("%v %q", bad, out)
	}
	// /home/bot prefix and another bot's link
	if _, bad := svc.verifyClaims(bot, link("d", "/home/bot/deck/talk.pptx"), true); len(bad) != 0 {
		t.Fatal(bad)
	}
	if _, bad := svc.verifyClaims(bot, "[x](/app/api/agents/other/computer/download?path=deck/talk.pptx)", false); len(bad) != 1 {
		t.Fatal("other bot's file accepted")
	}
	// "done" with no verified file at all (live run 2: "Xong! Link tải cho bạn")
	if _, bad := svc.verifyClaims(bot, "Đã tạo xong pptx. Xong! Link tải cho bạn: Bien_Thuy_Ngan.pptx", true); len(bad) == 0 {
		t.Fatal("unbacked done-claim passed")
	}
	if _, bad := svc.verifyClaims(bot, "Mình sẽ dựng file pptx 8 slide.", true); len(bad) != 0 {
		t.Fatal("plan sentence flagged", bad)
	}
}

func TestPlanIntroDropsImaginedRun(t *testing.T) {
	in := "Mình làm ngay nhé — giờ mình dựng file pptx. Trước tiên mình viết script tạo slide: \n\nSau đó chạy: \nĐã tạo xong pptx. Xong! Link tải cho bạn:"
	if got := planIntro(in); strings.Contains(got, "xong") || strings.Contains(got, "Link") || got == "" {
		t.Fatalf("%q", got)
	}
	if got := planIntro("Xong! Link tải: a.pptx"); got != "Mình làm theo kế hoạch dưới đây." {
		t.Fatalf("%q", got)
	}
	if got := planIntro("Mình chia việc làm 3 bước."); got != "Mình chia việc làm 3 bước." {
		t.Fatal(got)
	}
}

func TestCommandQueueIsStrictFIFO(t *testing.T) {
	svc := &Service{}
	var mu sync.Mutex
	var order []int
	running := 0
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		i := i
		svc.enqueueCmd("b", func() {
			defer wg.Done()
			mu.Lock()
			running++
			if running != 1 {
				t.Error("two commands at once")
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			order = append(order, i)
			running--
			mu.Unlock()
		})
	}
	wg.Wait()
	for i, v := range order {
		if v != i {
			t.Fatalf("out of order: %v", order)
		}
	}
}

func TestBuildFollowUpAndBounds(t *testing.T) {
	r := computer.Result{Cmd: "python3 deck.py", Output: "FileNotFoundError: x", ExitCode: 1}
	if f := buildFollowUp(r, pcState{fixes: 1, runs: 1}); !strings.Contains(f, "mã thoát 1") || !strings.Contains(f, "FileNotFoundError") || !strings.Contains(f, "LỖI (lần 1/6)") || !strings.Contains(f, "computer_run") {
		t.Fatal(f)
	}
	if f := buildFollowUp(r, pcState{fixes: maxFixes, runs: 6}); !strings.Contains(f, "DỪNG") {
		t.Fatal(f)
	}
	if f := buildFollowUp(computer.Result{Cmd: "x"}, pcState{runs: 1}); !strings.Contains(f, "computer_list") || strings.Contains(f, "LỖI") {
		t.Fatal(f)
	}
	svc := &Service{}
	for i := 0; i < maxFixes; i++ {
		if svc.buildExhausted("b") {
			t.Fatal("exhausted early")
		}
		svc.noteRun("b", "x", 2)
	}
	if !svc.buildExhausted("b") {
		t.Fatal("no bound")
	}
	svc.newRequest("b", "", true)
	svc.noteRun("b", "x", 0)
	if svc.buildExhausted("b") || svc.pcSnap("b").fixes != 0 {
		t.Fatal("success must reset the fix count")
	}
}

func TestComputerContextHasCwdFilesAndLastExit(t *testing.T) {
	svc, _ := newSvc(t)
	if svc.PC == nil {
		t.Skip("computers off")
	}
	bot := firstBot(svc)
	svc.PC.WriteFile(bot, "gold/make.py", "print(1)")
	svc.noteWrite(bot, "/home/bot/gold/make.py")
	svc.noteRun(bot, "cd /home/bot/gold_pptx && python3 make.py", 2)
	c := svc.computerContext(bot)
	for _, want := range []string{"/home/bot", "cd không giữ", "File bạn đã ghi gần đây: /home/bot/gold/make.py", "gold/make.py (8 B)", "mã thoát 2", "1/6"} {
		if !strings.Contains(c, want) {
			t.Fatalf("missing %q in %s", want, c)
		}
	}
	if !strings.Contains(svc.computerNote(bot), "File bạn đã ghi gần đây") {
		t.Fatal("context not in the system prompt")
	}
}

// End to end with a real sandbox: the bot's failing command comes back with
// its exit code, it fixes and reruns, an older pending card is superseded,
// and the final "Xong" keeps only the link to the file that exists.
func TestBuildLoopFixRerunVerify(t *testing.T) {
	if computer.NewBwrap().Available() != nil {
		t.Skip("no bwrap")
	}
	var bot string
	var mu sync.Mutex
	var prompts []string
	replies := func(n int) string {
		dl := func(p string) string { return "/app/api/agents/" + bot + "/computer/download?path=" + p }
		return []string{
			"TOOL: {\"name\":\"computer_write_file\",\"path\":\"deck/make.py\",\"content\":\"open('deck/out.pptx','w').write('x')\\nimport sys; sys.exit(3)\"}",
			"TOOL: {\"name\":\"computer_run\",\"cmd\":\"python3 deck/make.py # mail\"}",
			// follow-up after exit 3: fix and rerun
			"TOOL: {\"name\":\"computer_write_file\",\"path\":\"deck/make.py\",\"content\":\"open('deck/out.pptx','w').write('x')\"}",
			"TOOL: {\"name\":\"computer_run\",\"cmd\":\"python3 deck/make.py # mail\"}",
			// follow-up after exit 0: claims a video that was never made
			"Xong! [Slide](" + dl("deck/out.pptx") + ") và [Video](" + dl("deck/preview.mp4") + ")",
			"Đã có file pptx: [Slide](" + dl("deck/out.pptx") + "). Video xem trước chưa làm được.",
		}[min(n, 5)]
	}
	n := 0
	fa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		prompts = append(prompts, string(b))
		rep := replies(n)
		n++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", rep)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	if _, err := svc.PC.Run(context.Background(), "probe", "true"); err != nil {
		t.Skipf("sandbox unusable here: %v", err)
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot = firstBot(svc)
	cardFlow(svc, bot) // card flow
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"viết script python tạo file kết quả trên máy tính rồi chạy"}`)
	waitTask(t, svc, bot)
	approve := func() {
		t.Helper()
		p := svc.view().pending()
		for i := 0; i < 500; i++ {
			if p = svc.view().pending(); len(p) == 1 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if len(p) != 1 {
			t.Fatalf("pending %+v", p)
		}
		post(t, srv.URL+"/app/api/approvals/"+p[0].ID, `{"decision":"approved"}`)
		for i := 0; i < 500; i++ {
			if w, _ := svc.L.Get(p[0].TaskID); w.Direct.Approval.Delivery != nil && w.Direct.Approval.Delivery.Status != "sending" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	approve()
	approve()
	for i := 0; i < 500 && !strings.Contains(lastReply(svc, bot), "chưa làm được"); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	final := lastReply(svc, bot)
	if !strings.Contains(final, "deck/out.pptx") || strings.Contains(final, "preview.mp4") {
		t.Fatalf("final %q", final)
	}
	mu.Lock()
	defer mu.Unlock()
	var sawFail, sawCtx, sawVerify bool
	for _, p := range prompts {
		sawFail = sawFail || strings.Contains(p, "mã thoát 3") && strings.Contains(p, "LỖI (lần 1/6)")
		sawCtx = sawCtx || strings.Contains(p, "File bạn đã ghi gần đây: /home/bot/deck/make.py")
		sawVerify = sawVerify || strings.Contains(p, "deck/preview.mp4") && strings.Contains(p, "KHÔNG có trên máy tính")
	}
	if !sawFail || !sawCtx || !sawVerify {
		t.Fatalf("fail=%v ctx=%v verify=%v", sawFail, sawCtx, sawVerify)
	}
}

func TestNewCommandCardSupersedesOlder(t *testing.T) {
	fa := scriptedAI(t, "TOOL: {\"name\":\"computer_run\",\"cmd\":\"echo 1 > a.txt # mail\"}", "TOOL: {\"name\":\"computer_run\",\"cmd\":\"echo 2 > a.txt # mail\"}")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	if svc.PC == nil || svc.PC.CanRun() != nil {
		t.Skip("no computer")
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	cardFlow(svc, bot) // card flow
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"chạy lệnh ghi 1 vào a.txt"}`)
	waitTask(t, svc, bot)
	first := svc.view().pending()
	if len(first) != 1 {
		t.Fatalf("%+v", first)
	}
	// a follow-up turn (hidden computer- message) raised a newer card
	svc.supersedeCommands(bot, "req-newer")
	if r := post(t, srv.URL+"/app/api/approvals/"+first[0].ID, `{"decision":"approved"}`); r.StatusCode == 200 {
		t.Fatal("superseded card could still be approved")
	}
	old, _ := svc.L.Get(first[0].TaskID)
	if old.Direct.Approval.State != "superseded" || !strings.Contains(old.Direct.Approval.Outcome, "thay bằng lệnh mới") {
		t.Fatalf("%+v", old.Direct.Approval)
	}
	if st, _ := svc.PC.Status(bot); len(st.Log) != 0 {
		t.Fatal("superseded command ran")
	}
}

func TestCopiedToolNoteIsNotAReply(t *testing.T) {
	d := ParseDirectives("(đã gọi công cụ computer_write_file, trạng thái: pending)\n(đã gọi công cụ computer_write_file, trạng thái: pending)")
	if !promiseOnly(d) {
		t.Fatal("copied tool note must trigger the act nudge")
	}
	if got := scrubInternal("Mình viết script.\n(đã gọi công cụ computer_run, trạng thái duyệt: approved)\nXong phần 1."); strings.Contains(got, "đã gọi công cụ") || !strings.Contains(got, "Xong phần 1.") {
		t.Fatalf("%q", got)
	}
}

func TestComputerContextSummarizesFrames(t *testing.T) {
	svc := quietSvc(t)
	bot := firstBot(svc)
	for i := 0; i < 60; i++ {
		svc.PC.WriteFile(bot, fmt.Sprintf("frames/f%02d.png", i), "x")
	}
	svc.PC.WriteFile(bot, "out/deck.pptx", "PK")
	c := svc.computerContext(bot)
	if !strings.Contains(c, "out/deck.pptx") || !strings.Contains(c, "frames/ (60 file") || strings.Contains(c, "f05.png") {
		t.Fatal(c)
	}
	if n := svc.computerNote(bot); !strings.Contains(n, "append_data") || !strings.Contains(n, "pipefail") {
		t.Fatal("video/strict-shell guidance missing")
	}
}
