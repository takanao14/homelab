package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CommandEvent struct {
	Event      string
	Line       []byte
	ExitCode   *int
	DurationMS int64
	Category   string
}
type CommandOptions struct {
	Dir             string
	Env             map[string]string
	Timeout         time.Duration
	Capture, Digest bool
	Observer        func(CommandEvent) error
}
type CommandResult struct {
	Output   []byte
	Digest   string
	ExitCode int
}
type CommandFunc = func([]string, CommandOptions) (CommandResult, error)

func commandEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	r := make([]string, 0, len(keys))
	for _, k := range keys {
		r = append(r, k+"="+env[k])
	}
	return r
}
func commandPath(name string, env map[string]string) string {
	if strings.Contains(name, "/") {
		return name
	}
	for _, dir := range filepath.SplitList(env["PATH"]) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		st, e := os.Stat(p)
		if e == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0 {
			return p
		}
	}
	return ""
}
func RunCommand(args []string, opt CommandOptions) (CommandResult, error) {
	result := CommandResult{ExitCode: -1}
	if len(args) == 0 {
		return result, fail("invalid_request")
	}
	started := time.Now()
	observe := func(event CommandEvent) error {
		if opt.Observer != nil {
			return opt.Observer(event)
		}
		return nil
	}
	if e := observe(CommandEvent{Event: "started"}); e != nil {
		return result, fail("invalid_command_output")
	}
	env := opt.Env
	if env == nil {
		env = baseEnv()
	}
	path := commandPath(args[0], env)
	if path == "" {
		_ = observe(CommandEvent{Event: "failed", DurationMS: time.Since(started).Milliseconds(), Category: "command-exit"})
		return result, fail("command_failed")
	}
	cmd := exec.Command(path, args[1:]...)
	cmd.Dir = opt.Dir
	cmd.Env = commandEnv(env)
	processGroup(cmd)
	stream, e := cmd.StdoutPipe()
	if e != nil {
		return result, fail("command_failed")
	}
	cmd.Stderr = io.Discard
	if opt.Observer != nil && !opt.Capture && !opt.Digest {
		cmd.Stderr = cmd.Stdout
	}
	if cmd.Start() != nil {
		_ = observe(CommandEvent{Event: "failed", DurationMS: time.Since(started).Milliseconds(), Category: "command-exit"})
		return result, fail("command_failed")
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	type drained struct {
		data   []byte
		digest string
		bad    bool
	}
	readDone := make(chan drained, 1)
	go func() {
		var output bytes.Buffer
		hash := sha256.New()
		pending := make([]byte, 0, 16384)
		dropping := false
		bad := false
		buf := make([]byte, 65536)
		for {
			count, err := stream.Read(buf)
			if count > 0 {
				chunk := buf[:count]
				if opt.Digest {
					_, _ = hash.Write(chunk)
				} else if opt.Capture {
					if output.Len()+count <= 2097152 {
						_, _ = output.Write(chunk)
					} else {
						bad = true
					}
				}
				if opt.Observer != nil && !opt.Digest && !bad {
					for _, v := range chunk {
						if v == '\n' {
							if !dropping && observe(CommandEvent{Event: "output", Line: append([]byte(nil), pending...)}) != nil {
								bad = true
								killGroup(cmd.Process.Pid)
							}
							pending = pending[:0]
							dropping = false
						} else if !dropping {
							if len(pending) < 16384 {
								pending = append(pending, v)
							} else {
								pending = pending[:0]
								dropping = true
							}
						}
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					bad = true
				}
				break
			}
		}
		if opt.Observer != nil && !opt.Digest && !bad && !dropping && len(pending) > 0 {
			if observe(CommandEvent{Event: "output", Line: pending}) != nil {
				bad = true
			}
		}
		readDone <- drained{data: output.Bytes(), digest: hex.EncodeToString(hash.Sum(nil)), bad: bad}
	}()
	var read drained
	timedOut := false
	select {
	case read = <-readDone:
	case <-timer.C:
		timedOut = true
		killGroup(cmd.Process.Pid)
		_ = stream.Close()
		read = <-readDone
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case e = <-wait:
	case <-timer.C:
		timedOut = true
		killGroup(cmd.Process.Pid)
		e = <-wait
	}
	result.ExitCode = cmd.ProcessState.ExitCode()
	if result.ExitCode < 0 {
		result.ExitCode = processExitCode(cmd)
	}
	event := CommandEvent{Event: "completed", ExitCode: &result.ExitCode, DurationMS: time.Since(started).Milliseconds()}
	if timedOut || e != nil {
		event.Event = "failed"
		event.Category = "command-exit"
		if timedOut {
			event.Category = "timeout"
		}
	}
	observerErr := observe(event)
	if timedOut {
		return result, fail("command_timeout")
	}
	if observerErr != nil || read.bad {
		return result, fail("invalid_command_output")
	}
	if e != nil {
		return result, fail("command_failed")
	}
	if opt.Capture {
		result.Output = read.data
	}
	if opt.Digest {
		result.Digest = read.digest
	}
	return result, nil
}
