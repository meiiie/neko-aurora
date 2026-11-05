//go:build !windows

package computer

import "os/exec"

func hideComputerCommand(*exec.Cmd) {}
