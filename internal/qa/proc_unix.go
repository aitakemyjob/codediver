//go:build !windows

package qa

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts the command in its own process group so
// cancelling also stops the test processes it spawns.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the command and everything it started.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
