//go:build linux || darwin

package command

import (
	"os/exec"
	"syscall"
)

func processGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func killGroup(pid int) { _ = syscall.Kill(-pid, syscall.SIGKILL) }

func processExitCode(cmd *exec.Cmd) int {
	if st, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && st.Signaled() {
		return -int(st.Signal())
	}
	return -1
}
