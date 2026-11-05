package computer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerRunBoundary(t *testing.T) {
	d := NewDocker()
	s := Spec{Home: t.TempDir(), Cmd: "printf test", Limits: DefaultLimits()}
	args, err := d.runArgs(s, "aurora-run-fixture")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--pull=never", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--network\nnone", "--user=1000:1000", "--entrypoint=/usr/bin/env", s.Home + ",target=/home/bot", "-i", "set -e\n"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing isolation: %s", want)
		}
	}
	if strings.Contains(joined, "docker.sock") || strings.Contains(joined, "--privileged") || strings.Contains(joined, "--publish") {
		t.Fatal("host access exposed")
	}
	t.Setenv("AURORA_TEST_HOST_SECRET", "canary-private")
	t.Setenv("DOCKER_HOST", "tcp://untrusted.example:2375")
	cmd := d.command(context.Background(), "info")
	if strings.Contains(strings.Join(cmd.Env, "\n"), "canary-private") || strings.Contains(strings.Join(cmd.Args, "\n"), "untrusted.example") {
		t.Fatal("host configuration or secrets leaked")
	}
	s.Home = filepath.Join(s.Home, "comma,home")
	if _, err = d.runArgs(s, "fixture"); err == nil {
		t.Fatal("ambiguous bind mount accepted")
	}
	d.Image = "bad --privileged"
	if _, err = d.runArgs(Spec{Home: t.TempDir(), Limits: DefaultLimits()}, "fixture"); err == nil {
		t.Fatal("invalid image accepted")
	}
}

// Opt-in: executes the actual local Docker engine and never pulls an image.
func TestDockerRealWorkspace(t *testing.T) {
	if os.Getenv("AURORA_TEST_DOCKER") != "1" {
		t.Skip("set AURORA_TEST_DOCKER=1 for local sandbox integration")
	}
	d := NewDocker()
	if err := d.Available(); err != nil {
		t.Fatal(err)
	}
	m, err := New(t.TempDir(), d)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.WriteFile("one", "index.html", "<!doctype html><h1>Neko Aurora</h1>"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURORA_TEST_HOST_SECRET", "canary-private")
	r, err := m.Run(context.Background(), "one", `node -e 'const fs=require("fs");if(process.env.AURORA_TEST_HOST_SECRET)process.exit(1);if(!fs.readFileSync("index.html","utf8").includes("Neko Aurora"))process.exit(2);fs.writeFileSync("result.txt","42");console.log("NODE_OK")' && python3 -c 'print("PYTHON_OK")' && test ! -e /workspace && test ! -S /var/run/docker.sock && test ! -e /home/Admin && test "$(id -u)" = 1000 && ! touch /usr/aurora-probe`)
	if err != nil || r.ExitCode != 0 || !strings.Contains(r.Output, "NODE_OK") || !strings.Contains(r.Output, "PYTHON_OK") {
		t.Fatalf("execution: %+v %v", r, err)
	}
	text, _, err := m.ReadFile("one", "result.txt")
	if err != nil || text != "42" {
		t.Fatalf("persistence: %q %v", text, err)
	}
	if _, _, err = m.ReadFile("two", "result.txt"); err == nil {
		t.Fatal("another bot can see the file")
	}
	r, err = m.Run(context.Background(), "one", "false; echo should-not-run")
	if err != nil || r.ExitCode == 0 || strings.Contains(r.Output, "should-not-run") {
		t.Fatalf("failure masked: %+v %v", r, err)
	}
	r, err = m.Run(context.Background(), "one", `python3 -c 'import socket; s=socket.socket(); s.settimeout(1); assert s.connect_ex(("1.1.1.1",443)) != 0; print("NETWORK_BLOCKED")'`)
	if err != nil || r.ExitCode != 0 || !strings.Contains(r.Output, "NETWORK_BLOCKED") {
		t.Fatalf("network not blocked: %+v %v", r, err)
	}
	m.Limits.Timeout = 2 * time.Second
	r, err = m.Run(context.Background(), "one", "sleep 30")
	if err != nil || !r.TimedOut {
		t.Fatalf("timeout: %+v %v", r, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := d.command(ctx, "ps", "-q", "--filter", "label=neko.aurora.computer=ephemeral").Output()
	if err != nil || len(strings.TrimSpace(string(out))) > 0 {
		t.Fatalf("container survived timeout: %v", err)
	}
}

func TestDockerUnavailableIsBounded(t *testing.T) {
	d := NewDocker()
	d.Path = filepath.Join(t.TempDir(), "missing-docker")
	started := time.Now()
	err := d.Available()
	if err == nil || time.Since(started) > 5*time.Second {
		t.Fatalf("unavailable probe: %v", err)
	}
}
