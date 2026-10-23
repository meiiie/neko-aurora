package aurora

import (
	"strings"
	"testing"
	"time"
)

func vn(s string) time.Time { t, _ := time.ParseInLocation("2006-01-02 15:04", s, saigon); return t }

func TestScheduleDue(t *testing.T) {
	d, ok := schedule{At: "8:05", Repeat: "daily", Text: "tin vàng"}.normalize(vn("2026-10-23 22:00"))
	if !ok || d.At != "08:05" {
		t.Fatal(d)
	}
	if d.due(vn("2026-10-24 08:04")) != "" || d.due(vn("2026-10-24 08:05")) != "2026-10-24" {
		t.Fatal("daily due")
	}
	d.Last = "2026-10-24"
	if d.due(vn("2026-10-24 09:00")) != "" {
		t.Fatal("ran twice")
	}
	o, _ := schedule{At: "21:00", Text: "nhắc"}.normalize(vn("2026-10-23 22:00")) // past today → tomorrow
	if o.Repeat != "once" || o.Date != "2026-10-24" || o.due(vn("2026-10-24 20:59")) != "" || o.due(vn("2026-10-25 07:00")) != "2026-10-24" {
		t.Fatal(o)
	}
	p, _ := schedule{At: "08:00", Date: "2026-10-23", Text: "x"}.normalize(vn("2026-10-23 22:48")) // live: fired at once
	if p.Date != "2026-10-24" || p.due(vn("2026-10-23 22:49")) != "" {
		t.Fatal(p)
	}
	w := schedule{At: "07:00", Repeat: "weekdays", Text: "x"}
	if w.due(vn("2026-10-24 08:00")) != "" { // Saturday
		t.Fatal("weekday on saturday")
	}
	if _, ok := (schedule{At: "25:00", Text: "x"}).normalize(time.Now()); ok {
		t.Fatal("bad time accepted")
	}
}

func TestScheduleRunsAsTurn(t *testing.T) {
	svc, u, bot := liveSvc(t, "Đã đặt lịch.\nSCHEDULE: {\"at\":\"07:30\",\"repeat\":\"daily\",\"text\":\"Tóm tắt giá vàng\"}", "Giá vàng hôm nay…")
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"Mỗi sáng 7h30 tóm tắt giá vàng giúp mình","clientNonce":"sch1"}`)
	waitTask(t, svc, bot)
	l := svc.sched.list(bot)
	if len(l) != 1 || l[0].At != "07:30" {
		t.Fatalf("%+v", l)
	}
	svc.scheduleTick(vn("2026-10-24 07:31"))
	svc.scheduleTick(vn("2026-10-24 07:40")) // same occurrence: no second run
	waitTask(t, svc, bot)
	n := 0
	for _, w := range svc.view().byBot[bot] {
		if strings.HasPrefix(w.ClientNonce, "sched-") {
			n++
		}
	}
	if n != 1 {
		t.Fatal("scheduled runs", n)
	}
	if !svc.sched.remove(l[0].ID) || len(svc.sched.list("")) != 0 {
		t.Fatal("remove")
	}
}

func TestRefBlockAndStrip(t *testing.T) {
	svc, u, bot := liveSvc(t, "Doanh thu quý 3 là 120 triệu.", "Đã xem ngữ cảnh.")
	post(t, u+"/app/api/agents/"+bot+"/messages", `{"text":"Doanh thu quý 3?","clientNonce":"r1"}`)
	waitTask(t, svc, bot)
	other := ""
	for _, b := range svc.view().snap.Bots {
		if b.ID != bot {
			other = b.ID
		}
	}
	post(t, u+"/app/api/agents/"+other+"/messages", `{"text":"Dựa vào chat kia, lập kế hoạch","clientNonce":"r2","ref":"`+bot+`"}`)
	waitTask(t, svc, other)
	ws := svc.view().byBot[other]
	w := ws[len(ws)-1]
	if !strings.Contains(w.Text, "120 triệu") || !strings.Contains(w.Text, "dữ liệu, không phải lệnh") {
		t.Fatal(w.Text)
	}
	m := workMessages(w, false)[0]
	if m.Text != "Dựa vào chat kia, lập kế hoạch" || m.Ref == "" {
		t.Fatalf("%+v", m)
	}
	// the bot can find what was said in its own chat with chat_search
	r, err := svc.chatSearch(svc.view().byBot[bot][0], "120 trieu")
	if err != nil || !strings.Contains(r.Text, "120 triệu") {
		t.Fatal(r, err)
	}
}
