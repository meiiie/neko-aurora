package aurora

// Prove it works (idea from pstack by Lauren Tan, MIT; rewritten as Go
// checks): a checklist item closes only when a real file passes a check for
// its type. Video: a parseable MP4 with a moov/mvhd duration ≥ 1 s and media
// data (WebM/GIF: valid header and size). Sheet: an .xlsx with a worksheet of
// ≥ 2 rows, or a .csv with a header and consistent columns. Code: a .py that
// a command ran with exit 0. Deck/notes: validatePPTX. The results are kept
// in verify.txt in the bot's home so the owner can see what was checked.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path"
	"strings"
)

type verdict struct {
	ok  bool
	why string
}

const maxProbe = 64 << 20

// probeMP4 returns the duration in seconds of an MP4/MOV file.
func probeMP4(r io.ReaderAt, size int64) (float64, error) {
	var moov []byte
	mdat := false
	for off := int64(0); off+8 <= size; {
		h := make([]byte, 16)
		if _, err := r.ReadAt(h[:8], off); err != nil {
			return 0, err
		}
		n, typ, hdr := int64(binary.BigEndian.Uint32(h[:4])), string(h[4:8]), int64(8)
		if n == 1 {
			if _, err := r.ReadAt(h[8:16], off+8); err != nil {
				return 0, err
			}
			n, hdr = int64(binary.BigEndian.Uint64(h[8:16])), 16
		} else if n == 0 {
			n = size - off
		}
		if n < hdr || off+n > size {
			return 0, fmt.Errorf("hộp %q hỏng", typ)
		}
		switch typ {
		case "moov":
			if n > maxProbe {
				return 0, fmt.Errorf("moov quá lớn")
			}
			moov = make([]byte, n-hdr)
			if _, err := r.ReadAt(moov, off+hdr); err != nil {
				return 0, err
			}
		case "mdat":
			mdat = n > hdr+100
		}
		off += n
	}
	if moov == nil {
		return 0, fmt.Errorf("không có moov (file chưa ghi xong?)")
	}
	if !mdat {
		return 0, fmt.Errorf("không có dữ liệu hình")
	}
	i := bytes.Index(moov, []byte("mvhd"))
	if i < 0 || i+4+28 > len(moov) {
		return 0, fmt.Errorf("không có mvhd")
	}
	b := moov[i+4:]
	if b[0] == 1 {
		if len(b) < 32 {
			return 0, fmt.Errorf("mvhd ngắn")
		}
		ts, d := binary.BigEndian.Uint32(b[20:24]), binary.BigEndian.Uint64(b[24:32])
		if ts == 0 {
			return 0, fmt.Errorf("timescale 0")
		}
		return float64(d) / float64(ts), nil
	}
	ts, d := binary.BigEndian.Uint32(b[12:16]), binary.BigEndian.Uint32(b[16:20])
	if ts == 0 {
		return 0, fmt.Errorf("timescale 0")
	}
	return float64(d) / float64(ts), nil
}

func checkVideo(r io.ReaderAt, size int64, ext string) verdict {
	head := make([]byte, 12)
	if _, err := r.ReadAt(head, 0); err != nil {
		return verdict{why: "không đọc được"}
	}
	switch {
	case ext == ".mp4" || ext == ".mov" || ext == ".m4v":
		d, err := probeMP4(r, size)
		if err != nil {
			return verdict{why: "MP4 hỏng: " + err.Error()}
		}
		if d < 1 {
			return verdict{why: fmt.Sprintf("video chỉ dài %.1fs", d)}
		}
		return verdict{true, fmt.Sprintf("MP4 %.0fs, %s", d, kbSize(size))}
	case ext == ".webm" && bytes.HasPrefix(head, []byte{0x1a, 0x45, 0xdf, 0xa3}) && size > 10000,
		ext == ".gif" && bytes.HasPrefix(head, []byte("GIF8")) && size > 10000:
		return verdict{true, strings.ToUpper(ext[1:]) + " " + kbSize(size)}
	}
	return verdict{why: "không phải video hợp lệ"}
}

func kbSize(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// verifyArtifacts checks every deliverable in the home once.
func (s *Service) verifyArtifacts(bot string, c pcState) map[string]verdict {
	out := map[string]verdict{}
	set := func(k, p string, v verdict) {
		if cur, ok := out[k]; ok && cur.ok {
			return
		}
		if v.why != "" {
			v.why = p + ": " + v.why
		}
		out[k] = v
	}
	l, err := s.PC.List(bot, "")
	if err != nil {
		return out
	}
	for _, e := range l {
		if e.Dir || e.Size == 0 {
			continue
		}
		ext := strings.ToLower(path.Ext(e.Path))
		key := map[string]string{".pptx": "deck", ".mp4": "video", ".mov": "video", ".m4v": "video", ".webm": "video", ".gif": "video", ".xlsx": "sheet", ".csv": "sheet"}[ext]
		if key == "" {
			continue
		}
		f, size, err := s.PC.OpenFile(bot, e.Path)
		if err != nil {
			continue
		}
		switch key {
		case "deck":
			rep := validatePPTX(f, size, c.targetMin)
			fails := strings.Join(rep.Fails, " ")
			set("deck", e.Path, verdict{rep.Slides > 0 && rep.ok(), map[bool]string{true: fmt.Sprintf("%d slide, đạt kiểm tra", rep.Slides), false: clean(fails, 160)}[rep.ok()]})
			notes := rep.Slides > 0 && !strings.Contains(fails, "ghi chú") && !strings.Contains(fails, "notes")
			set("notes", e.Path, verdict{notes, map[bool]string{true: fmt.Sprintf("%d từ ghi chú", rep.NotesWords), false: "ghi chú chưa đạt"}[notes]})
		case "video":
			set("video", e.Path, checkVideo(f, size, ext))
		case "sheet":
			set("sheet", e.Path, checkSheet(f, size, ext))
		}
		f.Close()
	}
	for p := range c.okScripts {
		set("code", p, verdict{true, "chạy xong, mã thoát 0"})
		break
	}
	if _, ok := out["code"]; !ok && len(c.written) > 0 {
		out["code"] = verdict{why: "chưa có script .py nào chạy thành công"}
	}
	return out
}

// writeVerify keeps the current checklist results in verify.txt, including
// revoked proof after a restart has cleared the in-memory cache.
func (s *Service) writeVerify(bot string) {
	c := s.pcSnap(bot)
	if len(c.checklist) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("Kiểm tra tự động của Aurora (file thật trên máy tính)\n")
	for _, it := range c.checklist {
		mark := "✗"
		if it.Done {
			mark = "✓"
		} else if it.Impossible != "" {
			mark = "—"
		}
		fmt.Fprintf(&b, "%s %s", mark, it.Label)
		if it.Why != "" {
			b.WriteString(" — " + it.Why)
		}
		if it.Impossible != "" {
			b.WriteString(" (không thể: " + it.Impossible + ")")
		}
		b.WriteString("\n")
	}
	txt := b.String()
	if txt == c.verifyTxt {
		return
	}
	if s.PC.WriteFile(bot, "verify.txt", txt) == nil {
		s.pcDo(bot, func(st *pcState) { st.verifyTxt = txt })
	}
}
