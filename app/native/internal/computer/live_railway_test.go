package computer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// RAILWAY_LIVE=1 runs against the real Railway Sandboxes API (costs cents).
func TestRailwayLive(t *testing.T) {
	if os.Getenv("RAILWAY_LIVE") != "1" {
		t.Skip("set RAILWAY_LIVE=1")
	}
	r := NewRailwayFromEnv()
	home := filepath.Join(t.TempDir(), "home")
	os.MkdirAll(home, 0o700)
	os.WriteFile(filepath.Join(home, "d.csv"), []byte("a,b\n1,2\n3,4\n"), 0o600)
	ctx := context.Background()
	defer r.Destroy(ctx, home)
	sp := Spec{Home: home, Cmd: `python3 -c "import csv;r=list(csv.DictReader(open('d.csv')));print(sum(int(x['b']) for x in r))" > sum.txt; cat sum.txt; uname -s`, Limits: DefaultLimits()}
	t0 := time.Now()
	res, err := r.Exec(ctx, sp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("exit=%d ms=%d first=%v out=%q", res.ExitCode, res.Ms, time.Since(t0), res.Output)
	b, _ := os.ReadFile(filepath.Join(home, "sum.txt"))
	t.Logf("pulled sum.txt=%q", b)
	res, err = r.Exec(ctx, Spec{Home: home, Cmd: "cat sum.txt", Limits: DefaultLimits()})
	t.Logf("second: %v %q ms=%d", err, res.Output, res.Ms)
}

// RAILWAY_SETUP=<sandbox id>: install python3 into that sandbox and save it
// as the "aurora-base" checkpoint new bot VMs boot from.
func TestRailwayCheckpointSetup(t *testing.T) {
	id := os.Getenv("RAILWAY_SETUP")
	if id == "" {
		t.Skip("set RAILWAY_SETUP")
	}
	r := NewRailwayFromEnv()
	ctx := context.Background()
	o, err := r.exec(ctx, id, "(sudo -n true 2>/dev/null && S=sudo); $S apt-get update -qq >/dev/null && DEBIAN_FRONTEND=noninteractive $S apt-get install -y -qq --no-install-recommends python3 python3-pip >/dev/null; python3 --version; $S apt-get clean", 600)
	t.Logf("%v %+v", err, o)
	var out map[string]any
	err = r.gql(ctx, `mutation C($environmentId: String!, $name: String!, $sandboxId: String!) { sandboxCheckpointCreate(environmentId: $environmentId, name: $name, sandboxId: $sandboxId) { id key } }`,
		map[string]any{"environmentId": r.EnvironmentID, "name": "aurora-base", "sandboxId": id}, &out)
	t.Logf("checkpoint %v %v", err, out)
	r.destroyID(ctx, id)
}

// RAILWAY_IDLE=1: one command, then no destroy; the VM must go on its own.
func TestRailwayIdleStop(t *testing.T) {
	if os.Getenv("RAILWAY_IDLE") != "1" {
		t.Skip("set RAILWAY_IDLE=1")
	}
	r := NewRailwayFromEnv()
	home := t.TempDir()
	ctx := context.Background()
	if _, err := r.Exec(ctx, Spec{Home: home, Cmd: "true", Limits: DefaultLimits()}); err != nil {
		t.Fatal(err)
	}
	reg.Lock()
	id := reg.ids[home]
	reg.Unlock()
	t0 := time.Now()
	for time.Since(t0) < 9*time.Minute {
		var g struct {
			Sandbox struct{ Status string } `json:"sandbox"`
		}
		err := r.gql(ctx, qGet, map[string]any{"environmentId": r.EnvironmentID, "id": id}, &g)
		if err != nil || g.Sandbox.Status != "RUNNING" {
			t.Logf("gone after %v: %v %s", time.Since(t0).Round(time.Second), err, g.Sandbox.Status)
			return
		}
		time.Sleep(15 * time.Second)
	}
	r.destroyID(ctx, id)
	t.Fatal("still running after 9 min")
}

// RAILWAY_GONE=<destroyed id>: what does exec on a destroyed VM return?
func TestRailwayGoneError(t *testing.T) {
	id := os.Getenv("RAILWAY_GONE")
	if id == "" {
		t.Skip("set RAILWAY_GONE")
	}
	_, err := NewRailwayFromEnv().exec(context.Background(), id, "true", 10)
	t.Logf("err=%v gone=%v", err, err == errSandboxGone)
}

// RAILWAY_BAKE=<checkpoint name>: boot from the current base, preinstall the
// document/video stack (python-pptx, Pillow, imageio + its bundled ffmpeg)
// and save it as a new checkpoint, so bots stop pip-installing per task.
func TestRailwayBakeCheckpoint(t *testing.T) {
	name := os.Getenv("RAILWAY_BAKE")
	if name == "" {
		t.Skip("set RAILWAY_BAKE")
	}
	r := NewRailwayFromEnv()
	r.CPU, r.MemoryGB = 1, 1
	ctx := context.Background()
	id, err := r.create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r.destroyID(ctx, id)
	o, err := r.exec(ctx, id, "pip install -q --break-system-packages python-pptx Pillow imageio imageio-ffmpeg 2>&1 | tail -3 || pip install -q python-pptx Pillow imageio imageio-ffmpeg 2>&1 | tail -3; python3 -c 'import pptx,PIL,imageio,imageio_ffmpeg;print(pptx.__version__,PIL.__version__,imageio.__version__,imageio_ffmpeg.get_ffmpeg_exe())'; pip cache purge >/dev/null 2>&1; df -h / | tail -1", 600)
	t.Logf("%v exit=%v out=%s err=%s", err, o.ExitCode, o.Stdout, o.Stderr)
	if err != nil || o.ExitCode == nil || *o.ExitCode != 0 || !strings.Contains(o.Stdout, "ffmpeg") {
		t.Fatal("install failed")
	}
	var out map[string]any
	err = r.gql(ctx, `mutation C($environmentId: String!, $name: String!, $sandboxId: String!) { sandboxCheckpointCreate(environmentId: $environmentId, name: $name, sandboxId: $sandboxId) { id key } }`,
		map[string]any{"environmentId": r.EnvironmentID, "name": name, "sandboxId": id}, &out)
	t.Logf("checkpoint %v %v", err, out)
	if err != nil {
		t.Fatal(err)
	}
}
