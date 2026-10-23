package aurora

// Long build tasks and the turn clock (live run 7: three rounds ended on
// "context deadline exceeded" while the bot rewrote a 15 KB script).
//
// A turn gets turnBudget for its tool rounds and continue attempts. When it
// runs out, the turn ends cleanly on a short note and the request continues
// as a fresh hidden turn (like a resume): history, checklist and computer
// context carry over, the clock restarts. Bounded by maxResumes per request.
// turnSlack lets the in-flight model call finish before the hard deadline.
//
// Scripts are compiled (biên dịch python3) before a command that runs
// them is parked for approval; a syntax error goes straight back to the bot.
// Replies cut by the output cap are detected (finish_reason=length or a
// broken stream) and the bot is told to write in smaller pieces.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nekoaurora.local/native/internal/ledger"
	"nekoaurora.local/native/internal/tools"
)

var (
	turnBudget = 4 * time.Minute
	turnSlack  = 4 * time.Minute
)

const (
	maxResumes     = 6
	truncMark      = "\n\x00[aurora:cut]"
	checkpointText = "Việc còn dài — mình đã lưu tiến độ và làm tiếp ngay ở lượt sau."
	cutNudge       = "[Hệ thống] Câu trả lời vừa rồi bị CẮT vì quá dài (giới hạn đầu ra), nên dòng TOOL bị mất và chưa có gì được ghi. Ghi file thành nhiều phần nhỏ: computer_write_file phần đầu (< ~6 KB), rồi các phần tiếp với \"append\":true; hoặc tách thành vài module. Chỉ in dòng TOOL."
	resumeText     = "[Hệ thống] Lượt trước hết thời gian nên đã dừng giữa chừng; tiếp tục ĐÚNG việc đang làm (xem lịch sử và ngữ cảnh máy tính), không làm lại từ đầu. Làm tiếp NGAY bằng dòng TOOL."
	bigFileBytes   = 8000
)

// checkpoint ends a computer turn that is out of time and queues its
// continuation. ok=false: still within budget, not a computer task, or the
// request already used its continuations (then the turn finishes normally).
func (s *Service) checkpoint(w ledger.Work, start time.Time) (directives, bool) {
	if time.Since(start) < turnBudget || !computerTask(w) || s.PC == nil {
		return directives{}, false
	}
	n := 0
	s.pcDo(w.BotID, func(st *pcState) { st.resumes++; n = st.resumes })
	if n > maxResumes {
		return directives{}, false
	}
	text := resumeText + checklistText(s.pcSnap(w.BotID).checklist)
	if _, _, err := s.L.Accept(ledger.Input{BotID: w.BotID, ClientNonce: newID("computer-resume-"), Text: clean(text, 7000)}); err != nil {
		return directives{}, false
	}
	go s.poke()
	return directives{Text: checkpointText}, true
}

// compile only (no __pycache__ left in the bot's home)
const pyCheckCmd = `python3 -c 'import sys
for f in sys.argv[1:]: compile(open(f, encoding="utf-8").read(), f, "exec")' `

// unfinished: a computer turn that ends with checklist items still open and
// no card or question for the owner. Live run 9: replies were empty after
// tool steps, or asked "nhắn 'tiếp tục'" instead of continuing. The work
// goes on as a new turn (bounded by maxResumes); an empty reply becomes a
// short honest status instead of a blank bubble.
func (s *Service) unfinished(w ledger.Work, d directives) (directives, bool) {
	if !computerTask(w) || s.PC == nil || len(d.Choices) > 0 || d.Secret != nil {
		return d, false
	}
	open := s.openItems(w.BotID)
	empty := strings.TrimSpace(d.Text) == ""
	if len(open) == 0 {
		if empty {
			d.Text = "Mình đã làm các bước trên máy tính (xem Các bước) nhưng chưa có kết quả mới để báo."
			return d, true
		}
		return d, false
	}
	if cp, ok := s.checkpoint(w, time.Time{}); ok {
		if !empty {
			cp.Text = strings.TrimSpace(d.Text) + "\n\n" + checkpointText
		}
		return cp, true
	}
	if empty {
		var xs []string
		for _, it := range open {
			xs = append(xs, it.Label)
		}
		d.Text = "Mình chưa xong: còn " + strings.Join(xs, ", ") + ". Mình đã thử nhiều lần trong lượt này nhưng chưa được; bạn xem Các bước để biết lỗi gần nhất."
		return d, true
	}
	return d, false
}

var pyFileRe = regexp.MustCompile(`(?:^|[\s;&|(])((?:[\w.-]+/)*[\w.-]+\.py)\b`)

// pyPrecheck compiles the .py files a command runs. It returns the error
// text for the bot, or "" (compiles, no scripts, or no python on the host).
func (s *Service) pyPrecheck(ctx context.Context, bot, cmd string) string {
	var files []string
	seen := map[string]bool{}
	for _, m := range pyFileRe.FindAllStringSubmatch(cmd, -1) {
		f := m[1]
		if seen[f] {
			continue
		}
		seen[f] = true
		if _, _, err := s.PC.ReadFile(bot, f); err == nil {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	r, err := s.PC.Check(cctx, bot, pyCheckCmd+strings.Join(files, " "))
	if err != nil || r.ExitCode == 0 || !strings.Contains(r.Output, "Error") {
		return ""
	}
	return "Chưa chạy lệnh: hệ thống kiểm tra cú pháp (biên dịch python3) và script LỖI, chạy sẽ hỏng ngay.\n" + untrustedOut +
		truncateResult(r.Output, 2000) + "\nSửa đúng dòng lỗi bằng computer_edit_file (old = dòng sai nguyên văn, new = dòng đúng), rồi in lại dòng TOOL computer_run."
}

func bigFileHint(path string, n int) string {
	if n < bigFileBytes || !strings.HasSuffix(path, ".py") {
		return ""
	}
	return fmt.Sprintf(" File lớn (%d byte): lần sau sửa bằng computer_edit_file, không ghi lại cả file; nên tách module nhỏ.", n)
}

func (s *Service) appendFile(bot, path, add string) (tools.Result, error) {
	old, cut, err := s.PC.ReadFile(bot, path)
	if err != nil {
		old, cut = "", false // first chunk
	}
	if cut {
		return tools.Result{}, errors.New("file quá lớn để ghi nối")
	}
	if err := s.PC.WriteFile(bot, path, old+add); err != nil {
		return tools.Result{}, err
	}
	s.noteWrite(bot, path)
	return tools.Result{Summary: "Ghi nối " + clean(path, 80), Text: fmt.Sprintf("Đã ghi nối vào %s (+%d byte, tổng %d byte).", path, len(add), len(old)+len(add))}, nil
}

func (s *Service) editFile(bot, arg string) (tools.Result, error) {
	var x struct{ Path, Old, New string }
	if json.Unmarshal([]byte(arg), &x) != nil || x.Old == "" {
		return tools.Result{}, errors.New("cần path, old, new")
	}
	txt, cut, err := s.PC.ReadFile(bot, x.Path)
	if err != nil {
		return tools.Result{}, err
	}
	if cut {
		return tools.Result{}, errors.New("file quá lớn để sửa từng chỗ; ghi lại theo phần")
	}
	switch n := strings.Count(txt, x.Old); n {
	case 0:
		return tools.Result{}, errors.New("không tìm thấy đoạn old trong file (phải đúng nguyên văn, kể cả khoảng trắng); đọc lại file bằng computer_read_file")
	case 1:
	default:
		return tools.Result{}, fmt.Errorf("đoạn old xuất hiện %d lần; thêm dòng xung quanh cho duy nhất", n)
	}
	if err := s.PC.WriteFile(bot, x.Path, strings.Replace(txt, x.Old, x.New, 1)); err != nil {
		return tools.Result{}, err
	}
	s.noteWrite(bot, x.Path)
	return tools.Result{Summary: "Sửa " + clean(x.Path, 80), Text: "Đã sửa 1 chỗ trong " + x.Path + "."}, nil
}
