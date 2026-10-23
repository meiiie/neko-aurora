package aurora

// Deterministic deck checks added after live run 8: Vietnamese written
// without diacritics ("Bien thuy nganh thanh vang"), text boxes drawn on top
// of each other (slide 4 diagram), and one slide contradicting another
// (1941: "Au-198, Au-199" on slide 3, "Hg-198 + n → Au-197 + p" on slide 4).

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type textBox struct {
	name         string
	x, y, cx, cy int64
}

// boxOf reads a text shape's frame. Shapes inside a group use the group's
// child coordinates, so they are skipped rather than guessed.
func boxOf(sp *xnode, parents []*xnode) (textBox, bool) {
	for _, p := range parents {
		if p.XMLName.Local == "grpSp" {
			return textBox{}, false
		}
	}
	pr := sp.find("spPr")
	if pr == nil {
		return textBox{}, false
	}
	xf := pr.find("xfrm")
	if xf == nil {
		return textBox{}, false
	}
	off, ext := xf.find("off"), xf.find("ext")
	if off == nil || ext == nil {
		return textBox{}, false
	}
	n := func(x *xnode, k string) int64 { v, _ := strconv.ParseInt(x.attr(k), 10, 64); return v }
	b := textBox{name: clean(sp.text(), 24), x: n(off, "x"), y: n(off, "y"), cx: n(ext, "cx"), cy: n(ext, "cy")}
	return b, b.cx > 0 && b.cy > 0
}

// overlaps lists pairs of text frames that cover each other by more than a
// quarter of the smaller one (a label merely touching a box is fine).
func overlaps(bs []textBox) []string {
	var out []string
	for i := 0; i < len(bs); i++ {
		for j := i + 1; j < len(bs); j++ {
			a, b := bs[i], bs[j]
			w := min(a.x+a.cx, b.x+b.cx) - max(a.x, b.x)
			h := min(a.y+a.cy, b.y+b.cy) - max(a.y, b.y)
			if w <= 0 || h <= 0 {
				continue
			}
			small := min(a.cx*a.cy, b.cx*b.cy)
			if float64(w)*float64(h) > 0.25*float64(small) {
				out = append(out, fmt.Sprintf("“%s” đè “%s”", a.name, b.name))
			}
		}
	}
	return out
}

// Common Vietnamese words that always carry a mark when written correctly;
// seen bare, the text was typed without diacritics.
var bareVi = map[string]bool{"va": true, "cua": true, "khong": true, "duoc": true, "mot": true, "cac": true, "nguoi": true, "nhung": true, "nay": true, "voi": true, "den": true, "la": true, "nhu": true, "thuy": true, "ngan": true, "vang": true, "bien": true, "tu": true, "hat": true, "nhan": true}

// unaccentedVi returns a fail message when Vietnamese text is written
// without diacritics: enough bare marker words and few marked tokens.
func unaccentedVi(t string) string {
	words, marked, bare := 0, 0, 0
	var ex []string
	for _, f := range strings.FieldsFunc(t, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(f)) < 2 {
			continue
		}
		words++
		if viLetters(f) > 0 {
			marked++
			continue
		}
		if bareVi[strings.ToLower(f)] {
			bare++
			if len(ex) < 6 {
				ex = append(ex, f)
			}
		}
	}
	if bare < 6 || words == 0 || float64(marked)/float64(words) >= 0.2 {
		return ""
	}
	return fmt.Sprintf("chữ tiếng Việt KHÔNG DẤU (%d%% từ có dấu; vd. %s): viết chuỗi Unicode có dấu đầy đủ ngay trong script (file UTF-8, dòng đầu # -*- coding: utf-8 -*-). python-pptx lưu Unicode bình thường; font Arial/DejaVu Sans/Noto Sans hiển thị đủ dấu", 100*marked/words, strings.Join(ex, ", "))
}

const factPrompt = `Bạn kiểm tra sự thật cho một bài trình chiếu. Dưới đây là chữ trên từng slide (kèm GHI CHÚ thuyết trình) và GHI CHÚ NGHIÊN CỨU.
Tìm: (1) slide mâu thuẫn với slide khác (số liệu, đồng vị, phản ứng, năm, tên người, tổ chức); (2) điều trái với ghi chú nghiên cứu hoặc sai rõ ràng về khoa học.
Chỉ nêu lỗi chắc chắn: điều GHI CHÚ NGHIÊN CỨU nói rõ là khác, hoặc hai slide nói ngược nhau. Không báo lỗi chỉ vì có cách diễn đạt/phản ứng khác cũng đúng (vd. Hg-198(n,2n)Hg-197 là đề xuất của Marathon Fusion 2025, đúng). Mỗi lỗi một dòng đúng dạng: SAI: slide <số>: <điều sai> — <vì sao, đúng là gì>
Không có lỗi: trả lời đúng một chữ OK.`

// factCheck runs one model pass over a built deck. Results are cached by the
// deck text so an unchanged rerun costs nothing.
func (s *Service) factCheck(ctx context.Context, bot string, rep deckReport) []string {
	if s.AI == nil || !s.AI.Configured() || len(rep.SlideText) < 2 {
		return nil
	}
	var b strings.Builder
	for i, t := range rep.SlideText {
		fmt.Fprintf(&b, "=== SLIDE %d ===\n%s\n", i+1, clean(t, 2500))
	}
	notes := s.pcSnap(bot).research
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(b.String()+notes)))
	if st := s.pcSnap(bot); st.factKey == key {
		return st.factOut
	}
	if notes == "" {
		notes = "(không có — chỉ kiểm tra mâu thuẫn giữa các slide và lỗi khoa học rõ ràng)"
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := s.stream(cctx, bot, "factcheck", []ChatMsg{{"system", factPrompt}, {"user", "GHI CHÚ NGHIÊN CỨU:\n" + notes + "\n\n" + b.String()}}, func(string) {})
	if err != nil {
		return nil // never block a build on the checker itself
	}
	var fails []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(strings.TrimLeft(l, "-•* ")); strings.HasPrefix(l, "SAI:") && len(fails) < 8 {
			fails = append(fails, "sai sự thật / mâu thuẫn — "+clean(strings.TrimSpace(strings.TrimPrefix(l, "SAI:")), 300))
		}
	}
	s.pcDo(bot, func(st *pcState) { st.factKey, st.factOut = key, fails })
	return fails
}
