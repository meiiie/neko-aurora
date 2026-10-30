package aurora

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"nekoaurora.local/native/internal/computer"
	"nekoaurora.local/native/internal/ledger"
)

type downloadChangingWriter struct {
	*httptest.ResponseRecorder
	change func()
}

func (w *downloadChangingWriter) Write(b []byte) (int, error) {
	if w.change != nil {
		f := w.change
		w.change = nil
		f()
	}
	return w.ResponseRecorder.Write(b)
}
func TestComputerDownloadSnapshotPreventsMixedBytes(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		t.Fatal(err)
	}
	s.PC, err = computer.New(filepath.Join(dir, "computers"), computer.NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	bot := l.Snapshot().Bots[0].ID
	before := bytes.Repeat([]byte("A"), 256<<10)
	after := bytes.Repeat([]byte("B"), len(before))
	if err := s.PC.CreateFileOnce(bot, "report.bin", before); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.PC.OpenFile(bot, "report.bin")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	f.Close()
	changed := false
	w := &downloadChangingWriter{ResponseRecorder: httptest.NewRecorder(), change: func() {
		changed = true
		if err := os.WriteFile(name, after, 0600); err != nil {
			t.Fatal(err)
		}
	}}
	s.ServeHTTP(w, httptest.NewRequest("GET", "/app/api/agents/"+bot+"/computer/download?path=report.bin", nil))
	if !changed || w.Code != 200 {
		t.Fatal("download did not stream", changed, w.Code)
	}
	data, _ := io.ReadAll(w.Result().Body)
	if !bytes.Equal(data, before) {
		t.Fatal("download combined bytes from different file versions")
	}
}

func TestComputerDownloadHeadersScopeAndErrors(t *testing.T) {
	dir := t.TempDir()
	l, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := New(ctx, l, filepath.Join(dir, "aurora"), Options{NoBriefLoop: true, DisableCloud: true})
	if err != nil {
		t.Fatal(err)
	}
	s.PC, err = computer.New(filepath.Join(dir, "computers"), computer.NewBwrap())
	if err != nil {
		t.Fatal(err)
	}
	bot := l.Snapshot().Bots[0].ID
	data := []byte("download fixture")
	name := "Báo cáo.bin"
	if err := s.PC.CreateFileOnce(bot, name, data); err != nil {
		t.Fatal(err)
	}
	base := "/app/api/agents/" + bot + "/computer/download?path=" + url.QueryEscape(name)
	get := func(method, query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, base+query, nil))
		return w
	}
	for _, method := range []string{"GET", "HEAD"} {
		w := get(method, "&workspaceId="+url.QueryEscape(s.workspaceID))
		if w.Code != 200 {
			t.Fatal(method, w.Code)
		}
		h := w.Header()
		_, params, err := mime.ParseMediaType(h.Get("Content-Disposition"))
		sum := sha256.Sum256(data)
		if err != nil || params["filename"] != name || h.Get("Content-Length") != strconv.Itoa(len(data)) || h.Get("ETag") != `"`+hex.EncodeToString(sum[:])+`"` || h.Get("X-Aurora-Workspace") != s.workspaceID || h.Get("X-Aurora-Bot") != bot || h.Get("Cache-Control") != "no-store" || h.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("download contract", method, h)
		}
		if method == "GET" && !bytes.Equal(w.Body.Bytes(), data) || method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("body", method)
		}
	}
	for _, scope := range []string{"&workspaceId=other", "&workspaceId=", "&workspaceId=" + url.QueryEscape(s.workspaceID) + "&workspaceId=other"} {
		w := get("GET", scope)
		if w.Code != 409 || w.Header().Get("Content-Disposition") != "" {
			t.Fatal("scope allowed", w.Code)
		}
	}
	s.PC.Limits.HomeBytes = 3
	w := get("GET", "")
	if w.Code != 413 || w.Header().Get("Content-Disposition") != "" {
		t.Fatal("quota accepted", w.Code)
	}
	s.PC.Limits.HomeBytes = 1 << 20
	a, _ := s.PC.OpenDownload(ctx, bot, name)
	defer a.Close()
	b, _ := s.PC.OpenDownload(ctx, bot, name)
	defer b.Close()
	w = get("GET", "")
	if w.Code != 503 || w.Header().Get("Content-Disposition") != "" {
		t.Fatal("busy not surfaced", w.Code)
	}
	a.Close()
	b.Close()
	base = "/app/api/agents/" + bot + "/computer/download?path=missing"
	w = get("GET", "")
	if w.Code != 404 || w.Header().Get("Content-Disposition") != "" {
		t.Fatal("missing file", w.Code)
	}
	base = "/app/api/agents/absent/computer/download?path=file"
	if w := get("GET", ""); w.Code != 404 {
		t.Fatal("missing bot", w.Code)
	}
}
