package aurora

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// The real deck from live run 8: no diacritics and overlapping labels.
func TestRun8DeckRejected(t *testing.T) {
	b, err := os.ReadFile("testdata/run8_deck.pptx")
	if err != nil {
		t.Fatal(err)
	}
	r := validatePPTX(strings.NewReader(string(b)), int64(len(b)), 6)
	all := strings.Join(r.Fails, "\n")
	for _, want := range []string{"KHÔNG DẤU", "chữ chồng lên nhau", "slide 4"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	if len(r.SlideText) != 7 || !strings.Contains(r.SlideText[3], "GHI CHÚ:") {
		t.Fatalf("slide text %d", len(r.SlideText))
	}
}

func TestUnaccentedVi(t *testing.T) {
	bare := "Bien thuy ngan thanh vang la uoc mo cua gia kim. Nguoi ta khong the lam duoc dieu nay voi hoa hoc, nhung vat ly hat nhan thi co the den mot muc nao do."
	if unaccentedVi(bare) == "" {
		t.Fatal("bare Vietnamese passed")
	}
	good := "Biến thuỷ ngân thành vàng là ước mơ của giả kim. Người ta không thể làm được điều này với hoá học, nhưng vật lý hạt nhân thì có thể đến một mức nào đó."
	if m := unaccentedVi(good); m != "" {
		t.Fatal(m)
	}
	if unaccentedVi("Gold from mercury: the 1941 Sherr Bainbridge Anderson experiment used fast neutrons.") != "" {
		t.Fatal("English flagged")
	}
}

func TestOverlaps(t *testing.T) {
	a := textBox{name: "A", x: 0, y: 0, cx: 100, cy: 100}
	if len(overlaps([]textBox{a, {name: "B", x: 40, y: 40, cx: 100, cy: 100}})) != 1 {
		t.Fatal("overlap missed")
	}
	if len(overlaps([]textBox{a, {name: "C", x: 95, y: 0, cx: 100, cy: 100}, {name: "D", x: 0, y: 120, cx: 50, cy: 50}})) != 0 {
		t.Fatal("touching/separate flagged")
	}
}

func TestFactCheckCrossSlideCached(t *testing.T) {
	var calls atomic.Int32
	fa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"SAI: slide 2: Hg-198 + n → Au-197 + p — (n,p) trên Hg-198 cho Au-198 phóng xạ, mâu thuẫn slide 1\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer fa.Close()
	t.Setenv("ZAI_API_KEY", "k")
	t.Setenv("AURORA_AI_BASE_URL", fa.URL)
	svc, _ := newSvc(t)
	bot := firstBot(svc)
	rep := deckReport{SlideText: []string{"1941: Au-198, Au-199 phóng xạ", "Hg-198 + n → Au-197 + p"}}
	f := svc.factCheck(t.Context(), bot, rep)
	if len(f) != 1 || !strings.Contains(f[0], "slide 2") {
		t.Fatalf("%v", f)
	}
	svc.factCheck(t.Context(), bot, rep)
	if calls.Load() != 1 {
		t.Fatal("unchanged deck re-checked")
	}
}
