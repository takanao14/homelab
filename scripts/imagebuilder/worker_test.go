package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const workerTestID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const workerTestRevision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const workerTestImage = "ubuntu-26.04-base.img"

func workerTestRuntime(t *testing.T, publish bool) *Runtime {
	t.Helper()
	account, err := user.Current()
	if err != nil {
		t.Fatal("test account unavailable")
	}
	root := t.TempDir()
	r, err := NewRuntime(Config{RepoURL: "https://github.com/takanao14/homelab.git", WorkDir: filepath.Join(root, "work"),
		RuntimeDir: filepath.Join(root, "run"), User: account.Username, MinimumFreeBytes: 1, CacheMaxBytes: 1024,
		IdleSeconds: 600, TimeoutSeconds: 60, RetentionDays: 7, SmokeTimeoutSeconds: 30})
	if err != nil {
		t.Fatalf("test runtime unavailable: %s", safeCode(err))
	}
	r.Chown = func(string, int, int) error { return nil }
	if err := r.Initialize(); err != nil {
		t.Fatalf("test state initialization failed: %s", safeCode(err))
	}
	for _, directory := range []string{r.JobDir(workerTestID), r.WorkerPath(workerTestID), filepath.Dir(r.RequestPath(workerTestID))} {
		if os.MkdirAll(directory, 0700) != nil {
			t.Fatal("test job setup failed")
		}
	}
	if writeJSON(r.RequestPath(workerTestID), workerTestRequest(publish, "submit"), 0600) != nil {
		t.Fatal("test request setup failed")
	}
	return r
}

func workerTestRequest(publish bool, operation string) Object {
	values := Object{}
	if operation == "submit" {
		values["PKR_VAR_user_password"] = "WORKER_SECRET_MARKER"
	}
	if publish {
		values["SEAWEEDFS_S3_ENDPOINT"] = "https://s3.example.test"
		values["SEAWEEDFS_S3_ACCESS_KEY"] = "WORKER_SECRET_MARKER"
		values["SEAWEEDFS_S3_SECRET_KEY"] = "WORKER_SECRET_MARKER"
	}
	return Object{"op": operation, "revision": workerTestRevision, "target": "ubuntu26-base", "publish": publish, "credentials": values}
}

func workerTestHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func workerTestArtifact(t *testing.T, r *Runtime, contents []byte) {
	t.Helper()
	directory := filepath.Join(r.JobDir(workerTestID), "repo", "packer", "images")
	if os.MkdirAll(directory, 0700) != nil || os.WriteFile(filepath.Join(directory, workerTestImage), contents, 0600) != nil ||
		os.WriteFile(filepath.Join(directory, workerTestImage+".sha256"), []byte(workerTestHash(contents)+"\n"), 0600) != nil {
		t.Fatal("test artifact setup failed")
	}
}

type workerTestExecutor struct {
	t             *testing.T
	r             *Runtime
	remote        map[string][]byte
	builds        int
	smokes        int
	pushes        int
	copies        int
	failBuild     bool
	failPush      string
	refuseDeletes bool
}

func workerTestExecutorFor(t *testing.T, r *Runtime) *workerTestExecutor {
	t.Helper()
	x := &workerTestExecutor{t: t, r: r, remote: map[string][]byte{}}
	r.Exec = x.command
	r.SmokeHook = func(id, image string, env map[string]string) error {
		if id != workerTestID || filepath.Base(image) != workerTestImage || env["PKR_VAR_ssh_pubkey"] == "" {
			t.Fatal("unexpected smoke request")
		}
		x.assertNoSecrets(env)
		x.smokes++
		return nil
	}
	return x
}

func (x *workerTestExecutor) assertNoSecrets(env map[string]string) {
	x.t.Helper()
	for key := range credentialSet {
		if _, exists := env[key]; exists {
			x.t.Fatal("unnecessary credentials passed to child")
		}
	}
}

func (x *workerTestExecutor) command(args []string, options CommandOptions) (CommandResult, error) {
	if len(args) == 0 {
		return CommandResult{}, fail("command_failed")
	}
	switch args[0] {
	case "git":
		x.assertNoSecrets(options.Env)
		if len(args) > 3 && args[3] == "checkout" {
			variables := filepath.Join(x.r.JobDir(workerTestID), "repo", "packer", "vars")
			if os.MkdirAll(variables, 0700) != nil || os.WriteFile(filepath.Join(variables, "ubuntu26-base.pkrvars.hcl"), nil, 0600) != nil {
				x.t.Fatal("test checkout failed")
			}
		}
		if len(args) > 3 && args[3] == "rev-parse" {
			return CommandResult{Output: []byte(workerTestRevision + "\n")}, nil
		}
	case "ssh-keygen":
		x.assertNoSecrets(options.Env)
		if os.WriteFile(args[len(args)-1]+".pub", []byte("ssh-ed25519 AAAA worker\n"), 0600) != nil {
			x.t.Fatal("test public key failed")
		}
	case "ssh-agent":
		x.assertNoSecrets(options.Env)
		return CommandResult{Output: []byte("SSH_AUTH_SOCK=/tmp/test-agent; export SSH_AUTH_SOCK;\nSSH_AGENT_PID=999999999; export SSH_AGENT_PID;\n")}, nil
	case "ssh-add":
		x.assertNoSecrets(options.Env)
	case "qemu-img":
		return CommandResult{}, nil
	case "bash":
		if args[1] == "./build.sh" {
			if options.Env["PKR_VAR_user_password"] == "" || options.Env["SEAWEEDFS_S3_SECRET_KEY"] != "" || options.Env["IMAGEBUILDER_LOG_EVENTS"] != "1" {
				x.t.Fatal("build environment boundary failed")
			}
			x.builds++
			if x.failBuild {
				return CommandResult{}, errors.New("WORKER_SECRET_MARKER")
			}
			workerTestArtifact(x.t, x.r, []byte("image-bytes"))
		} else if args[1] == "./push.sh" {
			if options.Env["PKR_VAR_user_password"] != "" || options.Env["SEAWEEDFS_S3_SECRET_KEY"] == "" {
				x.t.Fatal("publication environment boundary failed")
			}
			x.pushes++
			if x.failPush == "before" {
				return CommandResult{}, fail("command_failed")
			}
			x.remote["seaweedfs:cloud-images/"+workerTestImage] = []byte("image-bytes")
			if x.failPush == "partial" {
				return CommandResult{}, fail("command_failed")
			}
			x.remote["seaweedfs:cloud-images/"+workerTestImage+".sha256"] = []byte(workerTestHash([]byte("image-bytes")) + "\n")
		}
	case "rclone":
		if len(args) < 3 {
			return CommandResult{}, fail("command_failed")
		}
		switch args[1] {
		case "lsf":
			prefix := args[len(args)-1] + "/"
			names := []string{}
			for key := range x.remote {
				if strings.HasPrefix(key, prefix) {
					name := strings.TrimPrefix(key, prefix)
					if !strings.Contains(name, "/") {
						names = append(names, name)
					}
				}
			}
			sort.Strings(names)
			return CommandResult{Output: []byte(strings.Join(names, "\n"))}, nil
		case "cat":
			data, exists := x.remote[args[2]]
			if !exists {
				return CommandResult{}, fail("command_failed")
			}
			if options.Digest {
				return CommandResult{Digest: workerTestHash(data)}, nil
			}
			return CommandResult{Output: append([]byte(nil), data...)}, nil
		case "copyto":
			data, exists := x.remote[args[2]]
			if !exists {
				return CommandResult{}, fail("command_failed")
			}
			x.remote[args[3]] = append([]byte(nil), data...)
			x.copies++
		case "deletefile":
			if x.refuseDeletes {
				return CommandResult{}, fail("command_failed")
			}
			if _, exists := x.remote[args[2]]; !exists {
				return CommandResult{}, fail("command_failed")
			}
			delete(x.remote, args[2])
		}
	}
	return CommandResult{}, nil
}

func TestWorkerRunBuildVerifiesWithoutRootStateOrUnnecessaryCredentials(t *testing.T) {
	r := workerTestRuntime(t, false)
	x := workerTestExecutorFor(t, r)
	if os.Rename(r.controlDir(), filepath.Join(r.Cfg.WorkDir, "control-unavailable")) != nil {
		t.Fatal("test control isolation failed")
	}
	if err := r.Run(workerTestID); err != nil {
		t.Fatalf("worker build failed: %s", safeCode(err))
	}
	progress, err := r.Progress(workerTestID)
	if err != nil || s(progress, "code") != "verified" || s(progress, "sha256") != workerTestHash([]byte("image-bytes")) {
		t.Fatal("verified build result missing")
	}
	if x.builds != 1 || x.smokes != 1 || x.pushes != 0 || r.CurrentJob != "" || r.CurrentStep != "" {
		t.Fatal("unexpected worker lifecycle")
	}
}

func TestWorkerRunFailureIsValueFreeAndNeverPublishes(t *testing.T) {
	r := workerTestRuntime(t, true)
	x := workerTestExecutorFor(t, r)
	x.failBuild = true
	err := r.Run(workerTestID)
	if safeCode(err) != "internal_error" || err.Error() != "internal_error" || x.pushes != 0 || x.smokes != 0 {
		t.Fatal("worker failure leaked data or published")
	}
	progress, err := r.Progress(workerTestID)
	if err != nil || s(progress, "code") != "internal_error" {
		t.Fatal("safe failure state missing")
	}
	data, err := os.ReadFile(r.ProgressPath(workerTestID))
	if err != nil || strings.Contains(string(data), "WORKER_SECRET_MARKER") {
		t.Fatal("failure state was not sanitized")
	}
}

func TestWorkerRetryReusesVerifiedArtifactWithoutCheckoutOrSmoke(t *testing.T) {
	r := workerTestRuntime(t, true)
	x := workerTestExecutorFor(t, r)
	if err := r.Run(workerTestID); err != nil {
		t.Fatalf("initial publication failed: %s", safeCode(err))
	}
	if writeJSON(r.RequestPath(workerTestID), workerTestRequest(true, "publish-retry"), 0600) != nil {
		t.Fatal("retry request failed")
	}
	if err := r.Run(workerTestID); err != nil {
		t.Fatalf("publication retry failed: %s", safeCode(err))
	}
	if x.builds != 1 || x.smokes != 1 || x.pushes != 2 {
		t.Fatal("retry rebuilt or repeated smoke")
	}
}

func TestWorkerRetryRejectsChangedButSelfConsistentArtifact(t *testing.T) {
	r := workerTestRuntime(t, false)
	x := workerTestExecutorFor(t, r)
	if err := r.Run(workerTestID); err != nil {
		t.Fatalf("initial build failed: %s", safeCode(err))
	}
	workerTestArtifact(t, r, []byte("changed-image"))
	if writeJSON(r.RequestPath(workerTestID), workerTestRequest(true, "publish-retry"), 0600) != nil {
		t.Fatal("retry request failed")
	}
	if err := r.Run(workerTestID); safeCode(err) != "artifact_changed" || x.pushes != 0 {
		t.Fatal("changed artifact was accepted")
	}
}

func TestWorkerPublicationFailureRestoresVerifiedOldPair(t *testing.T) {
	r := workerTestRuntime(t, true)
	x := workerTestExecutorFor(t, r)
	old := []byte("old-image")
	x.remote["seaweedfs:cloud-images/"+workerTestImage] = old
	x.remote["seaweedfs:cloud-images/"+workerTestImage+".sha256"] = []byte(workerTestHash(old) + "\n")
	x.failPush = "partial"
	if err := r.Run(workerTestID); safeCode(err) != "publication_failed_restored" {
		t.Fatal("failed publication did not restore old pair")
	}
	if !reflect.DeepEqual(x.remote["seaweedfs:cloud-images/"+workerTestImage], old) ||
		string(x.remote["seaweedfs:cloud-images/"+workerTestImage+".sha256"]) != workerTestHash(old)+"\n" || x.copies != 4 {
		t.Fatal("restored publication pair is inconsistent")
	}
	if _, exists := x.remote["seaweedfs:cloud-images/.imagebuilder-backups/"+workerTestID+"/"+workerTestImage]; !exists {
		t.Fatal("recovery image missing")
	}
}

func TestWorkerAbsentPairRollbackAcceptsAlreadyMissingObjects(t *testing.T) {
	r := workerTestRuntime(t, true)
	x := workerTestExecutorFor(t, r)
	x.failPush = "before"
	x.refuseDeletes = true
	if err := r.Run(workerTestID); safeCode(err) != "publication_failed_restored" || len(x.remote) != 0 {
		t.Fatal("already absent pair was not recognized as restored")
	}
}

func TestWorkerFailedRollbackBlocksPublication(t *testing.T) {
	r := workerTestRuntime(t, true)
	x := workerTestExecutorFor(t, r)
	x.failPush, x.refuseDeletes = "partial", true
	if err := r.Run(workerTestID); safeCode(err) != "publication_blocked" {
		t.Fatal("incomplete rollback was accepted")
	}
	progress, err := r.Progress(workerTestID)
	if err != nil || s(progress, "code") != "publication_blocked" {
		t.Fatal("blocked publication state missing")
	}
}

func TestWorkerPublicationSnapshotRejectsUnrelatedObjectsBeforeCommands(t *testing.T) {
	r := workerTestRuntime(t, true)
	if writeJSON(filepath.Join(r.JobDir(workerTestID), "publication.json"), Object{"artifacts": []Object{
		{"name": "../unrelated.img", "present": false, "sha256": nil},
	}}, 0600) != nil {
		t.Fatal("snapshot setup failed")
	}
	called := false
	r.Exec = func([]string, CommandOptions) (CommandResult, error) { called = true; return CommandResult{}, nil }
	err := r.workerPublish(workerTestID, filepath.Join(r.JobDir(workerTestID), "repo"), "ubuntu26-base",
		[]workerArtifact{{Name: workerTestImage, SHA256: workerTestHash([]byte("image"))}}, map[string]string{
			"SEAWEEDFS_S3_ENDPOINT": "https://s3.example.test", "SEAWEEDFS_S3_ACCESS_KEY": "WORKER_SECRET_MARKER", "SEAWEEDFS_S3_SECRET_KEY": "WORKER_SECRET_MARKER"})
	if safeCode(err) != "invalid_state" || called {
		t.Fatal("unrelated snapshot objects were accepted")
	}
}

func TestWorkerTargetsComeFromCheckoutMetadata(t *testing.T) {
	root := t.TempDir()
	if os.MkdirAll(filepath.Join(root, "packer", "vars"), 0700) != nil ||
		os.WriteFile(filepath.Join(root, "packer", "vars", "example-base.pkrvars.hcl"), nil, 0600) != nil ||
		os.WriteFile(filepath.Join(root, "packer", "import-upstream.sh"), []byte("import_target() {\n case \"$1\" in\n freebsd151)\n ;;\n esac\n}\n"), 0600) != nil {
		t.Fatal("target fixtures unavailable")
	}
	for target, script := range map[string]string{"example-base": "./build.sh", "all": "./build.sh", "freebsd151": "./import-upstream.sh"} {
		args, err := workerTargetCommand(root, target)
		if err != nil || len(args) != 4 || args[1] != script || args[3] != target {
			t.Fatal("checkout target resolution failed")
		}
	}
	for _, target := range []string{"unknown", "x;id", "../base"} {
		if _, err := workerTargetCommand(root, target); err == nil {
			t.Fatal("invalid target accepted")
		}
	}
}

func TestWorkerSmokeConfigurationMatchesLinuxAndNativeFreeBSD(t *testing.T) {
	for _, image := range []string{workerTestImage, "freebsd-15.1-cloudinit-ufs.img"} {
		configuration, check, err := workerSmokeConfiguration(workerTestID, image, "ssh-ed25519 AAAA worker")
		if err != nil || b(configuration, "ssh_pwauth") {
			t.Fatal("smoke configuration failed")
		}
		users, ok := configuration["users"].([]Object)
		if !ok || len(users) != 1 {
			t.Fatal("smoke account missing")
		}
		if strings.HasPrefix(image, "freebsd-") {
			if _, exists := users[0]["sudo"]; exists || strings.Contains(check, "sudo") || strings.Contains(check, "cloud-init") {
				t.Fatal("FreeBSD smoke requires unavailable Linux tooling")
			}
		} else if !strings.Contains(check, "sudo -n cloud-init status --wait") || !strings.Contains(check, "qemu-guest-agent") {
			t.Fatal("Linux smoke does not verify cloud-init and guest agent")
		}
		if check == "" || !strings.Contains(check, "/var/tmp/imagebuilder-smoke-"+workerTestID) {
			t.Fatal("smoke completion marker missing")
		}
	}
}

func TestWorkerArtifactValidationRejectsSymlinksAndWrongChecksums(t *testing.T) {
	r := workerTestRuntime(t, false)
	r.Exec = func([]string, CommandOptions) (CommandResult, error) { return CommandResult{}, nil }
	workerTestArtifact(t, r, []byte("image"))
	image := filepath.Join(r.JobDir(workerTestID), "repo", "packer", "images", workerTestImage)
	if os.WriteFile(image+".sha256", []byte(strings.Repeat("a", 64)), 0600) != nil {
		t.Fatal("checksum fixture failed")
	}
	if _, err := r.workerArtifacts(filepath.Join(r.JobDir(workerTestID), "repo")); safeCode(err) != "checksum_mismatch" {
		t.Fatal("wrong checksum accepted")
	}
	if os.Remove(image) != nil || os.Symlink(image+".sha256", image) != nil {
		t.Fatal("symlink fixture failed")
	}
	if _, err := r.workerArtifacts(filepath.Join(r.JobDir(workerTestID), "repo")); safeCode(err) != "invalid_artifact" {
		t.Fatal("symlink image accepted")
	}
}
