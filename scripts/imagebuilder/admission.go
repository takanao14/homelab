package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func (r *Runtime) locked(callback func(*ControlState) (Object, error)) (Object, error) {
	fd, err := syscall.Open(filepath.Join(r.Cfg.RuntimeDir, "control.lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fail("invalid_state")
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return nil, fail("invalid_state")
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	data, err := readJSON(r.ControlPath())
	if err != nil {
		return nil, err
	}
	state, err := controlStateFromObject(data)
	if err != nil {
		return nil, err
	}
	return callback(&state)
}

func (r *Runtime) saveControl(state *ControlState) error {
	return writeJSON(r.ControlPath(), state.Object(), 0600)
}

func (r *Runtime) checkAdmission(state *ControlState) error {
	if state.Stopping {
		return fail("stopping")
	}
	if state.Maintenance {
		return fail("maintenance")
	}
	if state.Active != "" {
		return fail("busy")
	}
	return nil
}

// Build-environment checks run in the controller's preceding ready op, which retries transient failures.
func (r *Runtime) submit(request Object, state *ControlState) (Object, error) {
	if err := r.checkAdmission(state); err != nil {
		return nil, err
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
	record := JobRecord{Revision: revision, Target: target, Publish: publish, Created: r.now().Unix()}
	stored := Object{"op": "submit", "revision": revision, "target": target, "publish": publish, "credentials": values}
	if branch, ok := request["branch"]; ok {
		record.Branch = branch.(string)
		stored["branch"] = branch
	}
	return r.launchLocked(id, record, stored, state, nil)
}

func (r *Runtime) retry(request Object, state *ControlState) (Object, error) {
	if !keysAllowed(request, "op", "job_id", "credentials") || !validJobID(s(request, "job_id")) {
		return nil, fail("invalid_request")
	}
	id := s(request, "job_id")
	if err := r.checkAdmission(state); err != nil {
		return nil, err
	}
	record, err := readJobRecord(r.RecordPath(id))
	if err != nil {
		return nil, fail("invalid_state")
	}
	if !record.Finished || record.Result == nil || !terminalSet[record.Result.Phase] {
		return nil, fail("job_not_finished")
	}
	if info, err := os.Lstat(filepath.Join(r.JobDir(id), "verified.json")); err != nil || !info.Mode().IsRegular() {
		return nil, fail("no_verified_artifact")
	}
	values, err := credentials(request["credentials"], true)
	if err != nil {
		return nil, err
	}
	stored := Object{"op": "publish-retry", "revision": record.Revision, "target": record.Target, "publish": true, "credentials": values}
	if err := os.Remove(r.ProgressPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fail("invalid_state")
	}
	previous := record
	record.Finished, record.Publish, record.Result = false, true, nil
	return r.launchLocked(id, record, stored, state, &previous)
}

// A failed retry restores previous so the job keeps its verified outcome.
func (r *Runtime) launchLocked(id string, record JobRecord, request Object, state *ControlState, previous *JobRecord) (Object, error) {
	if err := writeJobRecord(r.RecordPath(id), record); err != nil {
		return nil, err
	}
	state.Active = id
	if err := r.saveControl(state); err != nil {
		return nil, err
	}
	start := func() error {
		if err := r.storeRequest(id, request); err != nil {
			return err
		}
		unit := "imagebuilder-job@" + id + ".service"
		if previous != nil {
			// systemd rejects reset-failed for unloaded units, e.g. after success or a reboot.
			_, _ = r.Invoke([]string{"systemctl", "reset-failed", unit}, CommandOptions{})
		}
		_, err := r.Invoke([]string{"systemctl", "start", "--no-block", unit}, CommandOptions{})
		return err
	}
	if err := start(); err != nil {
		final, result := record, JobResult{Phase: "failed", Code: "start_failed"}
		if previous != nil {
			final, result = *previous, *previous.Result
		}
		if cleanupErr := r.completeLocked(id, final, state, result); cleanupErr != nil {
			return nil, cleanupErr
		}
		return nil, fail("start_failed")
	}
	return Object{"ok": true, "job_id": id, "phase": "preparing"}, nil
}
