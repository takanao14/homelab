package builder

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

func (r *Runtime) locked(callback func(*ControlState) (protocol.Object, error)) (protocol.Object, error) {
	fd, err := syscall.Open(filepath.Join(r.Cfg.RuntimeDir, "control.lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, protocol.Fail("invalid_state")
	}
	defer syscall.Close(fd)
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return nil, protocol.Fail("invalid_state")
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	data, err := store.ReadJSON(r.ControlPath())
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
	return store.WriteJSON(r.ControlPath(), state.Object(), 0600)
}

func (r *Runtime) checkAdmission(state *ControlState) error {
	if state.Stopping {
		return protocol.Fail("stopping")
	}
	if state.Maintenance {
		return protocol.Fail("maintenance")
	}
	if state.Active != "" {
		return protocol.Fail("busy")
	}
	return nil
}

// Build-environment checks run in the controller's preceding ready op, which retries transient failures.
func (r *Runtime) submit(request protocol.Object, state *ControlState) (protocol.Object, error) {
	if err := r.checkAdmission(state); err != nil {
		return nil, err
	}
	if !protocol.KeysAllowed(request, "op", "revision", "target", "publish", "branch", "credentials") {
		return nil, protocol.Fail("invalid_request")
	}
	revision, target := protocol.String(request, "revision"), protocol.String(request, "target")
	if !protocol.RevisionPattern.MatchString(revision) || !protocol.TargetPattern.MatchString(target) {
		return nil, protocol.Fail("invalid_request")
	}
	publish := false
	if value, ok := request["publish"]; ok {
		var valid bool
		publish, valid = value.(bool)
		if !valid {
			return nil, protocol.Fail("invalid_request")
		}
	}
	secretInput := request["credentials"]
	if secretInput == nil {
		secretInput = protocol.Object{}
	}
	values, err := credentials(secretInput, publish)
	if err != nil {
		return nil, err
	}
	if branch, ok := request["branch"]; ok {
		if _, err := protocol.ValidateBranch(branch); err != nil {
			return nil, err
		}
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, protocol.Fail("internal_error")
	}
	id := hex.EncodeToString(random[:])
	if err := os.Mkdir(r.JobDir(id), 0700); err != nil {
		return nil, protocol.Fail("invalid_state")
	}
	if err := r.chown(r.JobDir(id), r.UID, r.GID); err != nil {
		return nil, protocol.Fail("invalid_state")
	}
	record := JobRecord{Revision: revision, Target: target, Publish: publish, Created: r.now().Unix()}
	stored := protocol.Object{"op": "submit", "revision": revision, "target": target, "publish": publish, "credentials": values}
	if branch, ok := request["branch"]; ok {
		record.Branch = branch.(string)
		stored["branch"] = branch
	}
	return r.launchLocked(id, record, stored, state, nil)
}

func (r *Runtime) retry(request protocol.Object, state *ControlState) (protocol.Object, error) {
	if !protocol.KeysAllowed(request, "op", "job_id", "credentials") || !protocol.ValidJobID(protocol.String(request, "job_id")) {
		return nil, protocol.Fail("invalid_request")
	}
	id := protocol.String(request, "job_id")
	if err := r.checkAdmission(state); err != nil {
		return nil, err
	}
	record, err := readJobRecord(r.RecordPath(id))
	if err != nil {
		return nil, protocol.Fail("invalid_state")
	}
	if !record.Finished || record.Result == nil || !protocol.TerminalSet[record.Result.Phase] {
		return nil, protocol.Fail("job_not_finished")
	}
	if info, err := os.Lstat(filepath.Join(r.JobDir(id), "verified.json")); err != nil || !info.Mode().IsRegular() {
		return nil, protocol.Fail("no_verified_artifact")
	}
	values, err := credentials(request["credentials"], true)
	if err != nil {
		return nil, err
	}
	stored := protocol.Object{"op": "publish-retry", "revision": record.Revision, "target": record.Target, "publish": true, "credentials": values}
	if err := os.Remove(r.ProgressPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, protocol.Fail("invalid_state")
	}
	previous := record
	record.Finished, record.Publish, record.Result = false, true, nil
	return r.launchLocked(id, record, stored, state, &previous)
}

// A failed retry restores previous so the job keeps its verified outcome.
func (r *Runtime) launchLocked(id string, record JobRecord, request protocol.Object, state *ControlState, previous *JobRecord) (protocol.Object, error) {
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
			_, _ = r.Invoke([]string{"systemctl", "reset-failed", unit}, command.Options{})
		}
		_, err := r.Invoke([]string{"systemctl", "start", "--no-block", unit}, command.Options{})
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
		return nil, protocol.Fail("start_failed")
	}
	return protocol.Object{"ok": true, "job_id": id, "phase": "preparing"}, nil
}
