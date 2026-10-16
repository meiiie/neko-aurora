//go:build !windows

package connections

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
