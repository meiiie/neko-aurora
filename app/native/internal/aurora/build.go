package aurora

// Build loop for work on the bot's computer (live pptx task 2026-10-23):
//   - claims of finished files are checked against the computer: a download
//     link to a file that does not exist is rewritten, a "Xong" with no real
//     file gets an honest footer (and the final reply is asked once more);
//   - approved commands run strictly one at a time in approval order, and a
//     new command card supersedes an older pending one of the same bot;
//   - every model call sees the cwd rule, the files written recently and the
//     last command's exit code;
//   - after each approved command the bot gets exit code + output and must
//     fix and rerun, bounded by maxFixes failures / maxBuildRuns commands.

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"nekoaurora.local/native/internal/ledger"
)

const (
	maxFixes     = 6  // consecutive failed commands before the bot must stop and report
	maxBuildRuns = 14 // commands per owner request
)

type pcState struct {
	written   []string // recent computer_write_file paths, newest last
	lastCmd   string
	lastExit  int
	ran       bool
	fixes     int
	runs      int
	research  string // sources + step results of the last researched turn
	checklist []checkItem
	targetMin float64
}

type pcBook struct {
	mu sync.Mutex
	st map[string]*pcState
	q  map[string]chan func()
}

func (s *Service) pcDo(bot string, fn func(*pcState)) {
	s.pcb.mu.Lock()
	defer s.pcb.mu.Unlock()
	if s.pcb.st == nil {
		s.pcb.st = map[string]*pcState{}
	}
	st := s.pcb.st[bot]
	if st == nil {
		st = &pcState{}
		s.pcb.st[bot] = st
	}
	fn(st)
}

func (s *Service) pcSnap(bot string) (c pcState) {
	s.pcDo(bot, func(st *pcState) { c = *st; c.written = append([]string(nil), st.written...) })
	return
}

func (s *Service) noteWrite(bot, p string) {
	p = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(p), "/home/bot"), "/")
	s.pcDo(bot, func(st *pcState) {
		for i, w := range st.written {
			if w == p {
				st.written = append(st.written[:i], st.written[i+1:]...)
				break
			}
		}
		st.written = append(st.written, p)
		if len(st.written) > 12 {
			st.written = st.written[len(st.written)-12:]
		}
	})
}

func (s *Service) noteRun(bot, cmd string, exit int) {
	s.pcDo(bot, func(st *pcState) {
		st.lastCmd, st.lastExit, st.ran = cmd, exit, true
		st.runs++
		if exit != 0 {
			st.fixes++
		} else {
			st.fixes = 0
		}
	})
}

// newRequest resets the bounds when the owner asks for something new.
func (s *Service) newRequest(bot, text string, computer bool) {
	s.pcDo(bot, func(st *pcState) {
		st.fixes, st.runs = 0, 0
		if !computer { // a new, unrelated request: the old checklist is done with
			st.checklist, st.targetMin = nil, 0
			return
		}
		st.checklist = mergeChecklist(st.checklist, text)
		if m := targetMinutes(text); m > 0 {
			st.targetMin = m
		}
	})
}

// buildExhausted reports whether the bot must stop running commands.
func (s *Service) buildExhausted(bot string) bool {
	c := s.pcSnap(bot)
	return c.fixes >= maxFixes || c.runs >= maxBuildRuns
}

// computerContext is the planning context for the next command: cwd rule,
// what is really in the home and what was written / run last.
func (s *Service) computerContext(bot string) string {
	if s.PC == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nMÁY TÍNH — NGỮ CẢNH THẬT: mỗi computer_run chạy trong shell MỚI với thư mục hiện tại là /home/bot (cd không giữ sang lệnh sau; đường dẫn tương đối tính từ /home/bot; computer_write_file path=\"a/b.py\" tạo /home/bot/a/b.py và tự tạo thư mục). Lệnh dùng file nào thì file đó phải có trong danh sách dưới.")
	c := s.pcSnap(bot)
	if len(c.written) > 0 {
		sb.WriteString("\nFile bạn đã ghi gần đây: /home/bot/" + strings.Join(c.written, ", /home/bot/"))
	}
	if l, err := s.PC.List(bot, ""); err == nil && len(l) > 0 {
		var xs []string
		for _, e := range l {
			if len(xs) >= 40 {
				xs = append(xs, "…")
				break
			}
			if e.Summary {
				xs = append(xs, fmt.Sprintf("%s/ (%d file, %d B — thư mục lớn, đã gộp)", e.Path, e.Files, e.Size))
			} else if e.Dir {
				xs = append(xs, e.Path+"/")
			} else {
				xs = append(xs, fmt.Sprintf("%s (%d B)", e.Path, e.Size))
			}
		}
		sb.WriteString("\nĐang có trong /home/bot: " + strings.Join(xs, ", "))
	} else {
		sb.WriteString("\n/home/bot hiện đang trống.")
	}
	if c.ran {
		fmt.Fprintf(&sb, "\nLệnh gần nhất: `%s` → mã thoát %d.", clean(c.lastCmd, 200), c.lastExit)
	}
	if c.fixes > 0 {
		fmt.Fprintf(&sb, " Đã lỗi liên tiếp %d/%d lần.", c.fixes, maxFixes)
	}
	if len(c.checklist) > 0 {
		s.refreshChecklist(bot)
		sb.WriteString(checklistText(s.pcSnap(bot).checklist))
	}
	return sb.String()
}

var (
	dlLinkRe = regexp.MustCompile(`\[([^\]\n]{1,200})\]\((/app/api/agents/([A-Za-z0-9_-]+)/computer/download\?path=([^)\s]+))\)`)
	doneRe   = regexp.MustCompile(`(?i)(\bxong\s*[!.:]|đã (tạo|làm|dựng|render|ghép|xuất|hoàn thành)( \S+){0,3} xong|hoàn tất|đã hoàn thành|link tải|tải (về )?(tại|ở đây)|here is the (file|link)|download link)`)
	fileWord = regexp.MustCompile(`(?i)(pptx|mp4|video|slide|file|tệp|link|\.csv|\.xlsx|\.pdf|\.png)`)
)

// verifyClaims checks every download link against the bot's computer and
// rewrites links to files that do not exist. strict also flags a done-claim
// about files that carries no verified link. bad lists what was wrong.
func (s *Service) verifyClaims(bot, text string, strict bool) (string, []string) {
	if s.PC == nil {
		return text, nil
	}
	var bad []string
	okLinks := 0
	text = dlLinkRe.ReplaceAllStringFunc(text, func(m string) string {
		g := dlLinkRe.FindStringSubmatch(m)
		p, err := url.QueryUnescape(g[4])
		if err != nil {
			p = g[4]
		}
		p = strings.TrimPrefix(strings.TrimPrefix(p, "/home/bot"), "/")
		if g[3] == bot {
			if f, size, err := s.PC.OpenFile(bot, p); err == nil {
				f.Close()
				if size > 0 {
					okLinks++
					return m
				}
			}
		}
		bad = append(bad, p)
		return "**" + g[1] + "** (chưa có file này trên máy tính, chưa tạo được)"
	})
	if strict && okLinks == 0 && doneRe.MatchString(text) && fileWord.MatchString(text) {
		bad = append(bad, "(báo xong nhưng không có file nào được xác minh)")
	}
	return text, bad
}

const unverifiedFooter = "⚠️ Mình chưa xác minh được file kết quả nào trên máy tính, nên việc này CHƯA xong."

// computerTask: replies whose claims must be backed by the computer.
func computerTask(w ledger.Work) bool {
	return strings.HasPrefix(w.ClientNonce, "computer-") || wantsComputer.MatchString(w.Text)
}

func verifyNudge(bad []string, ctx string) string {
	return "[Hệ thống] Câu trả lời vừa rồi báo xong/đưa link cho thứ KHÔNG có trên máy tính: " + strings.Join(bad, ", ") +
		". Chỉ được báo xong và đưa link khi file có thật trong danh sách dưới. Nếu còn việc thì làm tiếp bằng TOOL (computer_write_file/computer_run); nếu không thì nói trung thực phần nào chưa xong." + ctx
}

// planIntro keeps the plan's short intro: the model sometimes narrates a
// whole imagined run there ("Đã tạo xong pptx… Xong! Link tải…").
func planIntro(t string) string {
	t = strings.TrimSpace(t)
	if i := strings.Index(t, "\n"); i > 0 {
		t = t[:i]
	}
	if r := []rune(t); len(r) > 240 {
		t = string(r[:240]) + "…"
	}
	if loc := doneRe.FindStringIndex(t); loc != nil {
		cut := strings.LastIndexAny(t[:loc[0]], ".!:—")
		if cut <= 0 {
			return "Mình làm theo kế hoạch dưới đây."
		}
		t = strings.TrimSpace(t[:cut+1])
	}
	return t
}

// enqueueCmd runs approved commands of one bot strictly one at a time in
// approval order (live: a second card approved while the first ran raced it).
func (s *Service) enqueueCmd(bot string, job func()) {
	s.pcb.mu.Lock()
	if s.pcb.q == nil {
		s.pcb.q = map[string]chan func(){}
	}
	q := s.pcb.q[bot]
	if q == nil {
		q = make(chan func(), 64)
		s.pcb.q[bot] = q
		go func() {
			for j := range q {
				j()
			}
		}()
	}
	s.pcb.mu.Unlock()
	q <- job
}

// supersedeCommands retires older pending command cards of the bot: only
// the newest command can be approved, so commands never run out of order.
func (s *Service) supersedeCommands(bot, keep string) {
	for _, w := range s.L.Snapshot().Work {
		if w.BotID != bot || w.RequestID == keep || w.Direct == nil || w.Direct.Approval == nil {
			continue
		}
		if a := w.Direct.Approval; a.Kind != "computer" || a.State != "pending" {
			continue
		}
		_, _ = s.L.DirectDecide(w.RequestID, func(a *ledger.DirectApproval, _ *ledger.DirectRun) error {
			if a.State != "pending" {
				return ErrConflict
			}
			a.State, a.DecidedAt = "superseded", time.Now().UTC().Format(time.RFC3339Nano)
			a.Outcome = "Lệnh này đã được thay bằng lệnh mới hơn bên dưới, mình không chạy."
			return nil
		})
	}
}
