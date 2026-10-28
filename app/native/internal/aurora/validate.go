package aurora

// Deliverable validator and task checklist (live runs 5-6: a deck whose
// animated shapes were all hidden="1", near-white titles, notes 30% over
// the requested 6 minutes, and a video that was announced but never made).
//
// After every successful command the bot's new or changed .pptx files are
// checked on the server (zip + XML, no renderer needed) and the findings go
// back into the build loop. The owner's request is turned into a checklist
// (deck, speaker notes, video …) that stays open until each item is
// verified on the computer or the bot honestly marks it impossible.

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"nekoaurora.local/native/internal/ledger"
)

type deckReport struct {
	Slides     int
	NotesWords int
	Fails      []string
	Warns      []string
	SlideText  []string // per slide: shape text, then notes (for the fact pass)
}

func (r deckReport) ok() bool { return len(r.Fails) == 0 }

func (r deckReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d slide, %d từ ghi chú", r.Slides, r.NotesWords)
	if len(r.Fails) == 0 {
		b.WriteString(" · ĐẠT")
	} else {
		b.WriteString(" · CHƯA ĐẠT:\n- " + strings.Join(r.Fails, "\n- "))
	}
	if len(r.Warns) > 0 {
		b.WriteString("\nLưu ý:\n- " + strings.Join(r.Warns, "\n- "))
	}
	return b.String()
}

const wordsPerMinute = 130

var (
	slideFileRe = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)
	minutesRe   = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(?:-\s*)?(phút|minutes?|mins?)\b`)
)

// targetMinutes reads "6 phút" / "6-minute" from the owner's request.
func targetMinutes(t string) float64 {
	if m := minutesRe.FindStringSubmatch(t); m != nil {
		f, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		return f
	}
	return 0
}

// generic XML element walk: just what the checks need
type xnode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []xnode    `xml:",any"`
	Text    string     `xml:",chardata"`
}

func (n *xnode) attr(k string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == k {
			return a.Value
		}
	}
	return ""
}

func (n *xnode) walk(fn func(n *xnode, parents []*xnode)) {
	var rec func(n *xnode, ps []*xnode)
	rec = func(n *xnode, ps []*xnode) {
		fn(n, ps)
		ps = append(ps, n)
		for i := range n.Nodes {
			rec(&n.Nodes[i], ps)
		}
	}
	rec(n, nil)
}

func (n *xnode) find(local string) *xnode {
	var hit *xnode
	n.walk(func(x *xnode, _ []*xnode) {
		if hit == nil && x.XMLName.Local == local {
			hit = x
		}
	})
	return hit
}

func (n *xnode) text() string {
	var b strings.Builder
	n.walk(func(x *xnode, _ []*xnode) {
		if x.XMLName.Local == "t" {
			b.WriteString(x.Text + " ")
		}
	})
	return strings.TrimSpace(b.String())
}

func fillColor(n *xnode) string { // first solidFill/srgbClr directly under n
	if n == nil {
		return ""
	}
	for i := range n.Nodes {
		if c := &n.Nodes[i]; c.XMLName.Local == "solidFill" {
			if s := c.find("srgbClr"); s != nil {
				return s.attr("val")
			}
		}
	}
	return ""
}

func lum(hex string) float64 {
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return -1
	}
	ch := func(c uint64) float64 {
		x := float64(c) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(v>>16&255) + 0.7152*ch(v>>8&255) + 0.0722*ch(v&255)
}

func contrast(a, b string) float64 {
	la, lb := lum(a), lum(b)
	if la < 0 || lb < 0 {
		return 21
	}
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// validatePPTX checks a deck for the defects that make it unusable when
// presented. targetMin 0 skips the duration check.
func validatePPTX(r io.ReaderAt, size int64, targetMin float64) deckReport {
	var rep deckReport
	z, err := zip.NewReader(r, size)
	if err != nil {
		rep.Fails = append(rep.Fails, "không mở được file pptx (zip hỏng)")
		return rep
	}
	files := map[string]*zip.File{}
	var slides []int
	for _, f := range z.File {
		files[f.Name] = f
		if m := slideFileRe.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, n)
		}
	}
	sort.Ints(slides)
	rep.Slides = len(slides)
	if rep.Slides == 0 {
		rep.Fails = append(rep.Fails, "không có slide nào")
		return rep
	}
	read := func(name string) *xnode {
		f := files[name]
		if f == nil {
			return nil
		}
		rc, err := f.Open()
		if err != nil {
			return nil
		}
		defer rc.Close()
		var n xnode
		if xml.NewDecoder(io.LimitReader(rc, 8<<20)).Decode(&n) != nil {
			return nil
		}
		return &n
	}
	var missingNotes, hiddenNoAnim, hiddenAnim, lowContrast, tiny, overlapped []string
	for _, i := range slides {
		sl := read(fmt.Sprintf("ppt/slides/slide%d.xml", i))
		if sl == nil {
			rep.Fails = append(rep.Fails, fmt.Sprintf("slide %d: XML hỏng", i))
			continue
		}
		// entrance-animated shape ids
		entr := map[string]bool{}
		if tm := sl.find("timing"); tm != nil {
			tm.walk(func(x *xnode, ps []*xnode) {
				if x.XMLName.Local != "spTgt" {
					return
				}
				for _, p := range ps {
					if p.XMLName.Local == "cTn" && p.attr("presetClass") == "entr" {
						entr[x.attr("spid")] = true
						return
					}
				}
			})
		}
		bg := "FFFFFF"
		if b := sl.find("bgPr"); b != nil && fillColor(b) != "" {
			bg = fillColor(b)
		}
		var titleTxt, titleClr, titleBg string
		titleSz := 0
		var stxt strings.Builder
		var boxes []textBox
		sl.walk(func(x *xnode, ps []*xnode) {
			if x.XMLName.Local != "sp" {
				return
			}
			if t := x.text(); t != "" {
				stxt.WriteString(t + "\n")
				if b, ok := boxOf(x, ps); ok {
					boxes = append(boxes, b)
				}
			}
			nv := x.find("cNvPr")
			if nv != nil && (nv.attr("hidden") == "1" || nv.attr("hidden") == "true") {
				name := fmt.Sprintf("slide %d “%s”", i, clean(x.text(), 30))
				if entr[nv.attr("id")] {
					hiddenAnim = append(hiddenAnim, name)
				} else {
					hiddenNoAnim = append(hiddenNoAnim, name)
				}
			}
			txt := x.text()
			if txt == "" {
				return
			}
			shapeBg := bg
			if sp := x.find("spPr"); sp != nil && fillColor(sp) != "" {
				shapeBg = fillColor(sp)
			}
			isTitle := false
			if ph := x.find("ph"); ph != nil && (ph.attr("type") == "title" || ph.attr("type") == "ctrTitle") {
				isTitle = true
			}
			x.walk(func(r *xnode, _ []*xnode) {
				if r.XMLName.Local != "rPr" && r.XMLName.Local != "defRPr" {
					return
				}
				sz, _ := strconv.Atoi(r.attr("sz"))
				if sz > 0 && sz < 1200 {
					tiny = append(tiny, fmt.Sprintf("slide %d (%.0fpt)", i, float64(sz)/100))
				}
				clr := fillColor(r)
				if clr == "" {
					clr = "000000"
				}
				if sz > titleSz || isTitle && titleTxt == "" {
					titleSz, titleTxt, titleClr, titleBg = sz, txt, clr, shapeBg
				}
			})
		})
		if titleTxt != "" {
			if c := contrast(titleClr, titleBg); c < 3 {
				lowContrast = append(lowContrast, fmt.Sprintf("slide %d tiêu đề “%s” #%s trên nền #%s (tỷ lệ %.1f:1, cần ≥ 3:1)", i, clean(titleTxt, 30), titleClr, titleBg, c))
			}
		}
		// notes: the slide's rels name its notesSlide
		words := 0
		if rel := read(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i)); rel != nil {
			rel.walk(func(x *xnode, _ []*xnode) {
				if x.XMLName.Local == "Relationship" && strings.HasSuffix(x.attr("Type"), "/notesSlide") {
					if ns := read(path.Clean("ppt/slides/" + x.attr("Target"))); ns != nil {
						ns.walk(func(sp *xnode, _ []*xnode) {
							if sp.XMLName.Local == "sp" {
								if ph := sp.find("ph"); ph != nil && ph.attr("type") == "body" {
									words += len(strings.FieldsFunc(sp.text(), func(r rune) bool { return unicode.IsSpace(r) }))
									stxt.WriteString("GHI CHÚ: " + sp.text() + "\n")
								}
							}
						})
					}
				}
			})
		}
		if words < 15 {
			missingNotes = append(missingNotes, strconv.Itoa(i))
		}
		rep.NotesWords += words
		rep.SlideText = append(rep.SlideText, stxt.String())
		for _, o := range overlaps(boxes) {
			overlapped = append(overlapped, fmt.Sprintf("slide %d %s", i, o))
		}
	}
	if m := unaccentedVi(strings.Join(rep.SlideText, "\n")); m != "" {
		rep.Fails = append(rep.Fails, m)
	}
	list := func(xs []string) string {
		if len(xs) > 6 {
			return strings.Join(xs[:6], ", ") + fmt.Sprintf(" … (+%d)", len(xs)-6)
		}
		return strings.Join(xs, ", ")
	}
	if len(hiddenNoAnim) > 0 {
		rep.Fails = append(rep.Fails, "shape bị ẩn (hidden=1) mà không có animation xuất hiện nào làm nó hiện: "+list(hiddenNoAnim)+" — bỏ thuộc tính hidden")
	}
	if len(hiddenAnim) > 0 {
		rep.Fails = append(rep.Fails, "shape vừa có hidden=1 vừa có animation xuất hiện: "+list(hiddenAnim)+" — hidden=1 làm shape ẩn hẳn (LibreOffice/xuất PDF không hiện); animation xuất hiện đã tự ẩn shape tới lúc bấm, nên bỏ hidden")
	}
	if len(lowContrast) > 0 {
		rep.Fails = append(rep.Fails, "tiêu đề khó đọc: "+list(lowContrast))
	}
	if len(missingNotes) > 0 {
		rep.Fails = append(rep.Fails, "slide thiếu speaker notes (dưới 15 từ): "+strings.Join(missingNotes, ", "))
	}
	if targetMin > 0 {
		want := targetMin * wordsPerMinute
		if got := float64(rep.NotesWords); got < want*0.8 || got > want*1.2 {
			rep.Fails = append(rep.Fails, fmt.Sprintf("ghi chú %d từ ≈ %.1f phút, yêu cầu %.0f phút (cần %d–%d từ ở %d từ/phút)", rep.NotesWords, got/wordsPerMinute, targetMin, int(want*0.8), int(want*1.2), wordsPerMinute))
		}
	}
	if len(overlapped) > 0 {
		rep.Fails = append(rep.Fails, "chữ chồng lên nhau (khung chữ đè nhau): "+list(overlapped)+" — dời vị trí/thu nhỏ để các khung chữ không giao nhau")
	}
	if len(tiny) > 0 {
		rep.Warns = append(rep.Warns, "chữ nhỏ hơn 12pt: "+list(tiny))
	}
	return rep
}

// ---- task checklist ----

type checkItem struct {
	Key, Label string
	Done       bool
	Impossible string // the bot's honest reason
	Why        string // what the check found (file and result)
}

var checkSpecs = []struct {
	key, label string
	re         *regexp.Regexp
}{
	{"deck", "file .pptx", regexp.MustCompile(`(?i)pptx|powerpoint|slide|thuyết trình|trình chiếu|presentation|\bdeck\b`)},
	{"notes", "speaker notes / kịch bản nói từng slide", regexp.MustCompile(`(?i)kịch bản|speaker notes|ghi chú (thuyết trình|người nói)|script nói|lời thoại`)},
	{"video", "video xem trước (.mp4)", regexp.MustCompile(`(?i)video|\bmp4\b|quay (lại|màn hình)|clip`)},
	{"sheet", "bảng tính (.xlsx/.csv)", regexp.MustCompile(`(?i)xlsx|excel|bảng tính|spreadsheet|\bcsv\b`)},
	{"code", "script chạy được (python)", regexp.MustCompile(`(?i)viết (một |cái )?(script|code|chương trình)|python script|\.py\b`)},
}

// mergeChecklist adds the items a new owner message asks for.
func mergeChecklist(cur []checkItem, text string) []checkItem {
	have := map[string]bool{}
	for _, c := range cur {
		have[c.Key] = true
	}
	for _, sp := range checkSpecs {
		if !have[sp.key] && sp.re.MatchString(text) {
			cur = append(cur, checkItem{Key: sp.key, Label: sp.label})
		}
	}
	return cur
}

var impossibleRe = regexp.MustCompile(`(?mi)^\s*KHÔNG THỂ\s*:\s*([^\n—:-]+?)\s*[—:-]\s*(.+)$`)

// refreshChecklist verifies each item against the real files (see
// artifacts.go): an item closes only when one of its files passes its check.
func (s *Service) refreshChecklist(bot string) {
	if s.PC == nil {
		return
	}
	c := s.pcSnap(bot)
	if len(c.checklist) == 0 {
		return
	}
	got := s.verifyArtifacts(bot, c)
	s.pcDo(bot, func(st *pcState) {
		for i := range st.checklist {
			v := got[st.checklist[i].Key]
			st.checklist[i].Done, st.checklist[i].Why = v.ok, v.why
		}
	})
	s.writeVerify(bot)
}

// noteImpossible records "KHÔNG THỂ: video — lý do" lines from a reply.
func (s *Service) noteImpossible(bot, text string) {
	for _, m := range impossibleRe.FindAllStringSubmatch(text, -1) {
		what := strings.ToLower(m[1])
		s.pcDo(bot, func(st *pcState) {
			for i := range st.checklist {
				it := &st.checklist[i]
				if strings.Contains(what, it.Key) || strings.Contains(strings.ToLower(it.Label), what) || (it.Key == "deck" && strings.Contains(what, "pptx")) || (it.Key == "notes" && strings.Contains(what, "kịch bản")) {
					it.Impossible = clean(m[2], 200)
				}
			}
		})
	}
}

func (s *Service) openItems(bot string) []checkItem {
	var out []checkItem
	for _, c := range s.pcSnap(bot).checklist {
		if !c.Done && c.Impossible == "" {
			out = append(out, c)
		}
	}
	return out
}

func checklistText(items []checkItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nDANH SÁCH VIỆC CỦA YÊU CẦU (hệ thống tự kiểm tra trên máy tính):")
	for _, c := range items {
		mark := "[ ]"
		switch {
		case c.Done:
			mark = "[x]"
		case c.Impossible != "":
			mark = "[không thể: " + c.Impossible + "]"
		}
		b.WriteString("\n" + mark + " " + c.Label)
		if c.Why != "" {
			b.WriteString(" — " + c.Why)
		}
	}
	b.WriteString("\nChưa xong mục nào thì làm tiếp bằng TOOL; mục thật sự không làm được thì ghi một dòng “KHÔNG THỂ: <mục> — <lý do>”.")
	return b.String()
}

// validateNew checks the decks changed since `since` and reports them for
// the build loop.
func (s *Service) validateNew(ctx context.Context, bot string, since time.Time) string {
	if s.PC == nil {
		return ""
	}
	l, err := s.PC.List(bot, "")
	if err != nil {
		return ""
	}
	target := s.pcSnap(bot).targetMin
	var b strings.Builder
	for _, e := range l {
		if e.Dir || strings.ToLower(path.Ext(e.Path)) != ".pptx" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, e.Mod); err != nil || t.Before(since.Add(-time.Second)) {
			continue
		}
		f, size, err := s.PC.OpenFile(bot, e.Path)
		if err != nil {
			continue
		}
		rep := validatePPTX(f, size, target)
		f.Close()
		rep.Fails = append(rep.Fails, s.factCheck(ctx, bot, rep)...)
		fmt.Fprintf(&b, "\nKIỂM TRA TỰ ĐỘNG %s: %s", e.Path, rep)
		if !rep.ok() {
			b.WriteString("\n→ Sửa script tạo file theo các điểm CHƯA ĐẠT rồi chạy lại; chưa đạt thì chưa được báo xong.")
			b.WriteString(s.failNote(bot, "deck", strings.Join(rep.Fails, "\n")))
		}
	}
	s.refreshChecklist(bot)
	return b.String()
}

var promiseTailRe = regexp.MustCompile(`(?i)(mình|em|tôi|tớ)\s+(sẽ|giờ|bây giờ|tiếp theo|tiếp tục|đang)\s+[\p{L} ]{0,20}(viết|làm|rút gọn|dựng|chạy|tạo|ghép|render|sửa|kiểm tra|cập nhật)|(I'll|I will|next I|let me|now I)\b`)

// continueNudge says why a reply without a tool call must not end the turn:
// open checklist items, or a last paragraph that announces more work.
func (s *Service) continueNudge(w ledger.Work, d directives) string {
	if s.PC == nil || len(d.Tools) > 0 || d.Approval != nil || len(d.Choices) > 0 || len(d.Questions) > 0 || !computerTask(w) || s.buildExhausted(w.BotID) {
		return ""
	}
	s.noteImpossible(w.BotID, d.Text)
	s.refreshChecklist(w.BotID)
	if open := s.openItems(w.BotID); len(open) > 0 {
		var xs []string
		for _, c := range open {
			xs = append(xs, c.Label)
		}
		return "[Hệ thống] Yêu cầu CHƯA xong — còn thiếu (đã kiểm tra trên máy tính): " + strings.Join(xs, "; ") + ". Đừng kết thúc lượt: làm tiếp NGAY bằng dòng TOOL (computer_write_file / computer_run). Mục nào thật sự không làm được thì ghi một dòng “KHÔNG THỂ: <mục> — <lý do>”."
	}
	paras := strings.Split(strings.TrimSpace(d.Text), "\n")
	if promiseTailRe.MatchString(paras[len(paras)-1]) {
		return "[Hệ thống] Bạn vừa báo sẽ làm tiếp nhưng không gọi công cụ nào nên chưa có gì xảy ra. Làm việc đó NGAY bằng dòng TOOL, không mô tả."
	}
	return ""
}
