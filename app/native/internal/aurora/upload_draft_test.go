package aurora

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/computer"
	"nekoaurora.local/native/internal/ledger"
)

func TestImageUploadKeepsEarlierDraftAndWorkspace(t *testing.T) {
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
	srv := httptest.NewServer(s)
	defer srv.Close()
	bot := l.Snapshot().Bots[0].ID
	upload := func(scope, content string) (int, string) {
		b, _ := json.Marshal(map[string]string{"name": "Dự án.jpg", "data": std64([]byte(content)), "workspaceId": scope})
		r := post(t, srv.URL+"/app/api/agents/"+bot+"/computer/upload", string(b))
		defer r.Body.Close()
		var v struct {
			Path string `json:"path"`
		}
		json.NewDecoder(r.Body).Decode(&v)
		return r.StatusCode, v.Path
	}
	if code, _ := upload("other-workspace", "wrong"); code != 409 {
		t.Fatal("workspace boundary", code)
	}
	code, first := upload(s.workspaceID, "first bytes")
	if code != 200 {
		t.Fatal(code)
	}
	code, second := upload(s.workspaceID, "second bytes")
	if code != 200 || first == second {
		t.Fatal("same name replaced a draft attachment", code)
	}
	for path, want := range map[string]string{first: "first bytes", second: "second bytes"} {
		f, _, err := s.PC.OpenFile(bot, path)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(b) != want {
			t.Fatal("uploaded bytes changed", err)
		}
		if !strings.HasSuffix(path, "Dự-án.jpg") || len(imageAuto("[Ảnh: "+path+"]")) != 1 {
			t.Fatal("attachment not readable by image tool")
		}
	}
}
