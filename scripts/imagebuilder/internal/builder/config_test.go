package builder

import (
	"os/user"
	"path/filepath"
	"testing"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

func TestConfigRejectsParentKernelPathAndPinsDefaults(t *testing.T) {
	u, e := user.Current()
	if e != nil {
		t.Fatal("user unavailable")
	}
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	data := protocol.Object{"repo_url": "https://github.com/takanao14/homelab.git", "work_dir": root, "runtime_dir": filepath.Join(root, "run"), "user": u.Username, "minimum_free_bytes": 1, "idle_seconds": 600, "timeout_seconds": 14400, "retention_days": 7, "smoke_timeout_seconds": 300, "kernel_path": "/var/lib/imagebuilder-kernel/.."}
	if store.WriteJSON(path, data, 0600) != nil {
		t.Fatal("fixture failed")
	}
	cfg, e := LoadConfig(path)
	if e != nil {
		t.Fatal("config load failed")
	}
	if !cfg.RequireMount || cfg.CacheMaxBytes != 20*1024*1024*1024 {
		t.Fatal("config defaults changed")
	}
	if _, e := NewRuntime(cfg); protocol.SafeCode(e) != "invalid_config" {
		t.Fatal("parent kernel path accepted")
	}
}
