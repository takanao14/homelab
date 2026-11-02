package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCommandCaptureAndDigestExcludeStderr(t *testing.T) {
	args := []string{"/bin/sh", "-c", "printf public; printf SECRET_MARKER >&2"}
	for _, digest := range []bool{false, true} {
		result, e := RunCommand(args, CommandOptions{Capture: !digest, Digest: digest, Observer: func(CommandEvent) error { return nil }})
		if e != nil {
			t.Fatal(safeCode(e))
		}
		if digest {
			hash := sha256.Sum256([]byte("public"))
			if result.Digest != hex.EncodeToString(hash[:]) {
				t.Fatal("stderr affected digest")
			}
		} else if string(result.Output) != "public" {
			t.Fatal("stderr affected captured output")
		}
	}
}
func TestCommandTimeoutKillsProcessGroup(t *testing.T) {
	start := time.Now()
	_, e := RunCommand([]string{"/bin/sh", "-c", "sleep 10"}, CommandOptions{Timeout: 50 * time.Millisecond})
	if safeCode(e) != "command_timeout" {
		t.Fatal("timeout not reported safely")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("process group remained alive")
	}
}
func TestCommandShortLineArrivesBeforeExit(t *testing.T) {
	ack := filepath.Join(t.TempDir(), "ack")
	seen := false
	_, e := RunCommand([]string{"/bin/sh", "-c", `printf 'IMAGEBUILDER_STEP=packer-init\n'; while [ ! -e "$1" ]; do sleep 0.01; done`, "sh", ack}, CommandOptions{Timeout: 3 * time.Second, Observer: func(ev CommandEvent) error {
		if ev.Event == "output" && string(ev.Line) == "IMAGEBUILDER_STEP=packer-init" {
			seen = true
			return os.WriteFile(ack, nil, 0600)
		}
		return nil
	}})
	if e != nil || !seen {
		t.Fatal("short progress line was buffered until process exit")
	}
}
func TestCommandFailureNeverReturnsChildErrorText(t *testing.T) {
	_, e := RunCommand([]string{"/bin/sh", "-c", "printf SECRET_MARKER; printf SECRET_MARKER >&2; exit 3"}, CommandOptions{})
	if safeCode(e) != "command_failed" || strings.Contains(e.Error(), "SECRET_MARKER") {
		t.Fatal("unsafe child error")
	}
}
func TestCommandCaptureIsBounded(t *testing.T) {
	_, e := RunCommand([]string{"/bin/sh", "-c", "dd if=/dev/zero bs=65536 count=33 2>/dev/null"}, CommandOptions{Capture: true})
	if safeCode(e) != "invalid_command_output" {
		t.Fatal("unbounded machine output")
	}
}

func TestCommandTimeoutClosesPipeHeldByDetachedChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	defer func() {
		data, e := os.ReadFile(pidFile)
		if e == nil {
			pid, e := strconv.Atoi(string(data))
			if e == nil {
				killGroup(pid)
			}
		}
	}()
	env := baseEnv()
	env["IMAGEBUILDER_COMMAND_HELPER"] = "leader"
	env["IMAGEBUILDER_HELPER_PID"] = pidFile
	env["GORACE"] = "atexit_sleep_ms=0"
	start := time.Now()
	_, e := RunCommand([]string{os.Args[0], "-test.run=^TestDetachedCommandHelper$"}, CommandOptions{Env: env, Timeout: 500 * time.Millisecond})
	if safeCode(e) != "command_timeout" || time.Since(start) > 3*time.Second {
		t.Fatal("detached stdout holder bypassed timeout")
	}
}
func TestDetachedCommandHelper(t *testing.T) {
	switch os.Getenv("IMAGEBUILDER_COMMAND_HELPER") {
	case "leader":
		child := exec.Command(os.Args[0], "-test.run=^TestDetachedCommandHelper$")
		child.Env = commandEnv(map[string]string{"PATH": baseEnv()["PATH"], "IMAGEBUILDER_COMMAND_HELPER": "child", "GORACE": "atexit_sleep_ms=0"})
		child.Stdout = os.Stdout
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if child.Start() != nil {
			os.Exit(2)
		}
		if os.WriteFile(os.Getenv("IMAGEBUILDER_HELPER_PID"), []byte(strconv.Itoa(child.Process.Pid)), 0600) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "child":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
}
func TestCommandDoesNotFallBackToParentPATH(t *testing.T) {
	_, e := RunCommand([]string{"sh", "-c", "exit 0"}, CommandOptions{Env: map[string]string{"PATH": t.TempDir()}})
	if safeCode(e) != "command_failed" {
		t.Fatal("parent PATH was used")
	}
}
