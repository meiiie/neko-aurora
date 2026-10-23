package aurora

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

type ledgerWork = ledger.Work

type tSlide struct {
	titleClr, bg string
	hidden       bool
	entr         bool
	notes        int // words
}

func makeDeck(t *testing.T, slides ...tSlide) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	put := func(n, s string) { w, _ := z.Create(n); w.Write([]byte(s)) }
	ns := `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	for i, s := range slides {
		hid := ""
		if s.hidden {
			hid = ` hidden="1"`
		}
		timing := ""
		if s.entr {
			timing = `<p:timing><p:tnLst><p:par><p:cTn id="1"><p:childTnLst><p:par><p:cTn id="5" presetID="1" presetClass="entr"><p:childTnLst><p:set><p:cBhvr><p:tgtEl><p:spTgt spid="3"/></p:tgtEl></p:cBhvr></p:set></p:childTnLst></p:cTn></p:par></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`
		}
		put(fmt.Sprintf("ppt/slides/slide%d.xml", i+1), `<p:sld `+ns+`><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="`+s.bg+`"/></a:solidFill></p:bgPr></p:bg><p:spTree>`+
			`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/></p:nvSpPr><p:spPr/><p:txBody><a:p><a:r><a:rPr sz="3600"><a:solidFill><a:srgbClr val="`+s.titleClr+`"/></a:solidFill></a:rPr><a:t>Tiêu đề</a:t></a:r></a:p></p:txBody></p:sp>`+
			`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Box"`+hid+`/></p:nvSpPr><p:spPr/><p:txBody><a:p><a:r><a:rPr sz="2000"/><a:t>Nội dung</a:t></a:r></a:p></p:txBody></p:sp>`+
			`</p:spTree></p:cSld>`+timing+`</p:sld>`)
		if s.notes > 0 {
			put(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i+1), `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide`+fmt.Sprint(i+1)+`.xml"/></Relationships>`)
			put(fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i+1), `<p:notes `+ns+`><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="2" name="Notes"/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>`+strings.TrimSpace(strings.Repeat("từ ", s.notes))+`</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>`)
		}
	}
	z.Close()
	return buf.Bytes()
}

func TestValidatePPTX(t *testing.T) {
	good := tSlide{titleClr: "1F4E79", bg: "FFFFFF", notes: 130}
	b := makeDeck(t, good, good, good)
	if r := validatePPTX(bytes.NewReader(b), int64(len(b)), 3); !r.ok() || r.Slides != 3 || r.NotesWords != 390 {
		t.Fatalf("good deck failed: %s", r)
	}
	// live run 6: every animated shape hidden=1, near-white title, notes too long
	bad := makeDeck(t,
		tSlide{titleClr: "F2F2F2", bg: "FFFFFF", hidden: true, entr: true, notes: 400},
		tSlide{titleClr: "1F4E79", bg: "FFFFFF", hidden: true, notes: 5})
	r := validatePPTX(bytes.NewReader(bad), int64(len(bad)), 2)
	all := strings.Join(r.Fails, "\n")
	for _, want := range []string{"không có animation xuất hiện", "vừa có hidden=1 vừa có animation", "tiêu đề khó đọc", "slide thiếu speaker notes (dưới 15 từ): 2", "yêu cầu 2 phút"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if r := validatePPTX(bytes.NewReader([]byte("x")), 1, 0); r.ok() {
		t.Fatal("broken zip passed")
	}
	if targetMinutes("bài thuyết trình 6 phút") != 6 || targetMinutes("a 6-minute talk") != 6 || targetMinutes("không có") != 0 {
		t.Fatal("targetMinutes")
	}
}

func TestChecklistFromRequestAndVerification(t *testing.T) {
	svc := quietSvc(t)
	bot := firstBot(svc)
	svc.newRequest(bot, "Làm bài thuyết trình 2 phút (.pptx), kịch bản nói từng slide và quay video xem trước", true)
	if c := svc.pcSnap(bot); len(c.checklist) != 3 || c.targetMin != 2 {
		t.Fatalf("%+v", c.checklist)
	}
	if len(svc.openItems(bot)) != 3 {
		t.Fatal("nothing verified yet")
	}
	good := tSlide{titleClr: "1F4E79", bg: "FFFFFF", notes: 130}
	svc.PC.WriteFile(bot, "out/deck.pptx", string(makeDeck(t, good, good)))
	svc.refreshChecklist(bot)
	if open := svc.openItems(bot); len(open) != 1 || open[0].Key != "video" {
		t.Fatalf("%+v", open)
	}
	svc.noteImpossible(bot, "Đã xong slide.\nKHÔNG THỂ: video — máy không đủ RAM để ghép mp4")
	if len(svc.openItems(bot)) != 0 || !strings.Contains(checklistText(svc.pcSnap(bot).checklist), "không thể: máy không đủ RAM") {
		t.Fatal(checklistText(svc.pcSnap(bot).checklist))
	}
	// an unrelated new request clears it; a nudge adds to it
	svc.newRequest(bot, "thời tiết hôm nay", false)
	if len(svc.pcSnap(bot).checklist) != 0 {
		t.Fatal("stale checklist kept")
	}
}

// A final reply that announces work ("mình sẽ rút gọn…") while the
// checklist is open does not end the turn: the bot is pushed to continue
// and the next reply's tool call runs.
func TestUnfinishedTaskForcesContinue(t *testing.T) {
	fa := scriptedAI(t,
		"File .pptx đã xong. Tổng 1020 từ, dài hơn 6 phút; mình sẽ rút gọn kịch bản rồi dựng video sau.",
		"TOOL: {\"name\":\"computer_write_file\",\"path\":\"make_video.py\",\"content\":\"print(1)\"}",
		"Đã ghi script video.\nKHÔNG THỂ: video — máy tính thử nghiệm không chạy lệnh")
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
	post(t, srv.URL+"/app/api/agents/"+bot+"/messages", `{"text":"làm video xem trước trên máy tính nhé"}`)
	waitTask(t, svc, bot)
	if _, _, err := svc.PC.ReadFile(bot, "make_video.py"); err != nil {
		t.Fatal("bot was not pushed to continue:", err)
	}
	if !strings.Contains(lastReply(svc, bot), "KHÔNG THỂ: video") {
		t.Fatalf("%q", lastReply(svc, bot))
	}
	if len(svc.openItems(bot)) != 0 {
		t.Fatal("impossible item still open")
	}
}

func TestPromiseTailWithoutChecklist(t *testing.T) {
	svc := quietSvc(t)
	w := startWorkLite(firstBot(svc), "chạy script python trên máy tính")
	long := strings.Repeat("Kết quả bước một đã có đầy đủ. ", 20) + "\nGiờ mình sẽ viết script tiếp theo để vẽ biểu đồ."
	if svc.continueNudge(w, directives{Text: long}) == "" {
		t.Fatal("long promise not caught")
	}
	if svc.continueNudge(w, directives{Text: "Đã xong, file ở out/a.csv."}) != "" {
		t.Fatal("finished reply pushed")
	}
}

func startWorkLite(bot, text string) ledgerWork { return ledgerWork{BotID: bot, Text: text} }
