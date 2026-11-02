package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

type Object = map[string]any

type safeError string

func (e safeError) Error() string { return string(e) }
func fail(code string) error      { return safeError(code) }
func safeCode(err error) string {
	if e, ok := err.(safeError); ok && codeSet[string(e)] {
		return string(e)
	}
	return "internal_error"
}
func set(values ...string) map[string]bool {
	r := make(map[string]bool, len(values))
	for _, v := range values {
		r[v] = true
	}
	return r
}

var phaseSet = set("preparing", "building", "verifying", "publishing", "succeeded", "failed", "interrupted")
var terminalSet = set("succeeded", "failed", "interrupted")
var codeSet = set(
	"accepted", "preparing", "building", "verifying", "publishing", "published", "verified",
	"invalid_request", "invalid_state", "invalid_config", "missing_credentials", "command_timeout",
	"invalid_command_output", "command_failed", "work_disk_not_mounted", "kvm_unavailable",
	"kernel_unavailable", "insufficient_space", "missing_tool", "stopping", "maintenance", "busy",
	"start_failed", "job_not_finished", "no_verified_artifact", "shutdown_failed",
	"service_failed", "invalid_progress", "host_restarted", "revision_mismatch", "unknown_target",
	"ssh_agent_failed", "ssh_key_failed", "invalid_artifact", "checksum_mismatch", "no_artifacts",
	"smoke_boot_failed", "smoke_timeout", "remote_checksum_mismatch",
	"artifact_changed", "internal_error",
	"invalid_branch", "branch_resolution_failed", "invalid_builder_response", "invalid_proxmox_url",
	"invalid_node", "missing_proxmox_token", "invalid_ssh_host", "invalid_ssh_user", "invalid_vmid", "invalid_proxmox_ca",
	"invalid_proxmox_response", "proxmox_request_failed", "builder_not_found", "unexpected_vm_identity",
	"builder_request_failed", "builder_unreachable", "builder_start_timeout", "missing_build_password",
	"missing_publish_credentials", "unexpected_job_identity", "job_failed", "job_wait_timeout",
	"shutdown_timeout", "log_wait_timeout", "invalid_revision", "invalid_target", "invalid_job_id",
	"invalid_log_cursor", "invalid_arguments", "controller_interrupted_job_may_continue", "controller_failed",
)
var credentialSet = set("PKR_VAR_user_password", "SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY", "SEAWEEDFS_CLOUD_IMAGES_BUCKET")
var jobPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var revisionPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var targetPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var branchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

func validJobID(v string) bool { return jobPattern.MatchString(v) }
func validateBranch(v any) (string, error) {
	x, ok := v.(string)
	if !ok || !branchPattern.MatchString(x) || strings.Contains(x, "..") || strings.Contains(x, "//") || strings.HasSuffix(x, "/") || strings.HasSuffix(x, ".") {
		return "", fail("invalid_branch")
	}
	for _, p := range strings.Split(x, "/") {
		if strings.HasPrefix(p, ".") || strings.HasSuffix(p, ".lock") {
			return "", fail("invalid_branch")
		}
	}
	return x, nil
}
func s(o Object, key string) string { v, _ := o[key].(string); return v }
func b(o Object, key string) bool   { v, _ := o[key].(bool); return v }
func num(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case json.Number:
		n, e := x.Int64()
		return n, e == nil
	}
	return 0, false
}
func n(o Object, key string) int64 { v, _ := num(o[key]); return v }
func keysAllowed(o Object, names ...string) bool {
	a := set(names...)
	for k := range o {
		if !a[k] {
			return false
		}
	}
	return true
}
func copyEnv(e map[string]string) map[string]string {
	r := map[string]string{}
	for k, v := range e {
		r[k] = v
	}
	return r
}
func baseEnv() map[string]string {
	return map[string]string{"PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "C.UTF-8", "LIBGUESTFS_BACKEND": "direct", "GIT_TERMINAL_PROMPT": "0"}
}
func credentials(value any, publish bool) (map[string]string, error) {
	o, ok := value.(map[string]any)
	if !ok {
		return nil, fail("invalid_request")
	}
	r := map[string]string{}
	for k, v := range o {
		x, ok := v.(string)
		if !credentialSet[k] || !ok || len(x) == 0 || len(x) > 16384 || strings.ContainsRune(x, 0) {
			return nil, fail("invalid_request")
		}
		r[k] = x
	}
	if publish {
		for _, k := range []string{"SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY"} {
			if r[k] == "" {
				return nil, fail("missing_credentials")
			}
		}
		if !regexp.MustCompile(`^https://[A-Za-z0-9.-]+(?::[0-9]+)?/?$`).MatchString(r["SEAWEEDFS_S3_ENDPOINT"]) {
			return nil, fail("invalid_request")
		}
		bucket := r["SEAWEEDFS_CLOUD_IMAGES_BUCKET"]
		if bucket == "" {
			bucket = "cloud-images"
		}
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(bucket) {
			return nil, fail("invalid_request")
		}
	}
	return r, nil
}
