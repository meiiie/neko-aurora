package aurora

// Attack the premise (idea from pstack by Lauren Tan, MIT; rewritten for
// Aurora's Go gates). Each failure in a request gets a fingerprint: kind plus
// its most specific error line, with numbers, paths and quoted text
// normalized. The 2nd identical failure tells the bot to state and test its
// premise and change approach; the 3rd stops the build loop with an honest
// status and options for the owner. Runs 9/10 retried the same missing font
// fix many times.

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	pivotAt = 2
	stopAt  = 3
)

var (
	errLineRe = regexp.MustCompile(`(?i)(error|exception|lỗi|không tìm thấy|cannot|failed|no such|not found|traceback)`)
	quotedRe  = regexp.MustCompile("[\"'`“”][^\"'`“”\n]{0,200}[\"'`“”]")
	pathRe    = regexp.MustCompile(`(?:[\w.-]*/)+[\w.-]+`)
	digitsRe  = regexp.MustCompile(`\d+`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

// failSig is the fingerprint of one failure.
func failSig(kind, out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	pick := ""
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || strings.HasPrefix(l, "[mã thoát") || strings.HasPrefix(l, "$ ") {
			continue
		}
		if pick == "" {
			pick = l // last meaningful line as a fallback
		}
		if errLineRe.MatchString(l) && !strings.HasPrefix(strings.ToLower(l), "traceback") {
			pick = l
			break
		}
	}
	n := strings.ToLower(pick)
	n = quotedRe.ReplaceAllString(n, "'…'")
	n = pathRe.ReplaceAllString(n, "<p>")
	n = digitsRe.ReplaceAllString(n, "#")
	n = spaceRe.ReplaceAllString(n, " ")
	return kind + "|" + clean(strings.TrimSpace(n), 160)
}

// failNote records a failure and returns what the bot must do about a repeat
// ("" on the first occurrence).
func (s *Service) failNote(bot, kind, out string) string {
	sig := failSig(kind, out)
	n := 0
	s.pcDo(bot, func(st *pcState) {
		if st.failSigs == nil {
			st.failSigs = map[string]int{}
		}
		st.failSigs[sig]++
		n = st.failSigs[sig]
		if n >= stopAt {
			st.stuck = sig
		}
	})
	shown := strings.SplitN(sig, "|", 2)[1]
	switch {
	case n >= stopAt:
		return fmt.Sprintf("\n[Hệ thống] DỪNG: cùng một lỗi lần thứ %d (“%s”). Không thử lại cách cũ, không chạy thêm lệnh. Báo trung thực cho bạn: đã xong gì (file có thật), lỗi gì còn lại và vì sao; rồi thêm dòng CHOICES với 2–3 hướng đi cụ thể (vd. bỏ phần lỗi và giữ phần đã xong, hoặc một cách làm khác).", n, shown)
	case n >= pivotAt:
		return fmt.Sprintf("\n[Hệ thống] Lỗi này lặp lại lần %d (“%s”): cách sửa vừa rồi dựa trên một giả định sai. Viết giả định đó trong MỘT câu, kiểm chứng bằng MỘT lệnh ngắn (vd. ls, fc-list, python3 -c \"import …\"), rồi ĐỔI cách làm (thư viện/font/đường dẫn/phương pháp khác). Không lặp lại cùng cách sửa — lần nữa sẽ phải dừng.", n, shown)
	}
	return ""
}
