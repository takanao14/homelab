package main

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestProtocolPreservesLegacyJSONIntegersAndRejectsNonIntegers(t *testing.T) {
	data, e := decodeObject([]byte(`{"sequence":9223372036854775807,"enabled":true}`))
	if e != nil {
		t.Fatal("legacy JSON rejected")
	}
	v, ok := num(data["sequence"])
	if !ok || v != 9223372036854775807 {
		t.Fatal("integer precision lost")
	}
	for _, raw := range []any{true, json.Number("1.0"), json.Number("1e2"), json.Number("9223372036854775808")} {
		if _, ok := num(raw); ok {
			t.Fatal("invalid integer accepted")
		}
	}
}
func TestProtocolRejectsSymlinkAndFIFOState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	if os.Symlink("/etc/passwd", path) != nil {
		t.Fatal("fixture failed")
	}
	if _, e := readJSON(path); safeCode(e) != "invalid_state" {
		t.Fatal("symlink followed")
	}
	if os.Remove(path) != nil || syscall.Mkfifo(path, 0600) != nil {
		t.Fatal("fixture failed")
	}
	if _, e := readJSON(path); safeCode(e) != "invalid_state" {
		t.Fatal("FIFO accepted")
	}
}
func TestProtocolConfigRejectsParentKernelPathAndPinsDefaults(t *testing.T) {
	u, e := user.Current()
	if e != nil {
		t.Fatal("user unavailable")
	}
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	data := Object{"repo_url": "https://github.com/takanao14/homelab.git", "work_dir": root, "runtime_dir": filepath.Join(root, "run"), "user": u.Username, "minimum_free_bytes": 1, "idle_seconds": 600, "timeout_seconds": 14400, "retention_days": 7, "smoke_timeout_seconds": 300, "kernel_path": "/var/lib/imagebuilder-kernel/.."}
	if writeJSON(path, data, 0600) != nil {
		t.Fatal("fixture failed")
	}
	cfg, e := LoadConfig(path)
	if e != nil {
		t.Fatal("config load failed")
	}
	if !cfg.RequireMount || cfg.CacheMaxBytes != 20*1024*1024*1024 {
		t.Fatal("config defaults changed")
	}
	if _, e := NewRuntime(cfg); safeCode(e) != "invalid_config" {
		t.Fatal("parent kernel path accepted")
	}
}
func TestProtocolUnknownErrorsAreValueFree(t *testing.T) {
	if safeCode(fail("SECRET_MARKER")) != "internal_error" {
		t.Fatal("unapproved error exposed")
	}
	if safeCode(fail("invalid_proxmox_ca")) != "invalid_proxmox_ca" {
		t.Fatal("approved error lost")
	}
	if _, e := decodeObject([]byte(`{"ok":true} SECRET_MARKER`)); e == nil || strings.Contains(e.Error(), "SECRET_MARKER") {
		t.Fatal("invalid input leaked")
	}
}
