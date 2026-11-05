package computer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SetupError struct{ Code string }

func (e *SetupError) Error() string { return "computer_unavailable: " + e.Code }
func (e *SetupError) Unwrap() error { return ErrUnavailable }

type Docker struct {
	Path, Host, Image string
	mu                sync.Mutex
	checked           time.Time
	checkErr          error
}

func NewDocker() *Docker {
	d := &Docker{Path: "docker", Host: "unix:///var/run/docker.sock", Image: os.Getenv("AURORA_COMPUTER_IMAGE")}
	if d.Image == "" {
		d.Image = "neko-aurora-computer:local"
	}
	if runtime.GOOS == "windows" {
		d.Host = "npipe:////./pipe/dockerDesktopLinuxEngine"
		candidate := filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker", "resources", "bin", "docker.exe")
		if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
			d.Path = candidate
		}
	}
	if !filepath.IsAbs(d.Path) {
		if p, err := exec.LookPath(d.Path); err == nil {
			d.Path = p
		}
	}
	return d
}

func (*Docker) Name() string { return "docker" }

var dockerImagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,250}$`)

func (d *Docker) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, d.Path, append([]string{"--host", d.Host}, args...)...)
	for _, key := range []string{"SYSTEMROOT", "WINDIR", "COMSPEC", "PATH", "PATHEXT", "TEMP", "TMP"} {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	hideComputerCommand(cmd)
	return cmd
}

func (d *Docker) Available() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	ttl := 30 * time.Second
	if d.checkErr != nil {
		ttl = 3 * time.Second
	}
	if time.Since(d.checked) < ttl {
		return d.checkErr
	}
	err := d.probe()
	d.checked, d.checkErr = time.Now(), err
	return err
}

func (d *Docker) probe() error {
	if !dockerImagePattern.MatchString(d.Image) {
		return &SetupError{Code: "docker_image_invalid"}
	}
	if !filepath.IsAbs(d.Path) {
		return &SetupError{Code: "docker_missing"}
	}
	// Windows can take several seconds to start each Docker CLI process. Both
	// daemon and image probes must fit; a four-second shared budget reported a
	// healthy engine offline mid-turn during the real ChatGPT landing-page run.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := d.command(ctx, "info", "--format", "{{.OSType}}").Output()
	if err != nil {
		return &SetupError{Code: "docker_offline"}
	}
	if strings.TrimSpace(string(output)) != "linux" {
		return &SetupError{Code: "docker_linux_required"}
	}
	if err := d.command(ctx, "image", "inspect", "--format", "{{.Id}}", d.Image).Run(); err != nil {
		return &SetupError{Code: "docker_image_missing"}
	}
	return nil
}

func (d *Docker) runArgs(s Spec, name string) ([]string, error) {
	if !dockerImagePattern.MatchString(d.Image) || !filepath.IsAbs(s.Home) || strings.ContainsAny(s.Home, ",\r\n") {
		return nil, ErrPath
	}
	real, err := filepath.EvalSymlinks(s.Home)
	same := filepath.Clean(real) == filepath.Clean(s.Home)
	if runtime.GOOS == "windows" {
		same = strings.EqualFold(filepath.Clean(real), filepath.Clean(s.Home))
	}
	if err != nil || !same {
		return nil, ErrPath
	}
	l := s.Limits
	if l.MemoryBytes <= 0 || l.FileBytes <= 0 || l.OutputBytes <= 0 || l.Timeout <= 0 || l.Procs <= 0 || l.CPUSeconds <= 0 {
		return nil, ErrPath
	}
	network := "none"
	if s.Network {
		network = "bridge"
	}
	a := []string{"run", "--rm", "--pull=never", "--name", name, "--label", "neko.aurora.computer=ephemeral",
		"--init", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--network", network,
		"--user=1000:1000", "--cpus=1", "--memory=" + strconv.FormatInt(l.MemoryBytes, 10), "--memory-swap=" + strconv.FormatInt(l.MemoryBytes, 10),
		"--pids-limit=" + strconv.Itoa(l.Procs), "--ulimit", fmt.Sprintf("fsize=%d:%d", l.FileBytes, l.FileBytes), "--ulimit", fmt.Sprintf("cpu=%d:%d", l.CPUSeconds, l.CPUSeconds),
		"--mount", "type=bind,source=" + s.Home + ",target=/home/bot", "--tmpfs", "/tmp:rw,nosuid,size=268435456",
		"--workdir=/home/bot", "--entrypoint=/usr/bin/env", d.Image, "-i"}
	a = append(a, sandboxEnv...)
	return append(a, "/bin/bash", "-c", strictShell+s.Cmd), nil
}

func (d *Docker) Exec(ctx context.Context, s Spec) (Result, error) {
	if err := d.Available(); err != nil {
		return Result{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Result{}, err
	}
	name := "aurora-run-" + hex.EncodeToString(random[:])
	args, err := d.runArgs(s, name)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.Limits.Timeout)
	defer cancel()
	started := time.Now()
	output := &capWriter{max: s.Limits.OutputBytes}
	cmd := d.command(ctx, args...)
	cmd.Stdout, cmd.Stderr = output, output
	err = cmd.Run()
	// Killing the CLI alone leaves a container running. Remove only this run's random name.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	cleanupErr := d.command(cleanupCtx, "rm", "--force", name).Run()
	cleanupCancel()
	r := Result{Cmd: s.Cmd, Output: output.buf.String(), Truncated: output.trunc, Ms: int(time.Since(started) / time.Millisecond), At: time.Now().UTC().Format(time.RFC3339)}
	if ctx.Err() != nil {
		r.TimedOut, r.ExitCode = true, 124
		if cleanupErr != nil {
			return r, fmt.Errorf("%w: container_stop_unconfirmed", ErrUnavailable)
		}
		return r, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		r.ExitCode = exit.ExitCode()
	} else if err != nil {
		return r, fmt.Errorf("%w: docker_launch_failed", ErrUnavailable)
	}
	return r, nil
}
