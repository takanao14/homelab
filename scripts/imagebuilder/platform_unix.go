//go:build linux || darwin

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func openNoFollow(path string, flags int, mode os.FileMode) (*os.File, error) {
	fd, err := syscall.Open(path, flags|syscall.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
func processGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killGroup(pid int)          { _ = syscall.Kill(-pid, syscall.SIGKILL) }
func processExitCode(cmd *exec.Cmd) int {
	if st, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && st.Signaled() {
		return -int(st.Signal())
	}
	return -1
}
