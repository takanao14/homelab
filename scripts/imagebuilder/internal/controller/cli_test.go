package controller

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func TestCLIUsesKongFlagSemantics(t *testing.T) {
	parsed, err := parseArgs([]string{"run", "ubuntu26-base", "--branch=first", "--branch=last", "--publish=true", "--publish=false", "--detach=true"})
	if err != nil || parsed.branch != "last" || parsed.publish || !parsed.detach {
		t.Fatal("scalar or boolean flag semantics changed")
	}
	parsed, err = parseArgs([]string{"logs", testJob, "--after=0x10"})
	if err != nil || parsed.after != 16 {
		t.Fatal("integer cursor was not parsed")
	}
	for _, args := range [][]string{
		{"run", "ubuntu26-base", "--revision="},
		{"run", "ubuntu26-base", "--branch="},
		{"run", "ubuntu26-base", "--branch=main", "--revision=" + testRevision},
		{"logs", testJob, "--after=-1"},
		{"logs", ""},
	} {
		if _, err := parseArgs(args); err == nil {
			t.Fatal("invalid operation was accepted")
		}
	}
}

func TestCLICommandSelection(t *testing.T) {
	for _, args := range [][]string{
		{"status"}, {"status", testJob}, {"shutdown"}, {"start"},
		{"maintenance", "on"}, {"logs", testJob}, {"publish-retry", testJob},
	} {
		parsed, err := parseArgs(args)
		if err != nil || parsed.command != args[0] {
			t.Fatal("command selection failed")
		}
	}
}

func TestCLIHelpDoesNotInitializeController(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"run", "--help"}, {"logs", "--help"}} {
		var output bytes.Buffer
		if err := Main(args, nil, &output); err != nil || !strings.Contains(output.String(), "Usage:") {
			t.Fatal("help did not return without credentials")
		}
	}
}

func TestCLIRejectsInputsWithoutEchoingValues(t *testing.T) {
	for _, args := range [][]string{
		{"--help", "--unknown=" + testSecret},
		{"run", "ubuntu26-base", "--publish=" + testSecret, "--branch=main"},
		{"logs", testJob, "--after=" + testSecret},
	} {
		var output bytes.Buffer
		err := Main(args, nil, &output)
		if err == nil || protocol.SafeCode(err) != "invalid_arguments" || output.Len() != 0 || strings.Contains(err.Error(), testSecret) {
			t.Fatal("parser diagnostics escaped or initialization was attempted")
		}
	}
	if _, _, err := parseCLI([]string{"--help"}, failingCLIWriter{}); protocol.SafeCode(err) != "controller_failed" {
		t.Fatal("help write failure was ignored")
	}
}

type failingCLIWriter struct{}

func (failingCLIWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
