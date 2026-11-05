package computer

import (
	"os/exec"
	"syscall"
)

func hideComputerCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
