package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	workerImportFunction = regexp.MustCompile(`(?s)import_target\(\)\s*\{(.*?)\n\}`)
	workerImportTarget   = regexp.MustCompile(`(?m)^\s*([a-z][a-z0-9-]*)\)`)
	workerAgentSocket    = regexp.MustCompile(`(?m)^SSH_AUTH_SOCK=([^;\n]+);`)
	workerAgentPID       = regexp.MustCompile(`(?m)^SSH_AGENT_PID=([0-9]+);`)
	workerPublicKey      = regexp.MustCompile(`^ssh-ed25519 [A-Za-z0-9+/=]+(?: [^\r\n]*)?$`)
)

func workerSeconds(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

func (r *Runtime) checkoutWorker(id, revision string, env map[string]string) (string, error) {
	r.CurrentStep = "checkout"
	if !revisionPattern.MatchString(revision) {
		return "", fail("invalid_request")
	}
	checkout := filepath.Join(r.JobDir(id), "repo")
	if err := os.Mkdir(checkout, 0700); err != nil {
		return "", fail("invalid_state")
	}
	commands := [][]string{
		{"git", "init", "--quiet", checkout},
		{"git", "-C", checkout, "remote", "add", "origin", r.Cfg.RepoURL},
		{"git", "-C", checkout, "fetch", "--depth=1", "origin", revision},
		{"git", "-C", checkout, "checkout", "--detach", revision},
	}
	for index, args := range commands {
		timeout := time.Minute
		if index == 2 {
			timeout = 5 * time.Minute
		}
		if _, err := r.Invoke(args, CommandOptions{Env: env, Timeout: timeout}); err != nil {
			return "", err
		}
	}
	result, err := r.Invoke([]string{"git", "-C", checkout, "rev-parse", "HEAD"}, CommandOptions{Env: env, Capture: true})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(result.Output)) != revision {
		return "", fail("revision_mismatch")
	}
	return checkout, nil
}

func workerTargetCommand(checkout, target string) ([]string, error) {
	if !targetPattern.MatchString(target) {
		return nil, fail("invalid_request")
	}
	packer := filepath.Join(checkout, "packer")
	info, err := os.Lstat(filepath.Join(packer, "vars", target+".pkrvars.hcl"))
	if target == "all" || err == nil && info.Mode().IsRegular() {
		return []string{"bash", "./build.sh", "-y", target}, nil
	}
	data, err := os.ReadFile(filepath.Join(packer, "import-upstream.sh"))
	if err != nil {
		return nil, fail("unknown_target")
	}
	function := workerImportFunction.FindSubmatch(data)
	if len(function) == 2 {
		for _, match := range workerImportTarget.FindAllSubmatch(function[1], -1) {
			if string(match[1]) == target {
				return []string{"bash", "./import-upstream.sh", "-y", target}, nil
			}
		}
	}
	return nil, fail("unknown_target")
}

func (r *Runtime) workerSSHEnvironment(id string, env map[string]string) (int, error) {
	r.CurrentStep = "ssh-setup"
	key := filepath.Join(r.WorkerPath(id), "id_ed25519")
	if _, err := r.Invoke([]string{"ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key}, CommandOptions{Env: env}); err != nil {
		return 0, err
	}
	result, err := r.Invoke([]string{"ssh-agent", "-s"}, CommandOptions{Env: env, Capture: true})
	if err != nil {
		return 0, err
	}
	socket, pid := workerAgentSocket.FindSubmatch(result.Output), workerAgentPID.FindSubmatch(result.Output)
	if len(socket) != 2 || len(pid) != 2 {
		return 0, fail("ssh_agent_failed")
	}
	pidValue, err := strconv.Atoi(string(pid[1]))
	if err != nil || pidValue <= 0 {
		return 0, fail("ssh_agent_failed")
	}
	env["SSH_AUTH_SOCK"], env["SSH_AGENT_PID"] = string(socket[1]), string(pid[1])
	if _, err := r.Invoke([]string{"ssh-add", key}, CommandOptions{Env: env}); err != nil {
		return pidValue, err
	}
	public, err := os.ReadFile(key + ".pub")
	if err != nil || !workerPublicKey.MatchString(strings.TrimSpace(string(public))) {
		return pidValue, fail("ssh_key_failed")
	}
	env["PKR_VAR_ssh_pubkey"] = strings.TrimSpace(string(public))
	return pidValue, nil
}

func (r *Runtime) Run(id string) (result error) {
	if !validJobID(id) {
		return fail("invalid_request")
	}
	request, err := readJSON(r.RequestPath(id))
	if err != nil {
		return err
	}
	secretValues, err := credentials(request["credentials"], b(request, "publish"))
	if err != nil {
		return err
	}
	env := baseEnv()
	env["HOME"], env["PACKER_CACHE_DIR"], env["TMPDIR"] = r.Home, filepath.Join(r.Cfg.WorkDir, "cache", "packer"), r.WorkerPath(id)
	for variable, value := range map[string]string{"SUPERMIN_KERNEL": r.Cfg.KernelPath,
		"SUPERMIN_KERNEL_VERSION": r.Cfg.KernelVersion, "SUPERMIN_MODULES": r.Cfg.KernelModules} {
		if value != "" {
			env[variable] = value
		}
	}
	agentPID := 0
	r.CurrentJob, r.CurrentStep = id, "checkout"
	defer func() {
		if result != nil {
			result = fail(safeCode(result))
			if err := r.Update(id, "failed", safeCode(result), nil); err != nil {
				result = fail(safeCode(err))
			}
		}
		r.CurrentJob, r.CurrentStep = "", ""
		if agentPID > 0 {
			if process, err := os.FindProcess(agentPID); err == nil {
				_ = process.Signal(syscall.SIGTERM)
			}
		}
	}()
	if err := r.Update(id, "preparing", "preparing", nil); err != nil {
		return err
	}
	checkout := filepath.Join(r.JobDir(id), "repo")
	var manifest []workerArtifact
	if s(request, "op") == "publish-retry" {
		value, err := readValue(filepath.Join(r.JobDir(id), "verified.json"))
		if err != nil {
			return err
		}
		manifest, err = workerManifest(value)
		if err != nil {
			return err
		}
		actual, err := r.workerArtifacts(checkout)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(actual, manifest) {
			return fail("artifact_changed")
		}
	} else {
		checkout, err = r.checkoutWorker(id, s(request, "revision"), env)
		if err != nil {
			return err
		}
		args, err := workerTargetCommand(checkout, s(request, "target"))
		if err != nil {
			return err
		}
		if args[1] != "./import-upstream.sh" && secretValues["PKR_VAR_user_password"] == "" {
			return fail("missing_credentials")
		}
		agentPID, err = r.workerSSHEnvironment(id, env)
		if err != nil {
			return err
		}
		if err := r.Update(id, "building", "building", nil); err != nil {
			return err
		}
		buildEnv := copyEnv(env)
		if value := secretValues["PKR_VAR_user_password"]; value != "" {
			buildEnv["PKR_VAR_user_password"] = value
		}
		buildEnv["IMAGEBUILDER_LOG_EVENTS"] = "1"
		r.CurrentStep = "build"
		if args[1] == "./import-upstream.sh" {
			r.CurrentStep = "import"
		}
		if _, err := r.Invoke(args, CommandOptions{Dir: filepath.Join(checkout, "packer"), Env: buildEnv, Timeout: workerSeconds(r.Cfg.TimeoutSeconds)}); err != nil {
			return err
		}
		if err := r.Update(id, "verifying", "verifying", nil); err != nil {
			return err
		}
		manifest, err = r.workerArtifacts(checkout)
		if err != nil {
			return err
		}
		for _, artifact := range manifest {
			if err := r.workerSmoke(id, filepath.Join(checkout, "packer", "images", artifact.Name), env); err != nil {
				return err
			}
		}
		if err := writeJSON(filepath.Join(r.JobDir(id), "verified.json"), manifest, 0600); err != nil {
			return err
		}
	}
	if b(request, "publish") {
		if err := r.Update(id, "publishing", "publishing", nil); err != nil {
			return err
		}
		publishEnv := copyEnv(env)
		for key, value := range secretValues {
			if key != "PKR_VAR_user_password" {
				publishEnv[key] = value
			}
		}
		if err := r.workerPublish(checkout, s(request, "target"), manifest, publishEnv); err != nil {
			return err
		}
	}
	fields := Object{}
	if len(manifest) == 1 {
		fields["sha256"] = manifest[0].SHA256
	}
	code := "verified"
	if b(request, "publish") {
		code = "published"
	}
	return r.Update(id, "succeeded", code, fields)
}
