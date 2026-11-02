package builder

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func (r *Runtime) prune(state *ControlState) error {
	cutoff := r.now().Unix() - int64(r.Cfg.RetentionDays)*86400
	paths, err := filepath.Glob(filepath.Join(r.recordsDir(), "*.json"))
	if err != nil {
		return protocol.Fail("invalid_state")
	}
	var result error
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		record, err := readJobRecord(path)
		if !protocol.ValidJobID(id) || err != nil {
			result = protocol.Fail("invalid_state")
			continue
		}
		if record.Finished && record.Created < cutoff && id != state.Active {
			if os.RemoveAll(r.JobDir(id)) != nil || os.Remove(path) != nil {
				result = protocol.Fail("invalid_state")
			}
		}
	}
	return result
}

// pruneCache walks the Packer cache, so it runs once per boot instead of on every idle tick.
func (r *Runtime) pruneCache() error {
	if !exists(filepath.Join(r.cacheDir(), "packer")) {
		return nil
	}
	_, err := r.Invoke([]string{"runuser", "-u", r.UserName, "--", "/usr/local/libexec/imagebuilder", "cache-prune"}, command.Options{})
	return err
}

func (r *Runtime) PruneCache() error {
	if os.Geteuid() != r.UID {
		return protocol.Fail("invalid_config")
	}
	cache := filepath.Join(r.cacheDir(), "packer")
	info, err := os.Lstat(cache)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return protocol.Fail("invalid_state")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(cache); err != nil {
			return protocol.Fail("invalid_state")
		}
		return nil
	}
	if !info.IsDir() {
		return nil
	}
	var total int64
	var newest time.Time
	count := 0
	err = filepath.WalkDir(cache, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return protocol.Fail("invalid_state")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return protocol.Fail("invalid_state")
		}
		if info.Mode().IsRegular() {
			total += info.Size()
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		count++
		if count > 100000 {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return protocol.Fail("invalid_state")
	}
	if count > 100000 || total > r.Cfg.CacheMaxBytes || (count > 0 && newest.Before(r.now().Add(-time.Duration(r.Cfg.RetentionDays)*24*time.Hour))) {
		if err := os.RemoveAll(cache); err != nil {
			return protocol.Fail("invalid_state")
		}
	}
	return nil
}
