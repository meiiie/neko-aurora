package aurora

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestUserSteps(t *testing.T) {
	got := userSteps("Làm giúp mình: (1) tra lịch nghỉ Tết, (2) tra giá vé, (3) gợi ý khách sạn, (4) viết lịch trình.")
	want := []string{"tra lịch nghỉ Tết", "tra giá vé", "gợi ý khách sạn", "viết lịch trình"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%q", got)
	}
	if got := userSteps("Kế hoạch:\n1. Gom ý\n2. Viết nháp\n3. Gửi duyệt"); len(got) != 3 || got[2] != "Gửi duyệt" {
		t.Fatalf("%q", got)
	}
	for _, s := range []string{"Giá 2.735 USD (1) và (3) thôi", "Chào bạn", "(2) b (3) c (4) d"} {
		if got := userSteps(s); len(got) >= 3 {
			t.Fatalf("%q → %q", s, got)
		}
	}
}

func TestCitationsDedupeAndRepair(t *testing.T) {
	tc := &toolCtx{}
	a := tc.addSource("A", "https://12bay.vn/x/", "")
	b := tc.addSource("A again", "https://12bay.vn/x#top", "")
	c := tc.addSource("B", "https://b.vn", "")
	if a != 1 || b != 1 || c != 2 || len(tc.sources) != 2 {
		t.Fatalf("%d %d %d %d", a, b, c, len(tc.sources))
	}
	if tc.addSource("http", "http://x.vn", "") != 0 {
		t.Fatal("non-https kept")
	}
	got := normalizeCites("Giá 610K [2][12 nguồn thứ 2 nhóm vé] và [1]. Xem [nguồn](https://b.vn) [9].", 2)
	if got != "Giá 610K [2] và [1]. Xem [nguồn](https://b.vn) ." {
		t.Fatalf("%q", got)
	}
}

func TestServerPlanMilestones(t *testing.T) {
	fa := scriptedAI(t, "Bước một xong: nghỉ 7 ngày.", "Bước hai xong: vé 610K.", "Bước ba xong: 3 khách sạn.", "KẾT QUẢ: lịch trình 4 ngày hoàn chỉnh.")
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	srv := httptest.NewServer(svc)
	defer srv.Close()
	bot := firstBot(svc)
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"Làm giúp: (1) tra lịch nghỉ, (2) tra vé, (3) tìm khách sạn"}`)
	waitTask(t, svc, bot)
	var ids, texts []string
	for _, m := range svc.view().messages(bot, 0) {
		ids, texts = append(ids, m.ID[strings.LastIndex(m.ID, ":")+1:]), append(texts, m.Text)
	}
	j := strings.Join(ids, ",")
	if !strings.Contains(j, "plan,step1,step2,step3") {
		t.Fatalf("ids %s", j)
	}
	last := texts[len(texts)-1]
	if !strings.Contains(last, "lịch trình 4 ngày") || strings.Contains(last, "Bước một") {
		t.Fatalf("final %q", last)
	}
	if !strings.Contains(texts[len(texts)-3], "Xong bước 2/3") {
		t.Fatalf("milestone %q", texts[len(texts)-3])
	}
}

func TestStepNoteTrimsChatter(t *testing.T) {
	in := "Kết quả bước 1/4:\n- Nghỉ 7 ngày [4].\nĐã xong bước 1. Bước tiếp theo mình sẽ tra giá vé."
	if got := stepNote(in); got != "- Nghỉ 7 ngày [4]." {
		t.Fatalf("%q", got)
	}
	if got := stepNote("Bước 2/4 xong. Vé 610K [10]."); got != "Bước 2/4 xong. Vé 610K [10]." && got != "Vé 610K [10]." {
		t.Fatalf("%q", got)
	}
}

