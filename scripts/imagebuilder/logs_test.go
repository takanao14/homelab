package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func logRuntime(t *testing.T) (*Runtime, string) {
	t.Helper()
	root := t.TempDir()
	r := &Runtime{Cfg: Config{WorkDir: root, RuntimeDir: filepath.Join(root, "run")}}
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if os.MkdirAll(filepath.Join(r.JobDir(id), "repo", "packer"), 0700) != nil {
		t.Fatal("fixture failed")
	}
	if os.WriteFile(filepath.Join(r.JobDir(id), "repo", "packer", "image.pkr.hcl"), []byte("packer {}\n"), 0600) != nil {
		t.Fatal("fixture failed")
	}
	return r, id
}
func TestLogsExtractOnlyKnownHCLAndProvisionerFacts(t *testing.T) {
	r, id := logRuntime(t)
	ctx := &logContext{Step: "packer-build"}
	for _, line := range []string{`1,,ui,error,Error: Unsupported argument\n\n  on image.pkr.hcl line 12, column 3:\nSECRET_MARKER`, `1,qemu.custom,ui,error,Script exited with non-zero exit status: 100. SECRET_MARKER`, `1,,ui,message,SECRET_MARKER`} {
		if r.diagnosticOutput(id, ctx, []byte(line)) != nil {
			t.Fatal("diagnostics failed")
		}
	}
	page, e := r.DiagnosticPage(id, 0)
	if e != nil {
		t.Fatal(safeCode(e))
	}
	raw, _ := json.Marshal(page)
	if containsBytes(raw, []byte("SECRET_MARKER")) {
		t.Fatal("raw message persisted")
	}
	records := page["records"].([]any)
	if len(records) != 2 {
		t.Fatal("unknown messages must be discarded")
	}
	first := records[0].(Object)
	if s(first, "category") != "hcl-unsupported-argument" || s(first, "source") != "packer/image.pkr.hcl" || n(first, "line") != 12 || n(first, "column") != 3 {
		t.Fatal("HCL location lost")
	}
	if n(records[1].(Object), "provisioner_exit_code") != 100 {
		t.Fatal("provisioner exit code lost")
	}
}
func TestLogsValidateBeforePersistingAndDisplaying(t *testing.T) {
	r, id := logRuntime(t)
	if r.appendDiagnostic(id, "build", "started", Object{"raw": "SECRET_MARKER"}) != nil {
		t.Fatal("safe projection failed")
	}
	page, e := r.DiagnosticPage(id, 0)
	if e != nil {
		t.Fatal(safeCode(e))
	}
	raw, _ := json.Marshal(page)
	if containsBytes(raw, []byte("SECRET_MARKER")) {
		t.Fatal("extra field leaked")
	}
	if _, e = validateLogRecord(Object{"sequence": 1, "time": 1, "step": []any{"build"}, "event": "started"}); e == nil {
		t.Fatal("malformed enum accepted")
	}
}
func TestLogsPaginateAndRetainBoundedHistory(t *testing.T) {
	r, id := logRuntime(t)
	for i := 0; i < 1003; i++ {
		if r.appendDiagnostic(id, "build", "started", nil) != nil {
			t.Fatal("append failed")
		}
	}
	page, e := r.DiagnosticPage(id, 0)
	if e != nil {
		t.Fatal(safeCode(e))
	}
	if !b(page, "truncated") || !b(page, "has_more") || len(page["records"].([]any)) != 256 {
		t.Fatal("pagination or retention failed")
	}
	cursor := n(page, "cursor")
	next, e := r.DiagnosticPage(id, cursor)
	if e != nil || n(next, "cursor") <= cursor || b(next, "truncated") {
		t.Fatal("cursor did not advance")
	}
	info, e := os.Stat(r.DiagnosticsPath(id))
	if e != nil || info.Size() > maxLogBytes {
		t.Fatal("log size exceeded cap")
	}
}
func TestLogsUnknownSourceIsNotExposed(t *testing.T) {
	r, id := logRuntime(t)
	ctx := &logContext{Step: "packer-build"}
	if r.diagnosticOutput(id, ctx, []byte(`1,,ui,error,Error: Invalid expression\n  on SECRET_MARKER.pkr.hcl line 1:`)) != nil {
		t.Fatal("parse failed")
	}
	page, e := r.DiagnosticPage(id, 0)
	if e != nil {
		t.Fatal(safeCode(e))
	}
	raw, _ := json.Marshal(page)
	if containsBytes(raw, []byte("SECRET_MARKER")) {
		t.Fatal("unapproved source exposed")
	}
}
func TestNonBuildAndBinaryOutputCannotForgePackerLogs(t *testing.T) {
	r, id := logRuntime(t)
	r.CurrentJob = id
	r.CurrentStep = "publish"
	_, e := r.Invoke([]string{"/bin/sh", "-c", "printf 'IMAGEBUILDER_STEP=packer-init\n'"}, CommandOptions{Digest: true})
	if e != nil {
		t.Fatal(safeCode(e))
	}
	_, rows, e := r.diagnostics(id)
	if e != nil || len(rows) != 2 || s(rows[0], "step") != "publish" || s(rows[1], "event") != "completed" {
		t.Fatal("binary output forged a progress event")
	}
}
func containsBytes(data, part []byte) bool { return bytes.Contains(data, part) }
