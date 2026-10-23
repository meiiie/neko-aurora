package aurora

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaimsFindNarratedToolWorkOnly(t *testing.T) {
	for _, c := range []string{
		"Mình viết script render ngay.\n\n[đã ghi render_video.py (dựng 8 slide bằng Pillow)]",
		"(đã gọi công cụ computer_write_file, trạng thái: pending)",
		"Xong (đã chạy lệnh python3 build.py)",
		"Done [wrote build.py and ran it]",
	} {
		if len(claims(c)) != 1 {
			t.Errorf("missed claim in %q", c)
		}
	}
	for _, c := range []string{
		"Hg-198 (80 proton · 118 neutron) bắt neutron",
		"Tải: [Hg_to_Au.pptx](/app/api/agents/b/computer/download?path=Hg_to_Au.pptx)",
		"Mình sẽ dựng file (8 slide) rồi chạy lệnh.",
		"(xem file slides.pptx)",
	} {
		if cl := claims(c); len(cl) != 0 {
			t.Errorf("false claim %v in %q", cl, c)
		}
	}
	if got := stripNarration("Mình viết script.\n\n[đã ghi render_video.py (x)]\n\nXong phần 1."); strings.Contains(got, "đã ghi") || !strings.Contains(got, "Xong phần 1.") {
		t.Fatalf("%q", got)
	}
}

func TestMixedScripts(t *testing.T) {
	got := mixedScripts("Lượng tạo ra: cực nhỏ (cỡ микro-gam), β⁺ phân rã, Hà Nội 北京")
	if strings.Join(got, ",") != "микro-gam,北京" {
		t.Fatal(got)
	}
}

// The model narrates a file write instead of calling the tool: the claim is
// stripped, it is asked for directives only, and the file really exists.
func TestNarratedWriteForcesRealToolCall(t *testing.T) {
	fa := scriptedAI(t,
		"Mình viết script render ngay.\n\n[đã ghi render.py (dựng khung hình rồi ghép mp4)]",
		"TOOL: {\"name\":\"computer_write_file\",\"path\":\"render.py\",\"content\":\"print('render')\"}",
		"Mình đã ghi render.py, giờ chạy nó.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	if svc.PC == nil {
		t.Skip("computers off")
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"viết script python render video trên máy tính"}`)
	waitTask(t, svc, bot)
	if txt, _, err := svc.PC.ReadFile(bot, "render.py"); err != nil || !strings.Contains(txt, "render") {
		t.Fatalf("file not written: %v", err)
	}
	for _, m := range svc.view().messages(bot, 0) {
		if m.Role == "agent" && strings.Contains(m.Text, "[đã ghi") {
			t.Fatalf("narration shown: %q", m.Text)
		}
	}
}

// A deliverable that contradicts the research notes is corrected before it
// is written (live run 4: "vàng phóng xạ Au-197" in the 1941 slide).
func TestDeliverableReviewedAgainstResearch(t *testing.T) {
	slide := func(iso string) string {
		return "from pptx import Presentation\nslides = [\n  ('Thí nghiệm 1941', 'Sherr, Bainbridge và Anderson bắn neutron nhanh vào thủy ngân, tạo được vàng phóng xạ " + iso + " với lượng cực nhỏ, chứng minh chuyển hoá hạt nhân là có thật và đo được chu kỳ bán rã của từng đồng vị.'),\n  ('Kết luận', 'Giả kim thuật đúng đích đến nhưng sai phương pháp; vật lý hạt nhân mới làm được điều đó.'),\n]\n"
	}
	bad, good := slide("Au-197"), slide("Au-198, Au-199, Au-200")
	write, _ := json.Marshal(map[string]string{"name": "computer_write_file", "path": "deck/build.py", "content": bad})
	fa := scriptedAI(t, "TOOL: "+string(write), "FIXED: Au-197 → Au-198, Au-199, Au-200 (theo ghi chú [1])\n```python\n"+good+"```", "Đã ghi script đã kiểm tra.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	if svc.PC == nil {
		t.Skip("computers off")
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	svc.pcDo(bot, func(st *pcState) {
		st.research = "[1] Sherr et al. 1941 — Phys. Rev. 60, 473\nKẾT QUẢ BƯỚC 1: neutron nhanh (Li+D) bắn Hg tạo 3 đồng vị vàng phóng xạ Au-198, Au-199, Au-200 [1]."
	})
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"viết script python tạo slide trên máy tính"}`)
	waitTask(t, svc, bot)
	txt, _, err := svc.PC.ReadFile(bot, "deck/build.py")
	if err != nil || strings.Contains(txt, "phóng xạ Au-197") || !strings.Contains(txt, "Au-198, Au-199, Au-200") {
		t.Fatalf("not corrected: %v %q", err, txt)
	}
}

func TestFixMixedScriptsDeterministic(t *testing.T) {
	vi := strings.Repeat("Giả kim sư tìm hòn đá triết gia để biến thủy ngân thành vàng. ", 3)
	got, fixed := fixMixedScripts(vi + "Lượng: cỡ микro-gam; tìm \"đan石 tạo vàng\"; phân rã β⁺ và γ; Hg-198.")
	if strings.ContainsAny(got, "микро石") || !strings.Contains(got, "mikro-gam") || !strings.Contains(got, "\"đan tạo vàng\"") || !strings.Contains(got, "β⁺ và γ") || len(fixed) != 2 {
		t.Fatalf("%q %v", got, fixed)
	}
	if got, fixed := fixMixedScripts("print('北京')"); got != "print('北京')" || fixed != nil {
		t.Fatal("non-Vietnamese text must be left alone")
	}
}
