package computer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syncFixture(t *testing.T, run func(string) execOut) *Railway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var in struct {
			Query     string
			Variables map[string]any
		}
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			t.Error(err)
			return
		}
		var data any
		switch {
		case strings.Contains(in.Query, "sandboxCreate("):
			data = map[string]any{"sandboxCreate": map[string]string{"id": "fixture-sync", "status": "RUNNING"}}
		case strings.Contains(in.Query, "sandboxExec("):
			data = map[string]any{"sandboxExec": run(in.Variables["command"].(string))}
		case strings.Contains(in.Query, "sandboxDestroy("):
			data = map[string]any{"sandboxDestroy": map[string]string{"id": "fixture-sync", "status": "DESTROYED"}}
		default:
			t.Error("unexpected sync fixture operation")
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return &Railway{Endpoint: srv.URL, Token: "local-fixture", EnvironmentID: "env1"}
}

func TestRailwaySyncRejectsHTTPFailureWithSuccessBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"sandboxExec": syncSuccess("")}})
	}))
	defer srv.Close()
	rw := &Railway{Endpoint: srv.URL}
	if err := rw.push(context.Background(), "fixture", t.TempDir(), nil); err == nil {
		t.Fatal("HTTP failure accepted as a sync receipt")
	}
}

func TestRailwaySyncRelayFallbackRequiresConfirmedChunks(t *testing.T) {
	remote := t.TempDir()
	if err := os.WriteFile(filepath.Join(remote, "ok"), []byte("remote"), 0600); err != nil {
		t.Fatal(err)
	}
	tgz, err := packHome(remote)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(tgz)
	for _, direction := range []string{"push", "pull"} {
		for _, kind := range []string{"timeout", "truncated"} {
			t.Run(direction+"/"+kind, func(t *testing.T) {
				rl := NewRelay("https://fixture.invalid")
				chunks := 0
				rw := syncFixture(t, func(cmd string) execOut {
					if strings.Contains(cmd, "python3 -c") {
						if direction == "pull" {
							start := strings.Index(cmd, RelayPath)
							rest := cmd[start:]
							end := strings.Index(rest, "'")
							rec := httptest.NewRecorder()
							rl.ServeHTTP(rec, httptest.NewRequest("PUT", rest[:end], bytes.NewReader(tgz)))
							if rec.Code != 204 {
								t.Error("fixture relay upload", rec.Code)
							}
						}
						o := syncFault(kind, syncSuccess(""))
						o.Stderr = "fixture-private-error-canary"
						return o
					}
					chunks++
					if strings.Contains(cmd, "wc -c") {
						return syncSuccess(fmt.Sprint(len(encoded)))
					}
					if strings.HasPrefix(cmd, "tail -c") {
						return syncSuccess(encoded)
					}
					return syncSuccess("")
				})
				rw.Relay = rl
				var logs bytes.Buffer
				original := log.Writer()
				log.SetOutput(&logs)
				defer log.SetOutput(original)
				if direction == "push" {
					err = rw.push(context.Background(), "fixture", t.TempDir(), tgz)
				} else {
					var got []byte
					got, err = rw.fetchHome(context.Background(), "fixture", Spec{Home: t.TempDir(), Limits: DefaultLimits()})
					if !bytes.Equal(got, tgz) {
						t.Error("fallback bytes differ")
					}
				}
				if err != nil || chunks == 0 {
					t.Fatal("unconfirmed relay result bypassed fallback", err, chunks)
				}
				if strings.Contains(logs.String(), "fixture-private-error-canary") {
					t.Fatal("remote stderr leaked into relay log")
				}
			})
		}
	}
}

func syncSuccess(stdout string) execOut { code := 0; return execOut{ExitCode: &code, Stdout: stdout} }

func syncFault(kind string, out execOut) execOut {
	switch kind {
	case "exit":
		code := 7
		out.ExitCode = &code
	case "missing-exit":
		out.ExitCode = nil
	case "timeout":
		out.TimedOut = true
	case "truncated":
		out.Truncated = true
	}
	return out
}

func TestRailwaySyncPushRefusesUnconfirmedStages(t *testing.T) {
	for _, stage := range []string{"prepare", "chunk", "extract"} {
		for _, kind := range []string{"exit", "missing-exit", "timeout", "truncated"} {
			t.Run(stage+"/"+kind, func(t *testing.T) {
				home := t.TempDir()
				if err := os.WriteFile(filepath.Join(home, "input.txt"), []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
				ownerRuns := 0
				failed := false
				afterFailure := 0
				rw := syncFixture(t, func(cmd string) execOut {
					if failed {
						afterFailure++
					}
					current := ""
					switch {
					case strings.HasPrefix(cmd, "rm -rf /home/bot"):
						current = "prepare"
					case strings.HasPrefix(cmd, "printf %s"):
						current = "chunk"
					case strings.Contains(cmd, "base64 -d /tmp/h.b64"):
						current = "extract"
					case strings.Contains(cmd, "env -i"):
						ownerRuns++
					}
					out := syncSuccess("")
					if current == stage {
						failed = true
						out = syncFault(kind, out)
					}
					return out
				})
				_, err := rw.Exec(context.Background(), Spec{Home: home, Cmd: "owner-command", Limits: DefaultLimits()})
				if err == nil || ownerRuns != 0 || afterFailure != 0 {
					t.Fatalf("sync failure did not stop before owner command: err=%v runs=%d subsequent=%d", err, ownerRuns, afterFailure)
				}
				reg.Lock()
				cached := reg.synced[home]
				delete(reg.ids, home)
				delete(reg.used, home)
				delete(reg.synced, home)
				reg.Unlock()
				if cached != "" {
					t.Fatal("failed push recorded as synchronized")
				}
			})
		}
	}
}

func TestRailwaySyncPullPreservesLocalHomeOnBadReceipt(t *testing.T) {
	remote := t.TempDir()
	if err := os.WriteFile(filepath.Join(remote, "remote.txt"), []byte("remote output"), 0600); err != nil {
		t.Fatal(err)
	}
	archive, err := packHome(remote)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(archive)
	for _, test := range []struct{ stage, kind string }{{"size", "exit"}, {"size", "missing-exit"}, {"size", "timeout"}, {"size", "truncated"}, {"size", "trailing"}, {"size", "negative"}, {"chunk", "exit"}, {"chunk", "missing-exit"}, {"chunk", "timeout"}, {"chunk", "truncated"}, {"chunk", "short"}} {
		t.Run(test.stage+"/"+test.kind, func(t *testing.T) {
			home := t.TempDir()
			keep := filepath.Join(home, "keep.txt")
			if err := os.WriteFile(keep, []byte("local original"), 0600); err != nil {
				t.Fatal(err)
			}
			rw := syncFixture(t, func(cmd string) execOut {
				if strings.Contains(cmd, "wc -c") {
					out := syncSuccess(fmt.Sprint(len(encoded)))
					if test.stage == "size" {
						out = syncFault(test.kind, out)
						if test.kind == "trailing" {
							out.Stdout += " extra"
						}
						if test.kind == "negative" {
							out.Stdout = "-4"
						}
					}
					return out
				}
				var off, n int
				if _, err := fmt.Sscanf(cmd, "tail -c +%d /tmp/o.b64 | head -c %d", &off, &n); err != nil {
					t.Error("unexpected chunk command", err)
					return syncFault("exit", syncSuccess(""))
				}
				out := syncSuccess(encoded[off-1 : min(off-1+n, len(encoded))])
				if test.stage == "chunk" {
					out = syncFault(test.kind, out)
					if test.kind == "short" {
						out.Stdout = out.Stdout[:len(out.Stdout)-4]
					}
				}
				return out
			})
			err := rw.pull(context.Background(), "fixture-sync", Spec{Home: home, Limits: DefaultLimits()})
			if err == nil {
				t.Fatal("unconfirmed download accepted")
			}
			b, err := os.ReadFile(keep)
			if err != nil || string(b) != "local original" {
				t.Fatal("local home replaced on failed receipt", err)
			}
			if _, err := os.Stat(filepath.Join(home, "remote.txt")); !os.IsNotExist(err) {
				t.Fatal("unconfirmed file published", err)
			}
		})
	}
}
