package builder

import (
	"path/filepath"
	"syscall"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func mounted(path string) bool {
	path = filepath.Clean(path)
	var current, parent syscall.Stat_t
	if syscall.Stat(path, &current) != nil || syscall.Stat(filepath.Dir(path), &parent) != nil {
		return false
	}
	return current.Dev != parent.Dev || current.Ino == parent.Ino
}

func (r *Runtime) Readiness() (protocol.Object, error) {
	if r.ReadyHook != nil {
		return r.ReadyHook()
	}
	if r.Cfg.RequireMount && !mounted(r.Cfg.WorkDir) {
		return nil, protocol.Fail("work_disk_not_mounted")
	}
	if syscall.Access("/dev/kvm", 6) != nil {
		return nil, protocol.Fail("kvm_unavailable")
	}
	for _, permission := range []string{"-r", "-w"} {
		if _, err := r.Invoke([]string{"runuser", "-u", r.UserName, "--", "test", permission, "/dev/kvm"}, command.Options{}); err != nil {
			return nil, protocol.Fail("kvm_unavailable")
		}
	}
	for _, entry := range []struct{ path, kind, permission string }{{r.Cfg.KernelPath, "-f", "-r"}, {r.Cfg.KernelModules, "-d", "-x"}} {
		if entry.path == "" {
			continue
		}
		for _, permission := range []string{entry.kind, entry.permission} {
			if _, err := r.Invoke([]string{"runuser", "-u", r.UserName, "--", "test", permission, entry.path}, command.Options{}); err != nil {
				return nil, protocol.Fail("kernel_unavailable")
			}
		}
	}
	var usage syscall.Statfs_t
	if syscall.Statfs(r.Cfg.WorkDir, &usage) != nil {
		return nil, protocol.Fail("invalid_state")
	}
	if int64(usage.Bavail)*int64(usage.Bsize) < r.Cfg.MinimumFreeBytes {
		return nil, protocol.Fail("insufficient_space")
	}
	for _, tool := range []string{"git", "packer", "qemu-img", "qemu-system-x86_64", "virt-sparsify", "rclone", "ssh", "ssh-agent", "ssh-add", "ssh-keygen", "cloud-localds", "curl", "xz"} {
		if command.LookPath(tool, protocol.BaseEnv()) == "" {
			return nil, protocol.Fail("missing_tool")
		}
	}
	if _, err := r.Invoke([]string{"cloud-init", "status", "--wait"}, command.Options{Timeout: 120 * time.Second}); err != nil {
		return nil, err
	}
	return protocol.Object{"ok": true, "phase": "ready"}, nil
}
