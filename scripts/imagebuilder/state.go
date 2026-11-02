package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func (r *Runtime) controlDir() string          { return filepath.Join(r.Cfg.WorkDir, "control") }
func (r *Runtime) recordsDir() string          { return filepath.Join(r.controlDir(), "jobs") }
func (r *Runtime) jobsDir() string             { return filepath.Join(r.Cfg.WorkDir, "jobs") }
func (r *Runtime) cacheDir() string            { return filepath.Join(r.Cfg.WorkDir, "cache") }
func (r *Runtime) ControlPath() string         { return filepath.Join(r.Cfg.RuntimeDir, "control.json") }
func (r *Runtime) RecordPath(id string) string { return filepath.Join(r.recordsDir(), id+".json") }
func (r *Runtime) RequestPath(id string) string {
	return filepath.Join(r.Cfg.RuntimeDir, "jobs", id, "request.json")
}
func (r *Runtime) WorkerPath(id string) string   { return filepath.Join(r.Cfg.RuntimeDir, "worker", id) }
func (r *Runtime) JobDir(id string) string       { return filepath.Join(r.jobsDir(), id) }
func (r *Runtime) ProgressPath(id string) string { return filepath.Join(r.JobDir(id), "progress.json") }
func (r *Runtime) DiagnosticsPath(id string) string {
	return filepath.Join(r.JobDir(id), "diagnostics.json")
}
func (r *Runtime) PublicationGuard() string {
	return filepath.Join(r.controlDir(), "publication-blocked.json")
}
func (r *Runtime) maintenancePath() string { return filepath.Join(r.controlDir(), "maintenance.json") }

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func mounted(path string) bool {
	path = filepath.Clean(path)
	var current, parent syscall.Stat_t
	if syscall.Stat(path, &current) != nil || syscall.Stat(filepath.Dir(path), &parent) != nil {
		return false
	}
	return current.Dev != parent.Dev || current.Ino == parent.Ino
}

func (r *Runtime) Initialize() error {
	if r.Cfg.RequireMount && !mounted(r.Cfg.WorkDir) {
		return fail("work_disk_not_mounted")
	}
	for _, entry := range []struct {
		path string
		uid  int
		mode os.FileMode
	}{
		{r.controlDir(), 0, 0700}, {r.recordsDir(), 0, 0700},
		{r.jobsDir(), 0, 0750}, {r.cacheDir(), r.UID, 0700},
		{r.Cfg.RuntimeDir, 0, 0750},
		{filepath.Join(r.Cfg.RuntimeDir, "jobs"), 0, 0750},
		{filepath.Join(r.Cfg.RuntimeDir, "worker"), 0, 0750},
	} {
		if info, err := os.Lstat(entry.path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fail("invalid_state")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail("invalid_state")
		}
		if err := os.MkdirAll(entry.path, entry.mode); err != nil {
			return fail("invalid_state")
		}
		if err := r.chown(entry.path, entry.uid, r.GID); err != nil {
			return fail("invalid_state")
		}
		if err := os.Chmod(entry.path, entry.mode); err != nil {
			return fail("invalid_state")
		}
	}
	if !exists(r.ControlPath()) {
		return r.saveControl(Object{"active": nil, "maintenance": false, "stopping": false, "idle_since": r.now().Unix()})
	}
	return nil
}

func (r *Runtime) locked(callback func(Object) (Object, error)) (Object, error) {
	fd, err := syscall.Open(filepath.Join(r.Cfg.RuntimeDir, "control.lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fail("invalid_state")
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return nil, fail("invalid_state")
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	state, err := readJSON(r.ControlPath())
	if err != nil {
		return nil, fail("invalid_state")
	}
	if active := state["active"]; active != nil && !validJobID(s(state, "active")) {
		return nil, fail("invalid_state")
	}
	if _, ok := state["maintenance"].(bool); !ok {
		return nil, fail("invalid_state")
	}
	if _, ok := state["stopping"].(bool); !ok {
		return nil, fail("invalid_state")
	}
	if _, ok := num(state["idle_since"]); !ok {
		return nil, fail("invalid_state")
	}
	return callback(state)
}

func (r *Runtime) saveControl(state Object) error { return writeJSON(r.ControlPath(), state, 0600) }

func (r *Runtime) Progress(id string) (Object, error) {
	if !validJobID(id) {
		return nil, fail("invalid_request")
	}
	if !exists(r.ProgressPath(id)) {
		return Object{"phase": "preparing", "code": "accepted", "events": []any{}}, nil
	}
	data, err := readJSON(r.ProgressPath(id))
	if err != nil {
		return nil, fail("invalid_state")
	}
	return safeProgress(data)
}

func safeProgress(data Object) (Object, error) {
	if !phaseSet[s(data, "phase")] || !codeSet[s(data, "code")] {
		return nil, fail("invalid_state")
	}
	result := Object{"phase": s(data, "phase"), "code": s(data, "code"), "events": []any{}}
	if digest, ok := data["sha256"]; ok {
		value, ok := digest.(string)
		if !ok || !shaPattern.MatchString(value) {
			return nil, fail("invalid_state")
		}
		result["sha256"] = value
	}
	if value, ok := data["events"]; ok {
		events, ok := value.([]any)
		if !ok || len(events) > 100 {
			return nil, fail("invalid_state")
		}
		approved := []any{}
		for _, value := range events {
			event, ok := value.(map[string]any)
			if !ok || !phaseSet[s(event, "phase")] || !codeSet[s(event, "code")] {
				return nil, fail("invalid_state")
			}
			stamp, ok := num(event["time"])
			if !ok {
				return nil, fail("invalid_state")
			}
			approved = append(approved, Object{"phase": s(event, "phase"), "code": s(event, "code"), "time": stamp})
		}
		result["events"] = approved
	}
	return result, nil
}

func (r *Runtime) Update(id, phase, code string, fields Object) error {
	if !phaseSet[phase] || !codeSet[code] {
		return fail("invalid_state")
	}
	old, err := r.Progress(id)
	if err != nil {
		return err
	}
	events := append(old["events"].([]any), Object{"phase": phase, "code": code, "time": r.now().Unix()})
	if len(events) > 100 {
		events = events[len(events)-100:]
	}
	data := Object{"phase": phase, "code": code, "events": events}
	for key, value := range fields {
		data[key] = value
	}
	return writeJSON(r.ProgressPath(id), data, 0600)
}

func (r *Runtime) safeJob(id string) (Object, error) {
	if !validJobID(id) {
		return nil, fail("invalid_request")
	}
	record, err := readJSON(r.RecordPath(id))
	if err != nil {
		return nil, fail("invalid_state")
	}
	finished, ok := record["finished"].(bool)
	if !ok {
		return nil, fail("invalid_state")
	}
	var progress Object
	if finished {
		value, ok := record["result"].(map[string]any)
		if !ok {
			return nil, fail("invalid_state")
		}
		progress, err = safeProgress(value)
	} else {
		progress, err = r.Progress(id)
	}
	if err != nil {
		return nil, err
	}
	if !revisionPattern.MatchString(s(record, "revision")) ||
		!targetPattern.MatchString(s(record, "target")) {
		return nil, fail("invalid_state")
	}
	publish, ok := record["publish"].(bool)
	if !ok {
		return nil, fail("invalid_state")
	}
	created, ok := num(record["created"])
	if !ok {
		return nil, fail("invalid_state")
	}
	result := Object{"ok": true, "job_id": id, "revision": s(record, "revision"), "target": s(record, "target"),
		"phase": s(progress, "phase"), "code": s(progress, "code"), "publish": publish, "created": created, "finished": finished}
	if branch, ok := record["branch"]; ok {
		value, err := validateBranch(branch)
		if err != nil {
			return nil, fail("invalid_state")
		}
		result["branch"] = value
	}
	if digest, ok := progress["sha256"]; ok {
		result["sha256"] = digest
	}
	return result, nil
}

func (r *Runtime) Readiness() (Object, error) {
	if r.ReadyHook != nil {
		return r.ReadyHook()
	}
	if r.Cfg.RequireMount && !mounted(r.Cfg.WorkDir) {
		return nil, fail("work_disk_not_mounted")
	}
	if syscall.Access("/dev/kvm", 6) != nil {
		return nil, fail("kvm_unavailable")
	}
	for _, permission := range []string{"-r", "-w"} {
		if _, err := r.Invoke([]string{"runuser", "-u", r.UserName, "--", "test", permission, "/dev/kvm"}, CommandOptions{}); err != nil {
			return nil, fail("kvm_unavailable")
		}
	}
	for _, entry := range []struct{ path, kind, permission string }{{r.Cfg.KernelPath, "-f", "-r"}, {r.Cfg.KernelModules, "-d", "-x"}} {
		if entry.path == "" {
			continue
		}
		for _, permission := range []string{entry.kind, entry.permission} {
			if _, err := r.Invoke([]string{"runuser", "-u", r.UserName, "--", "test", permission, entry.path}, CommandOptions{}); err != nil {
				return nil, fail("kernel_unavailable")
			}
		}
	}
	var usage syscall.Statfs_t
	if syscall.Statfs(r.Cfg.WorkDir, &usage) != nil {
		return nil, fail("invalid_state")
	}
	if int64(usage.Bavail)*int64(usage.Bsize) < r.Cfg.MinimumFreeBytes {
		return nil, fail("insufficient_space")
	}
	for _, tool := range []string{"git", "packer", "qemu-img", "qemu-system-x86_64", "virt-sparsify", "rclone", "ssh", "ssh-agent", "ssh-add", "ssh-keygen", "cloud-localds", "curl", "xz"} {
		found := false
		for _, dir := range filepath.SplitList(baseEnv()["PATH"]) {
			if info, err := os.Stat(filepath.Join(dir, tool)); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
				found = true
				break
			}
		}
		if !found {
			return nil, fail("missing_tool")
		}
	}
	if _, err := r.Invoke([]string{"cloud-init", "status", "--wait"}, CommandOptions{Timeout: 120 * time.Second}); err != nil {
		return nil, err
	}
	return Object{"ok": true, "phase": "ready"}, nil
}

func (r *Runtime) storeRequest(id string, request Object) error {
	directory := filepath.Dir(r.RequestPath(id))
	if err := os.Mkdir(directory, 0750); err != nil {
		return fail("invalid_state")
	}
	if err := os.Chmod(directory, 0750); err != nil {
		return fail("invalid_state")
	}
	if err := r.chown(directory, 0, r.GID); err != nil {
		return fail("invalid_state")
	}
	if err := writeJSON(r.RequestPath(id), request, 0640); err != nil {
		return err
	}
	if err := r.chown(r.RequestPath(id), 0, r.GID); err != nil {
		return fail("invalid_state")
	}
	if err := os.Mkdir(r.WorkerPath(id), 0700); err != nil {
		return fail("invalid_state")
	}
	if err := r.chown(r.WorkerPath(id), r.UID, r.GID); err != nil {
		return fail("invalid_state")
	}
	return nil
}

func (r *Runtime) cleanupSecrets(id string) error {
	if !validJobID(id) {
		return fail("invalid_request")
	}
	for _, path := range []string{filepath.Dir(r.RequestPath(id)), r.WorkerPath(id)} {
		if err := os.RemoveAll(path); err != nil {
			return fail("invalid_state")
		}
	}
	return nil
}

func (r *Runtime) checkAdmission(state Object) error {
	if b(state, "stopping") {
		return fail("stopping")
	}
	if b(state, "maintenance") {
		return fail("maintenance")
	}
	if state["active"] != nil {
		return fail("busy")
	}
	return nil
}

func (r *Runtime) submit(request, state Object) (Object, error) {
	if err := r.checkAdmission(state); err != nil {
		return nil, err
	}
	if exists(r.PublicationGuard()) {
		return nil, fail("publication_blocked")
	}
	if !keysAllowed(request, "op", "revision", "target", "publish", "branch", "credentials") {
		return nil, fail("invalid_request")
	}
	revision, target := s(request, "revision"), s(request, "target")
	if !revisionPattern.MatchString(revision) || !targetPattern.MatchString(target) {
		return nil, fail("invalid_request")
	}
	publish := false
	if value, ok := request["publish"]; ok {
		var valid bool
		publish, valid = value.(bool)
		if !valid {
			return nil, fail("invalid_request")
		}
	}
	secretInput := request["credentials"]
	if secretInput == nil {
		secretInput = Object{}
	}
	values, err := credentials(secretInput, publish)
	if err != nil {
		return nil, err
	}
	if branch, ok := request["branch"]; ok {
		if _, err := validateBranch(branch); err != nil {
			return nil, err
		}
	}
	if _, err := r.Readiness(); err != nil {
		return nil, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, fail("internal_error")
	}
	id := hex.EncodeToString(random[:])
	if err := os.Mkdir(r.JobDir(id), 0700); err != nil {
		return nil, fail("invalid_state")
	}
	if err := r.chown(r.JobDir(id), r.UID, r.GID); err != nil {
		return nil, fail("invalid_state")
	}
	record := Object{"revision": revision, "target": target, "publish": publish, "created": r.now().Unix(), "finished": false}
	stored := Object{"op": "submit", "revision": revision, "target": target, "publish": publish, "credentials": values}
	if branch, ok := request["branch"]; ok {
		record["branch"] = branch
		stored["branch"] = branch
	}
	if err := writeJSON(r.RecordPath(id), record, 0600); err != nil {
		return nil, err
	}
	if err := r.storeRequest(id, stored); err != nil {
		_ = r.cleanupSecrets(id)
		return nil, err
	}
	state["active"] = id
	if err := r.saveControl(state); err != nil {
		_ = r.cleanupSecrets(id)
		return nil, err
	}
	if _, err := r.Invoke([]string{"systemctl", "start", "--no-block", "imagebuilder-job@" + id + ".service"}, CommandOptions{}); err != nil {
		if cleanupErr := r.cleanupSecrets(id); cleanupErr != nil {
			return nil, cleanupErr
		}
		record["finished"], record["result"] = true, Object{"phase": "failed", "code": "start_failed"}
		if err := writeJSON(r.RecordPath(id), record, 0600); err != nil {
			return nil, err
		}
		state["active"], state["idle_since"] = nil, r.now().Unix()
		if err := r.saveControl(state); err != nil {
			return nil, err
		}
		return nil, fail("start_failed")
	}
	return Object{"ok": true, "job_id": id, "phase": "preparing"}, nil
}

func (r *Runtime) retry(request, state Object) (Object, error) {
	if !keysAllowed(request, "op", "job_id", "credentials") || !validJobID(s(request, "job_id")) {
		return nil, fail("invalid_request")
	}
	id := s(request, "job_id")
	if err := r.checkAdmission(state); err != nil {
		return nil, err
	}
	if exists(r.PublicationGuard()) {
		guard, err := readJSON(r.PublicationGuard())
		if err != nil || s(guard, "job_id") != id {
			return nil, fail("publication_blocked")
		}
	}
	record, err := readJSON(r.RecordPath(id))
	if err != nil {
		return nil, fail("invalid_state")
	}
	result, ok := record["result"].(map[string]any)
	if !b(record, "finished") || !ok || !terminalSet[s(result, "phase")] {
		return nil, fail("job_not_finished")
	}
	if info, err := os.Lstat(filepath.Join(r.JobDir(id), "verified.json")); err != nil || !info.Mode().IsRegular() {
		return nil, fail("no_verified_artifact")
	}
	values, err := credentials(request["credentials"], true)
	if err != nil {
		return nil, err
	}
	if _, err := r.Readiness(); err != nil {
		return nil, err
	}
	stored := Object{"op": "publish-retry", "revision": record["revision"], "target": record["target"], "publish": true, "credentials": values}
	if err := r.storeRequest(id, stored); err != nil {
		_ = r.cleanupSecrets(id)
		return nil, err
	}
	if err := os.Remove(r.ProgressPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = r.cleanupSecrets(id)
		return nil, fail("invalid_state")
	}
	record["finished"], record["publish"] = false, true
	if err := writeJSON(r.RecordPath(id), record, 0600); err != nil {
		_ = r.cleanupSecrets(id)
		return nil, err
	}
	state["active"] = id
	if err := r.saveControl(state); err != nil {
		_ = r.cleanupSecrets(id)
		return nil, err
	}
	for _, args := range [][]string{{"systemctl", "reset-failed", "imagebuilder-job@" + id + ".service"}, {"systemctl", "start", "--no-block", "imagebuilder-job@" + id + ".service"}} {
		if _, err := r.Invoke(args, CommandOptions{}); err != nil {
			if finishErr := r.finishLocked(id, state); finishErr != nil {
				return nil, finishErr
			}
			return nil, fail("start_failed")
		}
	}
	return Object{"ok": true, "job_id": id, "phase": "preparing"}, nil
}

func (r *Runtime) Control(request Object) (Object, error) {
	op := s(request, "op")
	if op == "resolve" {
		if !keysAllowed(request, "op", "branch") || len(request) != 2 {
			return nil, fail("invalid_request")
		}
		branch, err := validateBranch(request["branch"])
		if err != nil {
			return nil, err
		}
		ref := "refs/heads/" + branch
		result, err := r.Invoke([]string{"runuser", "-u", r.UserName, "--", "git", "ls-remote", "--exit-code", "--refs", r.Cfg.RepoURL, ref}, CommandOptions{Capture: true})
		if err != nil {
			return nil, fail("branch_resolution_failed")
		}
		lines := strings.Split(strings.TrimSuffix(string(result.Output), "\n"), "\n")
		if len(lines) != 1 {
			return nil, fail("branch_resolution_failed")
		}
		fields := strings.Split(lines[0], "\t")
		if len(fields) != 2 || fields[1] != ref || !revisionPattern.MatchString(fields[0]) {
			return nil, fail("branch_resolution_failed")
		}
		return Object{"ok": true, "revision": fields[0], "branch": branch}, nil
	}
	return r.locked(func(state Object) (Object, error) {
		switch op {
		case "submit":
			return r.submit(request, state)
		case "publish-retry":
			return r.retry(request, state)
		case "ready":
			if !keysAllowed(request, "op", "build_ready") {
				return nil, fail("invalid_request")
			}
			buildReady := true
			if value, ok := request["build_ready"]; ok {
				var valid bool
				buildReady, valid = value.(bool)
				if !valid {
					return nil, fail("invalid_request")
				}
			}
			if b(state, "stopping") {
				return nil, fail("stopping")
			}
			result := Object{"ok": true, "phase": "ready"}
			if buildReady {
				var err error
				result, err = r.Readiness()
				if err != nil {
					return nil, err
				}
			}
			if state["active"] == nil {
				state["idle_since"] = r.now().Unix()
				if err := r.saveControl(state); err != nil {
					return nil, err
				}
			}
			return result, nil
		case "status", "logs":
			allowed := []string{"op", "job_id"}
			if op == "logs" {
				allowed = append(allowed, "after")
			}
			if !keysAllowed(request, allowed...) {
				return nil, fail("invalid_request")
			}
			id := s(request, "job_id")
			if value, ok := request["job_id"]; ok && value != nil && !validJobID(id) {
				return nil, fail("invalid_request")
			}
			if id == "" {
				id = s(state, "active")
			}
			if id == "" {
				paths, err := filepath.Glob(filepath.Join(r.recordsDir(), "*.json"))
				if err != nil {
					return nil, fail("invalid_state")
				}
				var newest time.Time
				for _, path := range paths {
					info, err := os.Stat(path)
					if err != nil {
						return nil, fail("invalid_state")
					}
					if id == "" || !info.ModTime().Before(newest) {
						id = strings.TrimSuffix(filepath.Base(path), ".json")
						newest = info.ModTime()
					}
				}
			}
			if id == "" {
				phase := "idle"
				if b(state, "stopping") {
					phase = "stopping"
				}
				return Object{"ok": true, "phase": phase, "maintenance": state["maintenance"], "publication_blocked": exists(r.PublicationGuard())}, nil
			}
			result, err := r.safeJob(id)
			if err != nil {
				return nil, err
			}
			result["maintenance"], result["publication_blocked"] = state["maintenance"], exists(r.PublicationGuard())
			if op == "logs" {
				progress, err := r.Progress(id)
				if err != nil {
					return nil, err
				}
				result["events"] = progress["events"]
				after := int64(0)
				if value, ok := request["after"]; ok {
					var valid bool
					after, valid = num(value)
					if !valid || after < 0 {
						return nil, fail("invalid_request")
					}
				}
				page, err := r.DiagnosticPage(id, after)
				if err != nil {
					return nil, err
				}
				for key, value := range page {
					result[key] = value
				}
			}
			return result, nil
		case "maintenance":
			if !keysAllowed(request, "op", "enabled") || len(request) != 2 {
				return nil, fail("invalid_request")
			}
			enabled, ok := request["enabled"].(bool)
			if !ok {
				return nil, fail("invalid_request")
			}
			if b(state, "stopping") {
				return nil, fail("stopping")
			}
			state["maintenance"], state["idle_since"] = enabled, r.now().Unix()
			if err := writeJSON(r.maintenancePath(), Object{"enabled": enabled}, 0600); err != nil {
				return nil, err
			}
			if err := r.saveControl(state); err != nil {
				return nil, err
			}
			return Object{"ok": true, "maintenance": enabled}, nil
		case "shutdown":
			if !keysAllowed(request, "op") || len(request) != 1 {
				return nil, fail("invalid_request")
			}
			return r.shutdown(state)
		default:
			return nil, fail("invalid_request")
		}
	})
}

func (r *Runtime) shutdown(state Object) (Object, error) {
	if state["active"] != nil {
		return nil, fail("busy")
	}
	if b(state, "stopping") {
		return nil, fail("stopping")
	}
	state["stopping"] = true
	if err := r.saveControl(state); err != nil {
		return nil, err
	}
	if _, err := r.Invoke([]string{"systemctl", "poweroff", "--no-block"}, CommandOptions{}); err != nil {
		state["stopping"] = false
		if err := r.saveControl(state); err != nil {
			return nil, err
		}
		return nil, fail("shutdown_failed")
	}
	return Object{"ok": true, "phase": "stopping"}, nil
}

func (r *Runtime) finishLocked(id string, state Object) error {
	record, err := readJSON(r.RecordPath(id))
	if err != nil {
		return fail("invalid_state")
	}
	progress, progressErr := r.Progress(id)
	result := Object{}
	if progressErr != nil {
		code := "invalid_progress"
		if b(record, "publish") {
			code = "publication_blocked"
		}
		result = Object{"phase": "failed", "code": code}
	} else if !terminalSet[s(progress, "phase")] {
		code := "service_failed"
		if s(progress, "phase") == "publishing" {
			code = "publication_blocked"
		}
		result = Object{"phase": "failed", "code": code}
	} else {
		result = Object{"phase": s(progress, "phase"), "code": s(progress, "code")}
		if value, ok := progress["sha256"]; ok {
			result["sha256"] = value
		}
	}
	if err := r.cleanupSecrets(id); err != nil {
		return err
	}
	if s(result, "code") == "publication_blocked" {
		if err := writeJSON(r.PublicationGuard(), Object{"job_id": id}, 0600); err != nil {
			return err
		}
	} else if s(result, "phase") == "succeeded" && s(result, "code") == "published" && exists(r.PublicationGuard()) {
		guard, err := readJSON(r.PublicationGuard())
		if err != nil {
			return fail("invalid_state")
		}
		if s(guard, "job_id") == id {
			if err := os.Remove(r.PublicationGuard()); err != nil {
				return fail("invalid_state")
			}
		}
	}
	record["finished"], record["result"] = true, result
	if err := writeJSON(r.RecordPath(id), record, 0600); err != nil {
		return err
	}
	if s(state, "active") == id {
		state["active"], state["idle_since"] = nil, r.now().Unix()
		return r.saveControl(state)
	}
	return nil
}

func (r *Runtime) Finish(id string) error {
	if !validJobID(id) {
		return fail("invalid_request")
	}
	_, err := r.locked(func(state Object) (Object, error) { return nil, r.finishLocked(id, state) })
	return err
}

func (r *Runtime) Boot() error {
	if err := r.Initialize(); err != nil {
		return err
	}
	_, err := r.locked(func(state Object) (Object, error) {
		if id := s(state, "active"); id != "" {
			result, err := r.Invoke([]string{"systemctl", "show", "imagebuilder-job@" + id + ".service", "--property=ActiveState", "--value"}, CommandOptions{Capture: true})
			if err != nil {
				return nil, err
			}
			active := strings.TrimSpace(string(result.Output))
			switch active {
			case "inactive", "failed":
			case "active", "activating", "reloading", "deactivating":
				return nil, fail("busy")
			default:
				return nil, fail("invalid_state")
			}
		}
		paths, err := filepath.Glob(filepath.Join(r.recordsDir(), "*.json"))
		if err != nil {
			return nil, fail("invalid_state")
		}
		for _, path := range paths {
			id := strings.TrimSuffix(filepath.Base(path), ".json")
			if !validJobID(id) {
				return nil, fail("invalid_state")
			}
			record, err := readJSON(path)
			if err != nil {
				return nil, err
			}
			if !b(record, "finished") {
				progress, progressErr := r.Progress(id)
				publishing := (progressErr != nil && b(record, "publish")) || s(progress, "phase") == "publishing" || exists(filepath.Join(r.JobDir(id), "publication.json"))
				if publishing {
					if err := writeJSON(r.PublicationGuard(), Object{"job_id": id}, 0600); err != nil {
						return nil, err
					}
				}
				record["finished"], record["result"] = true, Object{"phase": "interrupted", "code": "host_restarted"}
				if err := writeJSON(path, record, 0600); err != nil {
					return nil, err
				}
			}
		}
		for _, parent := range []string{"jobs", "worker"} {
			paths, err := os.ReadDir(filepath.Join(r.Cfg.RuntimeDir, parent))
			if err != nil {
				return nil, fail("invalid_state")
			}
			for _, path := range paths {
				if validJobID(path.Name()) {
					if err := os.RemoveAll(filepath.Join(r.Cfg.RuntimeDir, parent, path.Name())); err != nil {
						return nil, fail("invalid_state")
					}
				}
			}
		}
		maintenance := false
		if exists(r.maintenancePath()) {
			saved, err := readJSON(r.maintenancePath())
			if err != nil {
				return nil, err
			}
			var ok bool
			maintenance, ok = saved["enabled"].(bool)
			if !ok {
				return nil, fail("invalid_state")
			}
		}
		state["active"], state["maintenance"], state["stopping"], state["idle_since"] = nil, maintenance, false, r.now().Unix()
		return nil, r.saveControl(state)
	})
	return err
}

func (r *Runtime) Idle() error {
	_, err := r.locked(func(state Object) (Object, error) {
		if err := r.prune(state); err != nil {
			return nil, err
		}
		if state["active"] == nil && !b(state, "maintenance") && !b(state, "stopping") && r.now().Unix()-n(state, "idle_since") >= int64(r.Cfg.IdleSeconds) {
			return r.shutdown(state)
		}
		return nil, nil
	})
	return err
}

func (r *Runtime) prune(state Object) error {
	cutoff := r.now().Unix() - int64(r.Cfg.RetentionDays)*86400
	blocked := ""
	if exists(r.PublicationGuard()) {
		guard, err := readJSON(r.PublicationGuard())
		if err != nil || !validJobID(s(guard, "job_id")) {
			return fail("invalid_state")
		}
		blocked = s(guard, "job_id")
	}
	paths, err := filepath.Glob(filepath.Join(r.recordsDir(), "*.json"))
	if err != nil {
		return fail("invalid_state")
	}
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		if !validJobID(id) {
			return fail("invalid_state")
		}
		record, err := readJSON(path)
		if err != nil {
			return err
		}
		created, ok := num(record["created"])
		if !ok {
			return fail("invalid_state")
		}
		if b(record, "finished") && created < cutoff && id != s(state, "active") && id != blocked {
			if err := os.RemoveAll(r.JobDir(id)); err != nil {
				return fail("invalid_state")
			}
			if err := os.Remove(path); err != nil {
				return fail("invalid_state")
			}
		}
	}
	if state["active"] == nil && exists(filepath.Join(r.cacheDir(), "packer")) {
		_, err := r.Invoke([]string{"runuser", "-u", r.UserName, "--", "/usr/local/libexec/imagebuilder", "cache-prune"}, CommandOptions{})
		return err
	}
	return nil
}

func (r *Runtime) PruneCache() error {
	if os.Geteuid() != r.UID {
		return fail("invalid_config")
	}
	cache := filepath.Join(r.cacheDir(), "packer")
	info, err := os.Lstat(cache)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fail("invalid_state")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(cache); err != nil {
			return fail("invalid_state")
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
			return fail("invalid_state")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fail("invalid_state")
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
		return fail("invalid_state")
	}
	limit := r.Cfg.CacheMaxBytes
	if limit == 0 {
		limit = 20 * 1024 * 1024 * 1024
	}
	if count > 100000 || total > limit || (count > 0 && newest.Before(r.now().Add(-time.Duration(r.Cfg.RetentionDays)*24*time.Hour))) {
		if err := os.RemoveAll(cache); err != nil {
			return fail("invalid_state")
		}
	}
	return nil
}
