//go:build windows

package qa

import (
	"os/exec"
	"strconv"
)

// setProcessGroup is a no-op on Windows; killProcessGroup uses taskkill /T.
func setProcessGroup(*exec.Cmd) {}

// killProcessGroup kills the command and its child processes.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		_ = cmd.Process.Kill()
	}
}
