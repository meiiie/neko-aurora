//go:build !windows

package documentworker

import "os/exec"

func hideWindow(command *exec.Cmd) {}
