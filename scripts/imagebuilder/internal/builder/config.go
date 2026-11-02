package builder

import (
	"bytes"
	"encoding/json"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

type Config struct {
	RepoURL             string `json:"repo_url"`
	WorkDir             string `json:"work_dir"`
	RuntimeDir          string `json:"runtime_dir"`
	User                string `json:"user"`
	KernelPath          string `json:"kernel_path,omitempty"`
	KernelVersion       string `json:"kernel_version,omitempty"`
	KernelModules       string `json:"kernel_modules,omitempty"`
	MinimumFreeBytes    int64  `json:"minimum_free_bytes"`
	CacheMaxBytes       int64  `json:"cache_max_bytes"`
	IdleSeconds         int    `json:"idle_seconds"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
	RetentionDays       int    `json:"retention_days"`
	SmokeTimeoutSeconds int    `json:"smoke_timeout_seconds"`
	RequireMount        bool   `json:"require_mount"`
}

var (
	repoURLPattern       = regexp.MustCompile(`^https://[A-Za-z0-9.-]+/[A-Za-z0-9_./-]+$`)
	kernelVersionPattern = regexp.MustCompile(`^[0-9][A-Za-z0-9._+-]{0,127}$`)
	kernelModulesPattern = regexp.MustCompile(`^/(?:usr/)?lib/modules/[0-9][A-Za-z0-9._+-]*$`)
	kernelPathPattern    = regexp.MustCompile(`^/[A-Za-z0-9._/+-]+$`)
)

func LoadConfig(path string) (Config, error) {
	v, e := store.ReadValue(path)
	if e != nil {
		return Config{}, protocol.Fail("invalid_config")
	}
	raw, e := json.Marshal(v)
	if e != nil {
		return Config{}, protocol.Fail("invalid_config")
	}
	c := Config{User: "imagebuilder", RequireMount: true, CacheMaxBytes: 20 * 1024 * 1024 * 1024}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return Config{}, protocol.Fail("invalid_config")
	}
	return c, nil
}

func NewRuntime(c Config) (*Runtime, error) {
	if !filepath.IsAbs(c.WorkDir) || !filepath.IsAbs(c.RuntimeDir) || !repoURLPattern.MatchString(c.RepoURL) || c.MinimumFreeBytes <= 0 || c.CacheMaxBytes <= 0 || c.IdleSeconds <= 0 || c.TimeoutSeconds <= 0 || c.RetentionDays <= 0 || c.SmokeTimeoutSeconds <= 0 {
		return nil, protocol.Fail("invalid_config")
	}
	if c.KernelVersion != "" && !kernelVersionPattern.MatchString(c.KernelVersion) {
		return nil, protocol.Fail("invalid_config")
	}
	if c.KernelModules != "" && (!kernelModulesPattern.MatchString(c.KernelModules) || (c.KernelVersion != "" && filepath.Base(c.KernelModules) != c.KernelVersion)) {
		return nil, protocol.Fail("invalid_config")
	}
	if c.KernelPath != "" && (!kernelPathPattern.MatchString(c.KernelPath) || strings.Contains(c.KernelPath, "/../") || strings.HasSuffix(c.KernelPath, "/..")) {
		return nil, protocol.Fail("invalid_config")
	}
	u, e := user.Lookup(c.User)
	if e != nil {
		return nil, protocol.Fail("invalid_config")
	}
	uid, e := strconv.Atoi(u.Uid)
	if e != nil {
		return nil, protocol.Fail("invalid_config")
	}
	gid, e := strconv.Atoi(u.Gid)
	if e != nil {
		return nil, protocol.Fail("invalid_config")
	}
	return &Runtime{Cfg: c, UID: uid, GID: gid, UserName: u.Username, Home: u.HomeDir}, nil
}
