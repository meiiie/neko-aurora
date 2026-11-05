//go:build windows

package aurora

import (
	"os/exec"
	"syscall"
)

func hideCoreWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
