//go:build !windows

package aurora

import "os/exec"

func hideCoreWindow(cmd *exec.Cmd) {}
