package computer

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func (b *Bwrap) Available() error {
	if _, err := exec.LookPath(b.Path); err != nil {
		return fmt.Errorf("%w: bwrap not installed", ErrUnavailable)
	}
	if _, err := exec.LookPath(b.Prlimit); err != nil {
		return fmt.Errorf("%w: prlimit not installed", ErrUnavailable)
	}
	return b.probe()
}

func (b *Bwrap) Exec(ctx context.Context, s Spec) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Limits.Timeout)
	defer cancel()
	t0 := time.Now()
	cmd := exec.Command(b.Prlimit, b.args(s)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"} // prlimit/bwrap themselves get no host env either
	out := &capWriter{max: s.Limits.OutputBytes}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	timedOut := false
	select {
	case err = <-done:
	case <-ctx.Done():
		timedOut = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // bwrap is pid 1 of the sandbox: everything dies
		err = <-done
	}
	r := Result{Cmd: s.Cmd, Output: out.buf.String(), Truncated: out.trunc, TimedOut: timedOut, Ms: int(time.Since(t0) / time.Millisecond), At: time.Now().UTC().Format(time.RFC3339)}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		r.ExitCode = ee.ExitCode()
		if r.ExitCode < 0 {
			r.ExitCode = 137
		}
	default:
		return r, err
	}
	if timedOut {
		r.ExitCode = 124
		r.Output += fmt.Sprintf("\n[hết thời gian %s: lệnh bị dừng]", s.Limits.Timeout)
	}
	if r.Truncated {
		r.Output += "\n[đã cắt bớt đầu ra]"
	}
	return r, nil
}
