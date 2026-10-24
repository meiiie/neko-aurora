package aurora

// Honest tool use and deliverable review for work on the bot's computer.
//
// Narration: the model sometimes writes what a tool would have done
// ("[đã ghi render_video.py]", "(đã gọi công cụ computer_write_file …)")
// instead of emitting the TOOL line, and the run silently stalls (live
// pptx runs 3 and 4). Any bracketed claim of a past tool action with no
// tool behind it is stripped and the model is asked for directives only.
//
// Review: before a file with user-facing text is written in a researched
// task, a second, non-streamed model pass compares it with the research
// notes (numbers, isotopes, names, years) and fixes contradictions, typos,
// mixed scripts and unfinished clauses (live run 4: "Au-197" for the 1941
// radioactive gold, "Hg-196", "микro-gam").

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"nekoaurora.local/native/internal/ledger"
)

var (
	claimSpanRe = regexp.MustCompile(`\[([^\[\]\n]{3,240})\]|\(([^()\n]{3,240})\)`)
	actionRe    = regexp.MustCompile(`(?i)(^|[\s,;:—-])(đã|vừa)\s+(ghi|gọi|chạy|tạo|lưu|viết|cài|render|ghép|xuất|sửa|cập nhật|dựng)(\s|$)|(^|\s)(wrote|ran|called|executed|saved|created|updated|installed)(\s|$)|trạng thái(\s+duyệt)?\s*:`)
	artifactRe  = regexp.MustCompile(`(?i)[\w./-]+\.(py|pptx|mp4|csv|txt|json|md|png|jpe?g|sh|js|html|xlsx|pdf|gif|webm)\b|computer_\w+|web_\w+|công cụ|lệnh|script|file|tệp|tool|command`)
	fileTokRe   = regexp.MustCompile(`(?i)[\w./-]+\.(py|pptx|mp4|csv|txt|json|md|png|jpe?g|sh|js|html|xlsx|pdf|gif|webm)\b`)
)

const directivesOnly = "[Hệ thống] Câu vừa rồi MÔ TẢ việc dùng công cụ (ghi file/chạy lệnh) nhưng KHÔNG có dòng TOOL nào nên chưa có gì xảy ra. Lần này chỉ trả lời bằng các dòng TOOL thật (vd. TOOL: {\"name\":\"computer_write_file\",\"path\":\"…\",\"content\":\"…\"} rồi TOOL: {\"name\":\"computer_run\",\"cmd\":\"…\"}), không văn xuôi. Nếu việc đã thật sự xong thì trả lời ngắn kèm link file có thật."

// claims returns bracketed spans that narrate a past tool action.
func claims(t string) []string {
	var out []string
	for _, m := range claimSpanRe.FindAllStringSubmatch(t, -1) {
		in := m[1] + m[2]
		if actionRe.MatchString(in) && artifactRe.MatchString(in) {
			out = append(out, m[0])
		}
	}
	return out
}

func stripNarration(t string) string {
	for _, c := range claims(t) {
		t = strings.Replace(t, c, "", 1)
	}
	lines := strings.Split(t, "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) == "" && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// narratedOnly: a reply with no tool call and no card whose claims of tool
// work are not backed by the bot's computer (a named file that does not
// exist, or a claim about a tool call itself).
func (s *Service) narratedOnly(w ledger.Work, d directives) bool {
	if len(d.Tools) > 0 || d.Approval != nil || s.PC == nil || !computerTask(w) {
		return false
	}
	for _, c := range claims(d.Text) {
		files := fileTokRe.FindAllString(c, -1)
		if len(files) == 0 {
			return true // "(đã gọi công cụ …)", "[đã chạy lệnh]": nothing to check, never a real call
		}
		for _, f := range files {
			fh, _, err := s.PC.OpenFile(w.BotID, f)
			if err != nil {
				return true
			}
			fh.Close()
		}
	}
	return false
}

// saveResearch keeps the turn's research (sources + step results) for the
// later build turns, whose own context no longer has it.
func (s *Service) saveResearch(bot string, tc *toolCtx) {
	if len(tc.sources) == 0 {
		return
	}
	var sb strings.Builder
	for i, src := range tc.sources {
		fmt.Fprintf(&sb, "[%d] %s — %s\n", i+1, src.Title, src.URL)
	}
	var rest []string
	for _, b := range tc.blocks {
		if strings.HasPrefix(b, "KẾT QUẢ BƯỚC") {
			sb.WriteString(b + "\n")
		} else {
			rest = append(rest, b)
		}
	}
	sb.WriteString(strings.Join(rest, "\n---\n"))
	notes := clean(sb.String(), 9000)
	s.pcDo(bot, func(st *pcState) { st.research = notes })
}

func (s *Service) researchNotes(bot string, tc *toolCtx) string {
	if tc != nil && len(tc.sources) > 0 {
		s.saveResearch(bot, tc)
	}
	return s.pcSnap(bot).research
}

// mixedScripts lists words that mix Latin with another script, or are
// Cyrillic/Han in otherwise Latin text ("микro-gam").
func mixedScripts(t string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.FieldsFunc(t, func(r rune) bool { return !unicode.IsLetter(r) && r != '-' }) {
		var lat, other bool
		for _, r := range f {
			switch {
			case unicode.Is(unicode.Latin, r):
				lat = true
			case unicode.Is(unicode.Cyrillic, r), unicode.Is(unicode.Han, r):
				other = true
			}
		}
		if other && (lat || len([]rune(f)) > 1) && !seen[f] {
			seen[f] = true
			out = append(out, f)
			if len(out) >= 12 {
				break
			}
		}
	}
	return out
}

func viLetters(t string) int {
	n := 0
	for _, r := range t {
		if r > 127 && unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

var reviewExt = regexp.MustCompile(`(?i)\.(py|md|txt|html|json|js|csv)$`)
var fenceRe = regexp.MustCompile("(?s)```[a-zA-Z0-9_-]*\\n(.*?)\\n?```")

const reviewPrompt = `Bạn là người kiểm tra sự thật và biên tập tiếng Việt cho một sản phẩm giao cho người dùng (vd. script tạo slide). Đối chiếu MỌI chữ hiển thị trong file (chuỗi văn bản, ghi chú thuyết trình) với GHI CHÚ NGHIÊN CỨU.
Sửa: (1) mâu thuẫn với ghi chú: số liệu, đồng vị, phản ứng, tên người, năm, tổ chức; (2) lỗi chính tả, từ gõ nhầm; (3) chữ lẫn hệ chữ khác (Cyrillic, Hán) trong từ tiếng Việt; (4) câu bỏ dở hoặc vô nghĩa.
Không đổi logic code, không thêm thông tin không có trong ghi chú, giữ nguyên định dạng.
Trả lời: dòng đầu đúng một chữ OK nếu không cần sửa. Nếu có sửa: dòng đầu "FIXED: <liệt kê ngắn từng chỗ sửa, cách nhau bằng ;>", sau đó TOÀN BỘ file đã sửa trong một khối code duy nhất.`

// reviewDeliverable returns the (possibly corrected) content and a short
// note of what was fixed. It never blocks the write: on any doubt the
// original content is kept.
func (s *Service) reviewDeliverable(ctx context.Context, bot string, tc *toolCtx, path, content string) (string, string) {
	if s.AI == nil || !s.AI.Configured() || !reviewExt.MatchString(path) || len(content) < 200 || len(content) > 60000 || viLetters(content) < 60 && unaccentedVi(content) == "" {
		return content, ""
	}
	notes := s.researchNotes(bot, tc)
	if notes == "" {
		return content, ""
	}
	user := "GHI CHÚ NGHIÊN CỨU:\n" + notes + "\n\nFILE " + path + ":\n```\n" + content + "\n```"
	if mx := mixedScripts(content); len(mx) > 0 {
		user += "\n\nĐã phát hiện chữ lẫn hệ chữ khác: " + strings.Join(mx, ", ")
	}
	rctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	out, err := s.stream(rctx, bot, "review", []ChatMsg{{"system", reviewPrompt}, {"user", user}}, func(string) {})
	if err != nil {
		return content, ""
	}
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(out, "FIXED") {
		return content, ""
	}
	head := firstLine(out)
	ms := fenceRe.FindAllStringSubmatch(out, -1)
	if len(ms) == 0 {
		return content, ""
	}
	fixed := ms[len(ms)-1][1]
	if r := float64(len(fixed)) / float64(len(content)); r < 0.7 || r > 1.5 {
		return content, "" // truncated or rewritten wholesale: keep the original
	}
	return fixed, clean(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(head, "FIXED"), ":")), 400)
}

var cyrLatin = map[rune]string{'а': "a", 'в': "v", 'е': "e", 'ё': "e", 'к': "k", 'м': "m", 'н': "n", 'о': "o", 'р': "r", 'с': "c", 'т': "t", 'у': "u", 'х': "x", 'и': "i", 'й': "i", 'л': "l", 'п': "p", 'д': "d", 'г': "g", 'б': "b", 'з': "z", 'ф': "f", 'ь': "", 'ъ': "", 'ы': "y", 'я': "ya", 'ю': "yu", 'ж': "zh", 'ш': "sh", 'щ': "sh", 'ч': "ch", 'ц': "ts", 'э': "e",
	'А': "A", 'В': "B", 'Е': "E", 'К': "K", 'М': "M", 'Н': "H", 'О': "O", 'Р': "P", 'С': "C", 'Т': "T", 'У': "Y", 'Х': "X", 'И': "I", 'Л': "L", 'П': "P", 'Д': "D", 'Г': "G"}

func foreignScript(r rune) bool {
	return unicode.In(r, unicode.Cyrillic, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana, unicode.Arabic, unicode.Thai)
}

// fixMixedScripts deterministically repairs words of a Vietnamese text that
// contain characters of another script (model drift: "микro-gam",
// "đan石"): Cyrillic letters become their Latin look-alikes, other foreign
// characters are removed. Greek letters (β, α, γ) are physics and stay.
// It returns the fixed text and the words it changed.
func fixMixedScripts(t string) (string, []string) {
	n := 0
	for _, r := range t {
		if r > 127 && unicode.Is(unicode.Latin, r) {
			n++
		}
	}
	if n < 20 { // not Vietnamese text
		return t, nil
	}
	var out strings.Builder
	var fixed []string
	rs := []rune(t)
	for i := 0; i < len(rs); {
		if !unicode.IsLetter(rs[i]) {
			out.WriteRune(rs[i])
			i++
			continue
		}
		j := i
		bad := false
		for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.Is(unicode.Mn, rs[j])) {
			bad = bad || foreignScript(rs[j])
			j++
		}
		word := string(rs[i:j])
		if bad {
			var w strings.Builder
			for _, r := range rs[i:j] {
				switch {
				case unicode.Is(unicode.Cyrillic, r):
					if l, ok := cyrLatin[r]; ok {
						w.WriteString(l)
					}
				case foreignScript(r):
				default:
					w.WriteRune(r)
				}
			}
			if len(fixed) < 20 {
				fixed = append(fixed, word+"→"+w.String())
			}
			word = w.String()
		}
		out.WriteString(word)
		i = j
	}
	return out.String(), fixed
}
