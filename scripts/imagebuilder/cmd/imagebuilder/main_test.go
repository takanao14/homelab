package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

const testJobID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func logged(inv invocation, err error) []map[string]any {
	var output bytes.Buffer
	inv.log(slog.New(slog.NewJSONHandler(&output, nil)), 1500*time.Millisecond, err)
	records := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if line == "" {
			continue
		}
		record := map[string]any{}
		if json.Unmarshal([]byte(line), &record) != nil {
			return nil
		}
		records = append(records, record)
	}
	return records
}

func TestInvocationLogSkipsPollingSuccess(t *testing.T) {
	for _, inv := range []invocation{{}, {command: "idle"}, {command: "control", op: "status"}, {command: "control", op: "logs", jobID: testJobID}} {
		if records := logged(inv, nil); len(records) != 0 {
			t.Fatal("polling success was logged")
		}
	}
	if records := logged(invocation{command: "idle"}, protocol.Fail("invalid_state")); len(records) != 1 || records[0]["code"] != "invalid_state" {
		t.Fatal("idle failure was not logged")
	}
}

func TestInvocationLogEmitsOnlyAllowlistedFields(t *testing.T) {
	records := logged(invocation{command: "control", op: "submit", jobID: testJobID}, nil)
	if len(records) != 1 || records[0]["msg"] != "completed" || records[0]["op"] != "submit" || records[0]["job_id"] != testJobID || records[0]["duration_ms"] != float64(1500) {
		t.Fatal("state-changing operation was not logged")
	}
	records = logged(invocation{command: "run", jobID: testJobID}, errors.New("SECRET_MARKER"))
	raw, _ := json.Marshal(records)
	if len(records) != 1 || records[0]["code"] != "internal_error" || records[0]["level"] != "ERROR" || strings.Contains(string(raw), "SECRET_MARKER") {
		t.Fatal("failure record exposed an unapproved value")
	}
}

func TestDispatchRecordsOnlyValidatedIdentity(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		command, idOK string
	}{
		{[]string{"run", testJobID}, "run", testJobID},
		{[]string{"run", "SECRET_MARKER"}, "run", ""},
		{[]string{"SECRET_MARKER"}, "", ""},
	} {
		var inv invocation
		_ = dispatch(tc.args, strings.NewReader(""), io.Discard, &inv)
		if inv.command != tc.command || inv.jobID != tc.idOK || inv.op != "" {
			t.Fatal("dispatch recorded unvalidated input")
		}
	}
}
