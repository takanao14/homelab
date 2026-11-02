package builder

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
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
	if !protocol.RevisionPattern.MatchString(revision) {
		return "", protocol.Fail("invalid_request")
	}
	checkout := filepath.Join(r.JobDir(id), "repo")
	if err := os.Mkdir(checkout, 0700); err != nil {
		return "", protocol.Fail("invalid_state")
	}
	commands := [][]string{
		{"git", "init", "--quiet", checkout},
		{"git", "-C", checkout, "remote", "add", "origin", r.Cfg.RepoURL},
		{"git", "-C", checkout, "fetch", "--depth=1", "origin", revision},
		{"git", "-C", checkout, "checkout", "--detach", revision},
	}
	for index, args := range commands {
		limit := time.Minute
		if index == 2 {
			limit = 5 * time.Minute
		}
		timeout, err := r.stepTimeout(limit)
		if err != nil {
			return "", err
		}
		if _, err := r.Invoke(args, command.Options{Env: env, Timeout: timeout}); err != nil {
			return "", err
		}
	}
	result, err := r.Invoke([]string{"git", "-C", checkout, "rev-parse", "HEAD"}, command.Options{Env: env, Capture: true})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(result.Output)) != revision {
		return "", protocol.Fail("revision_mismatch")
	}
	return checkout, nil
}

func workerTargetCommand(checkout, target string) ([]string, error) {
	if !protocol.TargetPattern.MatchString(target) {
		return nil, protocol.Fail("invalid_request")
	}
	packer := filepath.Join(checkout, "packer")
	info, err := os.Lstat(filepath.Join(packer, "vars", target+".pkrvars.hcl"))
	if target == "all" || err == nil && info.Mode().IsRegular() {
		return []string{"bash", "./build.sh", "-y", target}, nil
	}
	data, err := os.ReadFile(filepath.Join(packer, "import-upstream.sh"))
	if err != nil {
		return nil, protocol.Fail("unknown_target")
	}
	function := workerImportFunction.FindSubmatch(data)
	if len(function) == 2 {
		for _, match := range workerImportTarget.FindAllSubmatch(function[1], -1) {
			if string(match[1]) == target {
				return []string{"bash", "./import-upstream.sh", "-y", target}, nil
			}
		}
	}
	return nil, protocol.Fail("unknown_target")
}

func (r *Runtime) workerSSHEnvironment(id string, env map[string]string) (int, error) {
	r.CurrentStep = "ssh-setup"
	key := filepath.Join(r.WorkerPath(id), "id_ed25519")
	if _, err := r.Invoke([]string{"ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key}, command.Options{Env: env}); err != nil {
		return 0, err
	}
	result, err := r.Invoke([]string{"ssh-agent", "-s"}, command.Options{Env: env, Capture: true})
	if err != nil {
		return 0, err
	}
	socket, pid := workerAgentSocket.FindSubmatch(result.Output), workerAgentPID.FindSubmatch(result.Output)
	if len(socket) != 2 || len(pid) != 2 {
		return 0, protocol.Fail("ssh_agent_failed")
	}
	pidValue, err := strconv.Atoi(string(pid[1]))
	if err != nil || pidValue <= 0 {
		return 0, protocol.Fail("ssh_agent_failed")
	}
	env["SSH_AUTH_SOCK"], env["SSH_AGENT_PID"] = string(socket[1]), string(pid[1])
	if _, err := r.Invoke([]string{"ssh-add", key}, command.Options{Env: env}); err != nil {
		return pidValue, err
	}
	public, err := os.ReadFile(key + ".pub")
	if err != nil || !workerPublicKey.MatchString(strings.TrimSpace(string(public))) {
		return pidValue, protocol.Fail("ssh_key_failed")
	}
	env["PKR_VAR_ssh_pubkey"] = strings.TrimSpace(string(public))
	return pidValue, nil
}

func (r *Runtime) Run(id string) (result error) {
	if !protocol.ValidJobID(id) {
		return protocol.Fail("invalid_request")
	}
	request, err := store.ReadJSON(r.RequestPath(id))
	if err != nil {
		return err
	}
	secretValues, err := credentials(request["credentials"], protocol.Bool(request, "publish"))
	if err != nil {
		return err
	}
	env := protocol.BaseEnv()
	env["HOME"], env["PACKER_CACHE_DIR"], env["TMPDIR"] = r.Home, filepath.Join(r.Cfg.WorkDir, "cache", "packer"), r.WorkerPath(id)
	for variable, value := range map[string]string{"SUPERMIN_KERNEL": r.Cfg.KernelPath,
		"SUPERMIN_KERNEL_VERSION": r.Cfg.KernelVersion, "SUPERMIN_MODULES": r.Cfg.KernelModules} {
		if value != "" {
			env[variable] = value
		}
	}
	agentPID := 0
	r.CurrentJob, r.CurrentStep, r.Deadline = id, "checkout", r.now().Add(workerSeconds(r.Cfg.TimeoutSeconds))
	defer func() {
		if result != nil {
			result = protocol.Fail(protocol.SafeCode(result))
			if err := r.Update(id, "failed", protocol.SafeCode(result), nil); err != nil {
				result = protocol.Fail(protocol.SafeCode(err))
			}
		}
		r.CurrentJob, r.CurrentStep, r.Deadline = "", "", time.Time{}
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
	if protocol.String(request, "op") == "publish-retry" {
		value, err := store.ReadValue(filepath.Join(r.JobDir(id), "verified.json"))
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
			return protocol.Fail("artifact_changed")
		}
	} else {
		checkout, err = r.checkoutWorker(id, protocol.String(request, "revision"), env)
		if err != nil {
			return err
		}
		args, err := workerTargetCommand(checkout, protocol.String(request, "target"))
		if err != nil {
			return err
		}
		if args[1] != "./import-upstream.sh" && secretValues["PKR_VAR_user_password"] == "" {
			return protocol.Fail("missing_credentials")
		}
		agentPID, err = r.workerSSHEnvironment(id, env)
		if err != nil {
			return err
		}
		if err := r.Update(id, "building", "building", nil); err != nil {
			return err
		}
		buildEnv := protocol.CopyEnv(env)
		if value := secretValues["PKR_VAR_user_password"]; value != "" {
			buildEnv["PKR_VAR_user_password"] = value
		}
		buildEnv["IMAGEBUILDER_LOG_EVENTS"] = "1"
		r.CurrentStep = "build"
		if args[1] == "./import-upstream.sh" {
			r.CurrentStep = "import"
		}
		timeout, err := r.stepTimeout(0)
		if err != nil {
			return err
		}
		if _, err := r.Invoke(args, command.Options{Dir: filepath.Join(checkout, "packer"), Env: buildEnv, Timeout: timeout}); err != nil {
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
		if err := store.WriteJSON(filepath.Join(r.JobDir(id), "verified.json"), manifest, 0600); err != nil {
			return err
		}
	}
	if protocol.Bool(request, "publish") {
		if err := r.Update(id, "publishing", "publishing", nil); err != nil {
			return err
		}
		publishEnv := protocol.CopyEnv(env)
		for key, value := range secretValues {
			if key != "PKR_VAR_user_password" {
				publishEnv[key] = value
			}
		}
		if err := r.workerPublish(checkout, protocol.String(request, "target"), manifest, publishEnv); err != nil {
			return err
		}
	}
	fields := protocol.Object{}
	if len(manifest) == 1 {
		fields["sha256"] = manifest[0].SHA256
	}
	code := "verified"
	if protocol.Bool(request, "publish") {
		code = "published"
	}
	return r.Update(id, "succeeded", code, fields)
}
