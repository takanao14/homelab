package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type workerArtifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type workerSnapshot struct {
	Name    string  `json:"name"`
	Present bool    `json:"present"`
	SHA256  *string `json:"sha256"`
}

var (
	workerImagePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]+\.img$`)
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

func workerFileDigest(path string) (string, error) {
	file, err := openNoFollow(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", fail("invalid_artifact")
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", fail("invalid_artifact")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fail("invalid_artifact")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func workerReadSmallFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, fail("invalid_artifact")
	}
	file, err := openNoFollow(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fail("invalid_artifact")
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, fail("invalid_artifact")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, fail("invalid_artifact")
	}
	return data, nil
}

func (r *Runtime) workerArtifacts(checkout string) ([]workerArtifact, error) {
	r.CurrentStep = "artifact-check"
	images, err := filepath.Glob(filepath.Join(checkout, "packer", "images", "*.img"))
	if err != nil {
		return nil, fail("invalid_artifact")
	}
	sort.Strings(images)
	result := make([]workerArtifact, 0, len(images))
	for _, image := range images {
		info, err := os.Lstat(image)
		if err != nil || !info.Mode().IsRegular() || !workerImagePattern.MatchString(filepath.Base(image)) {
			return nil, fail("invalid_artifact")
		}
		digest, err := workerFileDigest(image)
		if err != nil {
			return nil, err
		}
		sidecar, err := workerReadSmallFile(image+".sha256", 65)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(sidecar)) != digest {
			return nil, fail("checksum_mismatch")
		}
		if _, err := r.Invoke([]string{"qemu-img", "check", "-f", "qcow2", image}, CommandOptions{Timeout: 5 * time.Minute}); err != nil {
			return nil, err
		}
		result = append(result, workerArtifact{Name: filepath.Base(image), SHA256: digest})
	}
	if len(result) == 0 {
		return nil, fail("no_artifacts")
	}
	return result, nil
}

func workerManifest(value any) ([]workerArtifact, error) {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, fail("invalid_artifact")
	}
	result := make([]workerArtifact, 0, len(items))
	previous := ""
	for _, item := range items {
		entry, ok := item.(Object)
		if !ok || len(entry) != 2 || !keysAllowed(entry, "name", "sha256") ||
			!workerImagePattern.MatchString(s(entry, "name")) || !shaPattern.MatchString(s(entry, "sha256")) ||
			s(entry, "name") <= previous {
			return nil, fail("invalid_artifact")
		}
		previous = s(entry, "name")
		result = append(result, workerArtifact{Name: previous, SHA256: s(entry, "sha256")})
	}
	return result, nil
}

func workerSmokeConfiguration(id, image, public string) (Object, string, error) {
	if !validJobID(id) || !workerPublicKey.MatchString(public) {
		return nil, "", fail("invalid_request")
	}
	marker := "/var/tmp/imagebuilder-smoke-" + id
	user := Object{"name": "imagecheck", "shell": "/bin/sh", "ssh_authorized_keys": []string{public}}
	configuration := Object{"users": []Object{user}, "ssh_pwauth": false, "runcmd": []string{"touch " + marker}}
	check := "test -f " + marker
	// FreeBSD BASIC-CLOUDINIT uses nuageinit without sudo or Python cloud-init.
	if !strings.HasPrefix(filepath.Base(image), "freebsd-") {
		user["sudo"], user["lock_passwd"] = "ALL=(ALL) NOPASSWD:ALL", true
		check = "sudo -n cloud-init status --wait && systemctl is-active --quiet qemu-guest-agent && " + check
	}
	return configuration, check, nil
}

func (r *Runtime) workerSmoke(id, image string, env map[string]string) error {
	r.CurrentStep = "smoke-boot"
	if r.SmokeHook != nil {
		return r.SmokeHook(id, image, env)
	}
	directory := r.WorkerPath(id)
	overlay, seed := filepath.Join(r.JobDir(id), "smoke.qcow2"), filepath.Join(directory, "seed.img")
	agentSocket := filepath.Join(directory, "agent.sock")
	configuration, check, err := workerSmokeConfiguration(id, image, env["PKR_VAR_ssh_pubkey"])
	if err != nil {
		return err
	}
	data, err := json.Marshal(configuration)
	if err != nil || os.WriteFile(filepath.Join(directory, "user-data"), append([]byte("#cloud-config\n"), data...), 0600) != nil {
		return fail("invalid_state")
	}
	if err := writeJSON(filepath.Join(directory, "meta-data"), Object{"instance-id": id}, 0600); err != nil {
		return err
	}
	defer os.Remove(overlay)
	defer os.Remove(seed)
	// Sequential smoke boots reuse the job directory, including the Unix socket path.
	if err := os.Remove(agentSocket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail("smoke_boot_failed")
	}
	defer os.Remove(agentSocket)
	if _, err := r.Invoke([]string{"cloud-localds", seed, filepath.Join(directory, "user-data"), filepath.Join(directory, "meta-data")}, CommandOptions{Env: env}); err != nil {
		return err
	}
	if _, err := r.Invoke([]string{"qemu-img", "create", "-f", "qcow2", "-F", "qcow2", "-b", image, overlay}, CommandOptions{Env: env}); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fail("smoke_boot_failed")
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	process := exec.Command(commandPath("qemu-system-x86_64", env), "-enable-kvm", "-cpu", "host", "-m", "2048", "-smp", "2",
		"-display", "none", "-serial", "none", "-monitor", "none",
		"-drive", "file="+overlay+",format=qcow2,if=virtio", "-drive", "file="+seed+",format=raw,media=cdrom,readonly=on",
		"-netdev", "user,id=net0,hostfwd=tcp:127.0.0.1:"+port+"-:22", "-device", "virtio-net-pci,netdev=net0",
		"-device", "virtio-serial-pci", "-chardev", "socket,path="+agentSocket+",server=on,wait=off,id=agent",
		"-device", "virtserialport,chardev=agent,name=org.qemu.guest_agent.0")
	process.Env = commandEnv(env)
	if err := process.Start(); err != nil {
		return fail("smoke_boot_failed")
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	defer func() {
		_ = process.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = process.Process.Kill()
			<-done
		}
	}()
	deadline := time.Now().Add(workerSeconds(r.Cfg.SmokeTimeoutSeconds))
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			done <- err
			return fail("smoke_boot_failed")
		default:
		}
		_, err := r.Invoke([]string{"ssh", "-F", "/dev/null", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5",
			"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-i", filepath.Join(directory, "id_ed25519"),
			"-p", port, "imagecheck@127.0.0.1", check}, CommandOptions{Env: env, Timeout: 15 * time.Second})
		if err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fail("smoke_timeout")
}

func workerRemoteEnvironment(env map[string]string) (map[string]string, error) {
	result := copyEnv(env)
	result["RCLONE_CONFIG_SEAWEEDFS_TYPE"] = "s3"
	result["RCLONE_CONFIG_SEAWEEDFS_PROVIDER"] = "Other"
	result["RCLONE_CONFIG_SEAWEEDFS_REGION"] = "us-east-1"
	result["RCLONE_CONFIG_SEAWEEDFS_NO_CHECK_BUCKET"] = "true"
	result["RCLONE_CONFIG_SEAWEEDFS_USE_MULTIPART_UPLOADS"] = "false"
	for destination, source := range map[string]string{"ENDPOINT": "ENDPOINT", "ACCESS_KEY_ID": "ACCESS_KEY", "SECRET_ACCESS_KEY": "SECRET_KEY"} {
		value := env["SEAWEEDFS_S3_"+source]
		if value == "" {
			return nil, fail("missing_credentials")
		}
		result["RCLONE_CONFIG_SEAWEEDFS_"+destination] = value
	}
	return result, nil
}

func (r *Runtime) workerRemoteDigest(path string, env map[string]string) (string, error) {
	result, err := r.Invoke([]string{"rclone", "cat", path}, CommandOptions{Env: env, Timeout: workerSeconds(r.Cfg.TimeoutSeconds), Digest: true})
	if err != nil {
		return "", err
	}
	if !shaPattern.MatchString(result.Digest) {
		return "", fail("invalid_command_output")
	}
	return result.Digest, nil
}

func workerPublicationSnapshot(path string, manifest []workerArtifact) ([]workerSnapshot, error) {
	data, err := readJSON(path)
	if err != nil || len(data) != 1 || !keysAllowed(data, "artifacts") {
		return nil, fail("invalid_state")
	}
	items, ok := data["artifacts"].([]any)
	if !ok || len(items) != len(manifest) {
		return nil, fail("invalid_state")
	}
	result := make([]workerSnapshot, 0, len(items))
	for index, item := range items {
		entry, ok := item.(Object)
		present, presentOK := entry["present"].(bool)
		if !ok || len(entry) != 3 || !keysAllowed(entry, "name", "present", "sha256") || !presentOK || s(entry, "name") != manifest[index].Name {
			return nil, fail("invalid_state")
		}
		old := workerSnapshot{Name: s(entry, "name"), Present: present}
		if present {
			digest := s(entry, "sha256")
			if !shaPattern.MatchString(digest) {
				return nil, fail("invalid_state")
			}
			old.SHA256 = &digest
		} else if entry["sha256"] != nil {
			return nil, fail("invalid_state")
		}
		result = append(result, old)
	}
	return result, nil
}

func workerListing(data []byte) map[string]bool {
	result := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		result[strings.TrimSuffix(line, "\r")] = true
	}
	return result
}

func (r *Runtime) workerPublish(id, checkout, target string, manifest []workerArtifact, env map[string]string) error {
	r.CurrentStep = "publish"
	remoteEnv, err := workerRemoteEnvironment(env)
	if err != nil {
		return err
	}
	bucket := env["SEAWEEDFS_CLOUD_IMAGES_BUCKET"]
	if bucket == "" {
		bucket = "cloud-images"
	}
	remote := "seaweedfs:" + bucket
	backup := filepath.Join(r.JobDir(id), "publication.json")
	if _, err := os.Lstat(backup); errors.Is(err, os.ErrNotExist) {
		listing, err := r.Invoke([]string{"rclone", "lsf", "--files-only", "--max-depth", "1", remote}, CommandOptions{Env: remoteEnv, Capture: true})
		if err != nil {
			return err
		}
		existing := workerListing(listing.Output)
		old := make([]workerSnapshot, 0, len(manifest))
		for _, artifact := range manifest {
			present := existing[artifact.Name]
			if present != existing[artifact.Name+".sha256"] {
				return fail("remote_pair_incomplete")
			}
			entry := workerSnapshot{Name: artifact.Name, Present: present}
			if present {
				digest, err := r.workerRemoteDigest(remote+"/"+artifact.Name, remoteEnv)
				if err != nil {
					return err
				}
				sidecar, err := r.Invoke([]string{"rclone", "cat", remote + "/" + artifact.Name + ".sha256"}, CommandOptions{Env: remoteEnv, Capture: true})
				if err != nil {
					return err
				}
				if strings.TrimSpace(string(sidecar.Output)) != digest {
					return fail("remote_checksum_mismatch")
				}
				entry.SHA256 = &digest
				for _, suffix := range []string{"", ".sha256"} {
					if _, err := r.Invoke([]string{"rclone", "copyto", remote + "/" + artifact.Name + suffix,
						remote + "/.imagebuilder-backups/" + id + "/" + artifact.Name + suffix}, CommandOptions{Env: remoteEnv, Timeout: workerSeconds(r.Cfg.TimeoutSeconds)}); err != nil {
						return err
					}
				}
			}
			old = append(old, entry)
		}
		if err := writeJSON(backup, Object{"artifacts": old}, 0600); err != nil {
			return err
		}
	} else if err != nil {
		return fail("invalid_state")
	}
	snapshot, err := workerPublicationSnapshot(backup, manifest)
	if err != nil {
		return err
	}
	if err := r.workerUploadAndVerify(checkout, target, manifest, remote, env, remoteEnv); err == nil {
		return nil
	}
	r.CurrentStep = "rollback"
	if err := r.workerRollback(id, remote, snapshot, remoteEnv); err != nil {
		return fail("publication_blocked")
	}
	return fail("publication_failed_restored")
}

func (r *Runtime) workerUploadAndVerify(checkout, target string, manifest []workerArtifact, remote string, env, remoteEnv map[string]string) error {
	if _, err := r.Invoke([]string{"bash", "./push.sh", target}, CommandOptions{Dir: filepath.Join(checkout, "packer"), Env: env, Timeout: workerSeconds(r.Cfg.TimeoutSeconds)}); err != nil {
		return err
	}
	for _, artifact := range manifest {
		digest, err := r.workerRemoteDigest(remote+"/"+artifact.Name, remoteEnv)
		if err != nil {
			return err
		}
		if digest != artifact.SHA256 {
			return fail("remote_checksum_mismatch")
		}
		sidecar, err := r.Invoke([]string{"rclone", "cat", remote + "/" + artifact.Name + ".sha256"}, CommandOptions{Env: remoteEnv, Capture: true})
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(sidecar.Output)) != artifact.SHA256 {
			return fail("remote_checksum_mismatch")
		}
	}
	return nil
}

func (r *Runtime) workerRollback(id, remote string, snapshot []workerSnapshot, env map[string]string) error {
	for _, artifact := range snapshot {
		for _, suffix := range []string{"", ".sha256"} {
			if artifact.Present {
				if _, err := r.Invoke([]string{"rclone", "copyto", remote + "/.imagebuilder-backups/" + id + "/" + artifact.Name + suffix,
					remote + "/" + artifact.Name + suffix}, CommandOptions{Env: env, Timeout: workerSeconds(r.Cfg.TimeoutSeconds)}); err != nil {
					return err
				}
			} else {
				_, _ = r.Invoke([]string{"rclone", "deletefile", remote + "/" + artifact.Name + suffix}, CommandOptions{Env: env, Timeout: workerSeconds(r.Cfg.TimeoutSeconds)})
			}
		}
		if artifact.Present {
			digest, err := r.workerRemoteDigest(remote+"/"+artifact.Name, env)
			if err != nil {
				return err
			}
			if artifact.SHA256 == nil || digest != *artifact.SHA256 {
				return fail("rollback_failed")
			}
			sidecar, err := r.Invoke([]string{"rclone", "cat", remote + "/" + artifact.Name + ".sha256"}, CommandOptions{Env: env, Capture: true})
			if err != nil {
				return err
			}
			if strings.TrimSpace(string(sidecar.Output)) != *artifact.SHA256 {
				return fail("rollback_failed")
			}
		} else {
			listing, err := r.Invoke([]string{"rclone", "lsf", "--files-only", "--max-depth", "1", remote}, CommandOptions{Env: env, Capture: true})
			if err != nil {
				return err
			}
			remaining := workerListing(listing.Output)
			if remaining[artifact.Name] || remaining[artifact.Name+".sha256"] {
				return fail("rollback_failed")
			}
		}
	}
	return nil
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
		if err := r.workerPublish(id, checkout, s(request, "target"), manifest, publishEnv); err != nil {
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
