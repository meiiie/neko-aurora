package computer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func forgetRailwayHome(home string) {
	reg.Lock()
	defer reg.Unlock()
	delete(reg.ids, home)
	delete(reg.used, home)
	delete(reg.synced, home)
}

func TestRailwayRecoveryUnknownBlocksRestartAndWarm(t *testing.T) {
	for _, kind := range []string{"missing-exit", "timeout", "negative-exit"} {
		t.Run(kind, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { forgetRailwayHome(home) })
			calls, runs := 0, 0
			rw := syncFixture(t, func(cmd string) execOut {
				calls++
				if strings.Contains(cmd, "env -i") {
					runs++
					if kind == "negative-exit" {
						code := -1
						return execOut{ExitCode: &code}
					}
					return syncFault(kind, syncSuccess("captured"))
				}
				return syncSuccess("")
			})
			spec := Spec{Home: home, Cmd: "owner-private-command", Limits: DefaultLimits()}
			if _, err := rw.Exec(context.Background(), spec); err == nil {
				t.Fatal("unknown command acknowledged")
			}
			if runs != 1 {
				t.Fatal("owner invocation missing", runs)
			}
			marker, err := os.ReadFile(home + ".sandbox-pending")
			if err != nil || strings.Contains(string(marker), spec.Cmd) || strings.Contains(string(marker), rw.Token) {
				t.Fatal("pending receipt absent or contains private inputs", err)
			}
			forgetRailwayHome(home)
			before := calls
			restarted := *rw
			if _, err := restarted.Exec(context.Background(), spec); err == nil {
				t.Fatal("unknown command replayed after restart")
			}
			if err := restarted.Warm(context.Background(), home); err == nil {
				t.Fatal("warm overwrote pending VM")
			}
			if calls != before || runs != 1 {
				t.Fatal("pending guard performed remote operations", calls, before, runs)
			}
		})
	}
}

func TestRailwayRecoveryPullDoesNotTrustStdoutMarker(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { forgetRailwayHome(home) })
	remote := t.TempDir()
	if err := os.WriteFile(filepath.Join(remote, "result.txt"), []byte("new result"), 0600); err != nil {
		t.Fatal(err)
	}
	archive, err := packHome(remote)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(archive)
	runs, pulls := 0, 0
	rw := syncFixture(t, func(cmd string) execOut {
		switch {
		case strings.Contains(cmd, "env -i"):
			runs++
			return syncSuccess("result without a marker")
		case strings.Contains(cmd, "wc -c"):
			pulls++
			return syncSuccess(fmt.Sprint(len(encoded)))
		case strings.HasPrefix(cmd, "tail -c"):
			return syncSuccess(encoded)
		default:
			return syncSuccess("")
		}
	})
	if _, err := rw.Exec(context.Background(), Spec{Home: home, Cmd: "owner-command", Limits: DefaultLimits()}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, "result.txt"))
	if err != nil || string(got) != "new result" || runs != 1 || pulls != 1 {
		t.Fatal("missing stdout marker bypassed file recovery", err, runs, pulls)
	}
}

func TestRailwayRecoveryPullFailureIsNotCompletion(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { forgetRailwayHome(home) })
	runs := 0
	rw := syncFixture(t, func(cmd string) execOut {
		if strings.Contains(cmd, "env -i") {
			runs++
			return syncSuccess("result\n" + changedMark)
		}
		if strings.Contains(cmd, "wc -c") {
			return syncFault("exit", syncSuccess(""))
		}
		return syncSuccess("")
	})
	spec := Spec{Home: home, Cmd: "owner-command", Limits: DefaultLimits()}
	if _, err := rw.Exec(context.Background(), spec); err == nil {
		t.Fatal("failed pull reported completed success")
	}
	if _, err := rw.Exec(context.Background(), spec); err == nil || runs != 1 {
		t.Fatal("failed pull allowed command replay", err, runs)
	}
}

func TestRailwayResetFailurePreservesLocalHome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "fixture unavailable", 500) }))
	defer srv.Close()
	rw := &Railway{Endpoint: srv.URL, Token: "fixture", EnvironmentID: "fixture"}
	m, err := New(t.TempDir(), rw)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.WriteFile("bot", "keep.txt", "local recovery copy"); err != nil {
		t.Fatal(err)
	}
	home, err := m.home("bot")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { forgetRailwayHome(home) })
	if err = os.WriteFile(idFile(home), []byte("existing-sandbox"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.Reset("bot"); err == nil {
		t.Fatal("reset acknowledged without remote destruction")
	}
	got, _, err := m.ReadFile("bot", "keep.txt")
	if err != nil || got != "local recovery copy" {
		t.Fatal("failed reset destroyed local recovery data", err)
	}
	if _, err = os.Stat(idFile(home)); err != nil {
		t.Fatal("failed reset lost remote identity", err)
	}
}

func TestRailwayRecoveryRetriesOnlyFilesAndProtectsLocalChanges(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { forgetRailwayHome(home) })
			remote := t.TempDir()
			if err := os.WriteFile(filepath.Join(remote, "result.txt"), []byte("remote result"), 0600); err != nil {
				t.Fatal(err)
			}
			archive, err := packHome(remote)
			if err != nil {
				t.Fatal(err)
			}
			encoded := base64.StdEncoding.EncodeToString(archive)
			failPull, runs, pulls := true, 0, 0
			rw := syncFixture(t, func(cmd string) execOut {
				if strings.Contains(cmd, "env -i") {
					runs++
					return syncSuccess("done")
				}
				if strings.Contains(cmd, "wc -c") {
					pulls++
					if failPull {
						return syncFault("exit", syncSuccess(""))
					}
					return syncSuccess(fmt.Sprint(len(encoded)))
				}
				if strings.HasPrefix(cmd, "tail -c") {
					return syncSuccess(encoded)
				}
				return syncSuccess("")
			})
			spec := Spec{Home: home, Cmd: "owner-once", Limits: DefaultLimits()}
			if _, err = rw.Exec(context.Background(), spec); !errors.Is(err, ErrRecoveryPending) {
				t.Fatal(err)
			}
			if st := rw.RecoveryStatus(home); st == nil || !st.CanRecover || st.State != "files" {
				t.Fatal("missing file recovery state", st)
			}
			forgetRailwayHome(home)
			if changed {
				if err = os.WriteFile(filepath.Join(home, "local.txt"), []byte("owner edit"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			failPull = false
			restarted := *rw
			err = restarted.Recover(context.Background(), spec)
			if runs != 1 {
				t.Fatal("recovery replayed owner command", runs)
			}
			if changed {
				if !errors.Is(err, ErrRecoveryConflict) || pulls != 1 {
					t.Fatal("local edit was not fenced", err, pulls)
				}
				got, readErr := os.ReadFile(filepath.Join(home, "local.txt"))
				if readErr != nil || string(got) != "owner edit" {
					t.Fatal("local edit lost", readErr)
				}
				if !pendingExists(home) {
					t.Fatal("conflict cleared pending state")
				}
			} else {
				if err != nil || pulls != 2 || pendingExists(home) {
					t.Fatal("file recovery incomplete", err, pulls)
				}
				got, readErr := os.ReadFile(filepath.Join(home, "result.txt"))
				if readErr != nil || string(got) != "remote result" {
					t.Fatal("recovered bytes differ", readErr)
				}
				if err = restarted.Recover(context.Background(), spec); err != nil || pulls != 2 {
					t.Fatal("completed recovery repeated transfer", err, pulls)
				}
			}
		})
	}
}

func TestRailwayRecoveryInvalidRecordAndPendingCapacity(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { forgetRailwayHome(home) })
	calls := 0
	rw := syncFixture(t, func(string) execOut { calls++; return syncSuccess("") })
	for _, raw := range []string{"", "{", strings.Repeat(" ", 4097)} {
		if err := os.WriteFile(pendingFile(home), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if st := rw.RecoveryStatus(home); st == nil || st.CanRecover || st.State != "blocked" {
			t.Fatal("invalid record did not block", st)
		}
		if err := rw.Recover(context.Background(), Spec{Home: home}); !errors.Is(err, ErrRecoveryPending) {
			t.Fatal(err)
		}
		if _, err := rw.Exec(context.Background(), Spec{Home: home, Cmd: "owner-command"}); !errors.Is(err, ErrRecoveryPending) {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("invalid record dispatched remote commands", calls)
	}
	reg.Lock()
	oldIDs, oldUsed, oldSynced := reg.ids, reg.used, reg.synced
	reg.ids = map[string]string{home: "protected"}
	reg.used = map[string]time.Time{home: time.Now()}
	reg.synced = map[string]string{}
	reg.Unlock()
	defer func() { reg.Lock(); reg.ids, reg.used, reg.synced = oldIDs, oldUsed, oldSynced; reg.Unlock() }()
	rw.Max = 1
	if _, err := rw.sandbox(context.Background(), filepath.Join(t.TempDir(), "next"), false); !errors.Is(err, ErrUnavailable) {
		t.Fatal("pending VM was evicted", err)
	}
	reg.Lock()
	id := reg.ids[home]
	reg.Unlock()
	if id != "protected" {
		t.Fatal("pending VM identity was lost")
	}
	if err := os.Remove(pendingFile(home)); err != nil {
		t.Fatal(err)
	}
	gate := railwayHomeGate(home)
	gate.Lock()
	_, err := rw.sandbox(context.Background(), filepath.Join(t.TempDir(), "next-busy"), false)
	gate.Unlock()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("busy VM was evicted before pending record was written", err)
	}
}

func TestRailwayRecoveryLostResponseAndFailedPhaseWrite(t *testing.T) {
	t.Run("lost owner response", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		if err := os.Mkdir(home, 0700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { forgetRailwayHome(home) })
		runs, requests := 0, 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			requests++
			var in struct {
				Query     string
				Variables map[string]any
			}
			if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
				t.Error(err)
				return
			}
			var data any
			if strings.Contains(in.Query, "sandboxCreate(") {
				data = map[string]any{"sandboxCreate": map[string]string{"id": "lost-response", "status": "RUNNING"}}
			} else {
				cmd, _ := in.Variables["command"].(string)
				if strings.Contains(cmd, "env -i") {
					runs++
					http.Error(w, "fixture lost response", 502)
					return
				}
				data = map[string]any{"sandboxExec": syncSuccess("")}
			}
			json.NewEncoder(w).Encode(map[string]any{"data": data})
		}))
		defer srv.Close()
		rw := &Railway{Endpoint: srv.URL, Token: "fixture", EnvironmentID: "env1"}
		spec := Spec{Home: home, Cmd: "owner-command", Limits: DefaultLimits()}
		if _, err := rw.Exec(context.Background(), spec); !errors.Is(err, ErrRecoveryPending) || runs != 1 {
			t.Fatal("lost receipt not fenced", err, runs)
		}
		before := requests
		forgetRailwayHome(home)
		if _, err := rw.Exec(context.Background(), spec); !errors.Is(err, ErrRecoveryPending) {
			t.Fatal(err)
		}
		if err := rw.Recover(context.Background(), spec); !errors.Is(err, ErrRecoveryPending) || requests != before {
			t.Fatal("unknown outcome performed remote recovery", err, requests, before)
		}
	})
	t.Run("phase persistence failure", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		if err := os.Mkdir(home, 0700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { forgetRailwayHome(home) })
		runs := 0
		rw := syncFixture(t, func(cmd string) execOut {
			if strings.Contains(cmd, "env -i") {
				runs++
			}
			if strings.Contains(cmd, "base64 -d /tmp/h.b64") {
				if err := os.Remove(pendingFile(home)); err != nil {
					t.Error(err)
				}
				if err := os.Mkdir(pendingFile(home), 0700); err != nil {
					t.Error(err)
				}
			}
			return syncSuccess("")
		})
		if _, err := rw.Exec(context.Background(), Spec{Home: home, Cmd: "owner-command", Limits: DefaultLimits()}); !errors.Is(err, ErrRecoveryPending) || runs != 0 {
			t.Fatal("command dispatched before durable phase update", err, runs)
		}
	})
}

func TestRailwayRecoveryFailedEvictionRetainsIdentity(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idFile(home), []byte("keep-id"), 0600); err != nil {
		t.Fatal(err)
	}
	reg.Lock()
	oldIDs, oldUsed, oldSynced := reg.ids, reg.used, reg.synced
	reg.ids = map[string]string{home: "keep-id"}
	reg.used = map[string]time.Time{home: time.Now()}
	reg.synced = map[string]string{}
	reg.Unlock()
	defer func() { reg.Lock(); reg.ids, reg.used, reg.synced = oldIDs, oldUsed, oldSynced; reg.Unlock() }()
	creates := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var in struct{ Query string }
		json.NewDecoder(req.Body).Decode(&in)
		if strings.Contains(in.Query, "sandboxCreate(") {
			creates++
		}
		http.Error(w, "fixture cleanup unknown", 502)
	}))
	defer srv.Close()
	rw := &Railway{Endpoint: srv.URL, Max: 1}
	if _, err := rw.sandbox(context.Background(), filepath.Join(t.TempDir(), "new"), false); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	reg.Lock()
	id := reg.ids[home]
	reg.Unlock()
	if id != "keep-id" || creates != 0 {
		t.Fatal("unconfirmed eviction lost identity or created another VM", id, creates)
	}
	if raw, err := os.ReadFile(idFile(home)); err != nil || string(raw) != "keep-id" {
		t.Fatal("unconfirmed eviction removed persisted identity", err)
	}
	gate := railwayHomeGate(home)
	if !gate.TryLock() {
		t.Fatal("failed eviction leaked home lock")
	}
	gate.Unlock()
}
