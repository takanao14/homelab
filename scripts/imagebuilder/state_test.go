package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type stateFixture struct {
	r        *Runtime
	mu       sync.Mutex
	commands [][]string
	stamp    time.Time
}

func newStateFixture(t *testing.T) *stateFixture {
	t.Helper()
	root := t.TempDir()
	f := &stateFixture{stamp: time.Unix(1800000000, 0)}
	f.r = &Runtime{
		Cfg: Config{WorkDir: filepath.Join(root, "work"), RuntimeDir: filepath.Join(root, "run"),
			RepoURL: "https://github.com/takanao14/homelab.git", User: "worker", MinimumFreeBytes: 1,
			IdleSeconds: 600, TimeoutSeconds: 60, RetentionDays: 7, SmokeTimeoutSeconds: 30, CacheMaxBytes: 1024},
		UID: os.Geteuid(), GID: os.Getegid(), UserName: "worker", Home: root,
		Chown:     func(string, int, int) error { return nil },
		ReadyHook: func() (Object, error) { return Object{"ok": true, "phase": "ready"}, nil },
	}
	f.r.Now = func() time.Time { return f.stamp }
	f.r.Exec = func(args []string, _ CommandOptions) (CommandResult, error) {
		f.mu.Lock()
		f.commands = append(f.commands, append([]string{}, args...))
		f.mu.Unlock()
		if len(args) > 1 && args[0] == "systemctl" && args[1] == "show" {
			return CommandResult{Output: []byte("inactive\n")}, nil
		}
		return CommandResult{}, nil
	}
	if err := f.r.Initialize(); err != nil {
		t.Fatal(safeCode(err))
	}
	return f
}

func stateError(t *testing.T, code string, err error) {
	t.Helper()
	if err == nil || err.Error() != code {
		t.Fatalf("expected %s, got %s", code, safeCode(err))
	}
}

func stateOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(safeCode(err))
	}
}

func stateRequest() Object {
	return Object{"op": "submit", "revision": strings.Repeat("a", 40), "target": "ubuntu26-base",
		"credentials": Object{"PKR_VAR_user_password": "SECRET_MARKER"}}
}

func stateSubmit(t *testing.T, r *Runtime, fields Object) string {
	t.Helper()
	request := stateRequest()
	for key, value := range fields {
		request[key] = value
	}
	response, err := r.Control(request)
	stateOK(t, err)
	if !validJobID(s(response, "job_id")) {
		t.Fatal("invalid submitted job identity")
	}
	return s(response, "job_id")
}

func stateComplete(t *testing.T, r *Runtime, id, phase, code string) {
	t.Helper()
	stateOK(t, r.Update(id, phase, code, nil))
	stateOK(t, r.Finish(id))
}

func stateControl(t *testing.T, r *Runtime, change func(Object)) {
	t.Helper()
	_, err := r.locked(func(state Object) (Object, error) { change(state); return nil, r.saveControl(state) })
	stateOK(t, err)
}

func stateRead(t *testing.T, path string) Object {
	t.Helper()
	result, err := readJSON(path)
	stateOK(t, err)
	return result
}

func TestStateSecretsUseRuntimeDirectoryAndAreRemoved(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, nil)
	request, err := os.ReadFile(f.r.RequestPath(id))
	stateOK(t, err)
	if !bytes.Contains(request, []byte("SECRET_MARKER")) {
		t.Fatal("request credentials missing")
	}
	info, err := os.Stat(f.r.RequestPath(id))
	stateOK(t, err)
	if info.Mode().Perm() != 0640 {
		t.Fatal("request permissions changed")
	}
	info, err = os.Stat(filepath.Dir(f.r.RequestPath(id)))
	stateOK(t, err)
	if info.Mode().Perm() != 0750 {
		t.Fatal("worker cannot traverse request directory")
	}
	record, err := os.ReadFile(f.r.RecordPath(id))
	stateOK(t, err)
	if bytes.Contains(record, []byte("SECRET_MARKER")) {
		t.Fatal("secret escaped runtime directory")
	}
	stateComplete(t, f.r, id, "succeeded", "verified")
	if exists(filepath.Dir(f.r.RequestPath(id))) || exists(f.r.WorkerPath(id)) {
		t.Fatal("runtime secrets remain")
	}
	response, err := f.r.Control(Object{"op": "status"})
	stateOK(t, err)
	if s(response, "phase") != "succeeded" || !b(response, "finished") {
		t.Fatal("missing finalized result")
	}
}

func TestStateJobsParentIsRootOwnedAndSubmittedChildrenAreWorkerOwned(t *testing.T) {
	f := newStateFixture(t)
	owners := map[string][2]int{}
	f.r.Chown = func(path string, uid, gid int) error {
		owners[path] = [2]int{uid, gid}
		return nil
	}
	stateOK(t, f.r.Initialize())
	info, err := os.Stat(f.r.jobsDir())
	stateOK(t, err)
	if owners[f.r.jobsDir()] != [2]int{0, f.r.GID} || info.Mode().Perm() != 0750 {
		t.Fatal("worker must not be able to replace job directories in the parent")
	}
	id := stateSubmit(t, f.r, nil)
	info, err = os.Stat(f.r.JobDir(id))
	stateOK(t, err)
	if owners[f.r.JobDir(id)] != [2]int{f.r.UID, f.r.GID} || info.Mode().Perm() != 0700 {
		t.Fatal("submitted job directory must remain private to the worker")
	}
	info, err = os.Stat(f.r.jobsDir())
	stateOK(t, err)
	if owners[f.r.jobsDir()] != [2]int{0, f.r.GID} || info.Mode().Perm() != 0750 {
		t.Fatal("submission changed parent directory ownership or permissions")
	}
}

func TestStateConcurrentAdmissionHasOneWinner(t *testing.T) {
	f := newStateFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, err := f.r.Control(stateRequest()); results <- err }()
	}
	close(start)
	winners, busy := 0, 0
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				winners++
			} else if err.Error() == "busy" {
				busy++
			} else {
				t.Fatal(safeCode(err))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("admission deadlocked")
		}
	}
	if winners != 1 || busy != 1 || len(f.commands) != 1 {
		t.Fatal("admission was not serialized")
	}
}

func TestStateShutdownAndAdmissionUseSameLock(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, nil)
	_, err := f.r.Control(Object{"op": "shutdown"})
	stateError(t, "busy", err)
	stateComplete(t, f.r, id, "succeeded", "verified")
	result, err := f.r.Control(Object{"op": "shutdown"})
	stateOK(t, err)
	if s(result, "phase") != "stopping" {
		t.Fatal("shutdown state missing")
	}
	_, err = f.r.Control(stateRequest())
	stateError(t, "stopping", err)
	if !reflect.DeepEqual(f.commands[len(f.commands)-1], []string{"systemctl", "poweroff", "--no-block"}) {
		t.Fatal("unexpected power command")
	}
}

func TestStateStartFailureReleasesActiveAndCredentials(t *testing.T) {
	f := newStateFixture(t)
	f.r.Exec = func([]string, CommandOptions) (CommandResult, error) { return CommandResult{}, fail("command_failed") }
	_, err := f.r.Control(stateRequest())
	stateError(t, "start_failed", err)
	if stateRead(t, f.r.ControlPath())["active"] != nil {
		t.Fatal("start failure left active job")
	}
	entries, err := os.ReadDir(filepath.Join(f.r.Cfg.RuntimeDir, "jobs"))
	stateOK(t, err)
	if len(entries) != 0 {
		t.Fatal("start failure retained credentials")
	}
	result, err := f.r.Control(Object{"op": "status"})
	stateOK(t, err)
	if s(result, "code") != "start_failed" {
		t.Fatal("start failure not retained")
	}
}

func TestStateShutdownFailureCanRetry(t *testing.T) {
	f := newStateFixture(t)
	f.r.Exec = func([]string, CommandOptions) (CommandResult, error) { return CommandResult{}, fail("command_timeout") }
	_, err := f.r.Control(Object{"op": "shutdown"})
	stateError(t, "shutdown_failed", err)
	if b(stateRead(t, f.r.ControlPath()), "stopping") {
		t.Fatal("failed shutdown retained stopping state")
	}
}

func TestStateIdleGraceAndMaintenancePersistAcrossBoot(t *testing.T) {
	f := newStateFixture(t)
	stateControl(t, f.r, func(state Object) { state["idle_since"] = int64(0) })
	_, err := f.r.Control(Object{"op": "maintenance", "enabled": true})
	stateOK(t, err)
	stateOK(t, f.r.Idle())
	if len(f.commands) != 0 {
		t.Fatal("maintenance stopped the VM")
	}
	stateOK(t, os.Remove(f.r.ControlPath()))
	stateOK(t, f.r.Boot())
	result, err := f.r.Control(Object{"op": "status"})
	stateOK(t, err)
	if !b(result, "maintenance") {
		t.Fatal("maintenance lost on reboot")
	}
	_, err = f.r.Control(stateRequest())
	stateError(t, "maintenance", err)
	_, err = f.r.Control(Object{"op": "maintenance", "enabled": false})
	stateOK(t, err)
	stateOK(t, os.Remove(f.r.ControlPath()))
	stateOK(t, f.r.Boot())
	if b(stateRead(t, f.r.ControlPath()), "maintenance") {
		t.Fatal("disabled maintenance resurrected")
	}
	stateControl(t, f.r, func(state Object) { state["idle_since"] = int64(0) })
	stateOK(t, f.r.Idle())
	if len(f.commands) != 1 || f.commands[0][1] != "poweroff" {
		t.Fatal("idle VM did not stop")
	}
}

func TestStateReadyReservesGraceAndDiagnosticsSkipBuildChecks(t *testing.T) {
	f := newStateFixture(t)
	stateControl(t, f.r, func(state Object) { state["idle_since"] = int64(0) })
	_, err := f.r.Control(Object{"op": "ready"})
	stateOK(t, err)
	stateOK(t, f.r.Idle())
	if len(f.commands) != 0 {
		t.Fatal("readiness did not reserve submission grace")
	}
	f.r.ReadyHook = func() (Object, error) { return nil, fail("insufficient_space") }
	_, err = f.r.Control(Object{"op": "ready", "build_ready": false})
	stateOK(t, err)
	_, err = f.r.Control(Object{"op": "ready"})
	stateError(t, "insufficient_space", err)
	stateControl(t, f.r, func(state Object) { state["stopping"] = true })
	_, err = f.r.Control(Object{"op": "ready", "build_ready": false})
	stateError(t, "stopping", err)
}

func TestStateBootInterruptsJobsAndDiscardsSecrets(t *testing.T) {
	for _, phase := range []string{"building", "publishing"} {
		t.Run(phase, func(t *testing.T) {
			f := newStateFixture(t)
			id := stateSubmit(t, f.r, nil)
			stateOK(t, f.r.Update(id, phase, phase, nil))
			stateOK(t, os.Remove(f.r.ControlPath()))
			stateOK(t, f.r.Boot())
			result, err := f.r.Control(Object{"op": "status", "job_id": id})
			stateOK(t, err)
			if s(result, "phase") != "interrupted" || s(result, "code") != "host_restarted" {
				t.Fatal("reboot did not interrupt job")
			}
			if exists(filepath.Dir(f.r.RequestPath(id))) || exists(f.r.WorkerPath(id)) {
				t.Fatal("reboot retained runtime secrets")
			}
			if stateRead(t, f.r.ControlPath())["active"] != nil {
				t.Fatal("reboot retained active job")
			}
			if phase == "publishing" {
				if s(stateRead(t, f.r.PublicationGuard()), "job_id") != id {
					t.Fatal("publication guard missing")
				}
				_, err = f.r.Control(stateRequest())
				stateError(t, "publication_blocked", err)
			}
		})
	}
}

func TestStateBootRefusesToDisruptRunningService(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, nil)
	f.r.Exec = func([]string, CommandOptions) (CommandResult, error) {
		return CommandResult{Output: []byte("active\n")}, nil
	}
	stateError(t, "busy", f.r.Boot())
	if s(stateRead(t, f.r.ControlPath()), "active") != id || !exists(f.r.RequestPath(id)) {
		t.Fatal("active service was disrupted")
	}
}

func TestStateTimeoutFinalizationAndInvalidProgressAreSafe(t *testing.T) {
	for _, phase := range []string{"building", "publishing", "invalid"} {
		t.Run(phase, func(t *testing.T) {
			f := newStateFixture(t)
			id := stateSubmit(t, f.r, nil)
			if phase == "invalid" {
				stateOK(t, writeJSON(f.r.ProgressPath(id), Object{"phase": "failed", "code": "SECRET_MARKER"}, 0600))
			} else {
				stateOK(t, f.r.Update(id, phase, phase, nil))
			}
			stateOK(t, f.r.Finish(id))
			result, err := f.r.Control(Object{"op": "status", "job_id": id})
			stateOK(t, err)
			want := map[string]string{"building": "service_failed", "publishing": "publication_blocked", "invalid": "invalid_progress"}[phase]
			if s(result, "code") != want || exists(f.r.WorkerPath(id)) {
				t.Fatal("unsafe finalization result")
			}
			if phase == "publishing" {
				_, err = f.r.Control(stateRequest())
				stateError(t, "publication_blocked", err)
			}
		})
	}
}

func TestStateRejectsUnknownCredentialsAndShellTokens(t *testing.T) {
	for _, fields := range []Object{{"credentials": Object{"PATH": "SECRET_MARKER"}}, {"target": "x;id"}, {"revision": "main"}, {"publish": 1}, {"unexpected": "SECRET_MARKER"}} {
		f := newStateFixture(t)
		request := stateRequest()
		for key, value := range fields {
			request[key] = value
		}
		_, err := f.r.Control(request)
		stateError(t, "invalid_request", err)
		if len(f.commands) != 0 {
			t.Fatal("invalid request started a service")
		}
	}
}

func TestStateBranchResolutionUsesExactRefWithoutLock(t *testing.T) {
	f := newStateFixture(t)
	stateOK(t, os.Remove(f.r.ControlPath()))
	f.r.Exec = func(args []string, opts CommandOptions) (CommandResult, error) {
		want := []string{"runuser", "-u", "worker", "--", "git", "ls-remote", "--exit-code", "--refs", f.r.Cfg.RepoURL, "refs/heads/feature/packer"}
		if !reflect.DeepEqual(args, want) || !opts.Capture {
			t.Fatal("branch resolver command changed")
		}
		return CommandResult{Output: []byte(strings.Repeat("b", 40) + "\trefs/heads/feature/packer\n")}, nil
	}
	result, err := f.r.Control(Object{"op": "resolve", "branch": "feature/packer"})
	stateOK(t, err)
	if s(result, "revision") != strings.Repeat("b", 40) || s(result, "branch") != "feature/packer" {
		t.Fatal("branch resolution lost identity")
	}
}

func TestStateBranchResolutionRejectsUntrustedOutputAndInputs(t *testing.T) {
	f := newStateFixture(t)
	for _, output := range []string{"SECRET_MARKER", strings.Repeat("b", 40) + "\trefs/heads/other\n", strings.Repeat(strings.Repeat("b", 40)+"\trefs/heads/topic\n", 2)} {
		f.r.Exec = func([]string, CommandOptions) (CommandResult, error) {
			return CommandResult{Output: []byte(output)}, nil
		}
		_, err := f.r.Control(Object{"op": "resolve", "branch": "topic"})
		stateError(t, "branch_resolution_failed", err)
	}
	called := false
	f.r.Exec = func([]string, CommandOptions) (CommandResult, error) { called = true; return CommandResult{}, nil }
	for _, branch := range []string{"../main", "a//b", "a/.hidden", "x.lock", "x/y.lock", "x@{1}", "x\nSECRET_MARKER", "-x", "x/", "x..y"} {
		_, err := f.r.Control(Object{"op": "resolve", "branch": branch})
		stateError(t, "invalid_branch", err)
	}
	if called {
		t.Fatal("invalid branch invoked git")
	}
}

func TestStateBranchProvenanceKeepsPinnedRevision(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, Object{"branch": "feature/packer"})
	result, err := f.r.Control(Object{"op": "status", "job_id": id})
	stateOK(t, err)
	if s(result, "revision") != strings.Repeat("a", 40) || s(result, "branch") != "feature/packer" {
		t.Fatal("branch replaced pinned revision")
	}
}

func TestStateRetryReusesVerifiedArtifactsAndCanClearGuard(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, nil)
	stateOK(t, writeJSON(filepath.Join(f.r.JobDir(id), "verified.json"), []any{}, 0600))
	stateComplete(t, f.r, id, "failed", "publication_blocked")
	request := Object{"op": "publish-retry", "job_id": id, "credentials": Object{"SEAWEEDFS_S3_ENDPOINT": "https://s3.example.test", "SEAWEEDFS_S3_ACCESS_KEY": "SECRET_MARKER", "SEAWEEDFS_S3_SECRET_KEY": "SECRET_MARKER"}}
	result, err := f.r.Control(request)
	stateOK(t, err)
	if s(result, "job_id") != id || s(stateRead(t, f.r.RequestPath(id)), "op") != "publish-retry" {
		t.Fatal("retry changed job identity")
	}
	if s(stateRead(t, f.r.ControlPath()), "active") != id {
		t.Fatal("retry did not reserve active job")
	}
	stateComplete(t, f.r, id, "succeeded", "published")
	if exists(f.r.PublicationGuard()) {
		t.Fatal("successful retry retained publication guard")
	}
}

func TestStateRetryStartFailureFinalizesWithoutDeadlock(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, nil)
	stateOK(t, writeJSON(filepath.Join(f.r.JobDir(id), "verified.json"), []any{}, 0600))
	stateComplete(t, f.r, id, "succeeded", "verified")
	f.r.Exec = func([]string, CommandOptions) (CommandResult, error) { return CommandResult{}, fail("command_failed") }
	done := make(chan error, 1)
	go func() {
		_, err := f.r.Control(Object{"op": "publish-retry", "job_id": id, "credentials": Object{"SEAWEEDFS_S3_ENDPOINT": "https://s3.example.test", "SEAWEEDFS_S3_ACCESS_KEY": "SECRET_MARKER", "SEAWEEDFS_S3_SECRET_KEY": "SECRET_MARKER"}})
		done <- err
	}()
	select {
	case err := <-done:
		stateError(t, "start_failed", err)
	case <-time.After(5 * time.Second):
		t.Fatal("retry finalizer deadlocked")
	}
	if stateRead(t, f.r.ControlPath())["active"] != nil || exists(f.r.RequestPath(id)) {
		t.Fatal("failed retry retained active state or secrets")
	}
}

func TestStateRetentionProtectsActiveAndBlockedJobs(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, nil)
	record := stateRead(t, f.r.RecordPath(id))
	record["created"] = int64(0)
	stateOK(t, writeJSON(f.r.RecordPath(id), record, 0600))
	stateOK(t, f.r.Idle())
	if !exists(f.r.RecordPath(id)) {
		t.Fatal("retention removed active record")
	}
	stateComplete(t, f.r, id, "failed", "publication_blocked")
	stateOK(t, f.r.Idle())
	if !exists(f.r.RecordPath(id)) {
		t.Fatal("retention removed blocked record")
	}
	stateOK(t, os.Remove(f.r.PublicationGuard()))
	stateOK(t, f.r.Idle())
	if exists(f.r.RecordPath(id)) || exists(f.r.JobDir(id)) {
		t.Fatal("retention failed to remove expired job")
	}
}

func TestStateCachePruningRunsAsWorkerOnlyWhenIdle(t *testing.T) {
	f := newStateFixture(t)
	cache := filepath.Join(f.r.cacheDir(), "packer")
	stateOK(t, os.Mkdir(cache, 0700))
	id := stateSubmit(t, f.r, nil)
	stateOK(t, f.r.Idle())
	if len(f.commands) != 1 {
		t.Fatal("active build pruned cache")
	}
	stateComplete(t, f.r, id, "succeeded", "verified")
	stateOK(t, f.r.Idle())
	want := []string{"runuser", "-u", "worker", "--", "/usr/local/libexec/imagebuilder", "cache-prune"}
	if !reflect.DeepEqual(f.commands[len(f.commands)-1], want) {
		t.Fatal("cache prune did not drop privileges")
	}
}

func TestStateCacheEvictsOversizeAndExpiredFilesWithoutFollowingLinks(t *testing.T) {
	f := newStateFixture(t)
	cache := filepath.Join(f.r.cacheDir(), "packer")
	stateOK(t, os.Mkdir(cache, 0700))
	stateOK(t, os.WriteFile(filepath.Join(cache, "image"), []byte("cache"), 0600))
	stateOK(t, os.Chtimes(filepath.Join(cache, "image"), f.stamp, f.stamp))
	stateOK(t, f.r.PruneCache())
	if !exists(cache) {
		t.Fatal("small fresh cache was removed")
	}
	f.r.Cfg.CacheMaxBytes = 1
	stateOK(t, f.r.PruneCache())
	if exists(cache) {
		t.Fatal("oversize cache retained")
	}
	stateOK(t, os.Mkdir(cache, 0700))
	stateOK(t, os.WriteFile(filepath.Join(cache, "image"), []byte("a"), 0600))
	old := f.stamp.Add(-8 * 24 * time.Hour)
	stateOK(t, os.Chtimes(filepath.Join(cache, "image"), old, old))
	stateOK(t, f.r.PruneCache())
	if exists(cache) {
		t.Fatal("expired cache retained")
	}
	outside := filepath.Join(t.TempDir(), "retained")
	stateOK(t, os.Mkdir(outside, 0700))
	stateOK(t, os.Symlink(outside, cache))
	stateOK(t, f.r.PruneCache())
	if exists(cache) || !exists(outside) {
		t.Fatal("cache symlink handling crossed boundary")
	}
}

func TestStateInitializeRejectsMissingMountAndSymlinkedControl(t *testing.T) {
	f := newStateFixture(t)
	f.r.Cfg.RequireMount = true
	stateError(t, "work_disk_not_mounted", f.r.Initialize())
	f = newStateFixture(t)
	stateOK(t, os.RemoveAll(f.r.controlDir()))
	stateOK(t, os.Symlink(t.TempDir(), f.r.controlDir()))
	stateError(t, "invalid_state", f.r.Initialize())
}

func TestStateStatusAndLogsDiscardUnknownProgressFields(t *testing.T) {
	f := newStateFixture(t)
	id := stateSubmit(t, f.r, nil)
	stateOK(t, f.r.Update(id, "building", "building", Object{"secret": "SECRET_MARKER"}))
	response, err := f.r.Control(Object{"op": "logs", "job_id": id})
	stateOK(t, err)
	for _, event := range response["events"].([]any) {
		if len(event.(map[string]any)) != 3 {
			t.Fatal("unknown event field escaped")
		}
	}
	if _, ok := response["secret"]; ok {
		t.Fatal("unapproved progress field escaped")
	}
}
