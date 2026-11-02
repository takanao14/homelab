package main

import (
	"errors"
	"os"
	"path/filepath"
)

func (r *Runtime) controlDir() string { return filepath.Join(r.Cfg.WorkDir, "control") }

func (r *Runtime) recordsDir() string { return filepath.Join(r.controlDir(), "jobs") }

func (r *Runtime) jobsDir() string { return filepath.Join(r.Cfg.WorkDir, "jobs") }

func (r *Runtime) cacheDir() string { return filepath.Join(r.Cfg.WorkDir, "cache") }

func (r *Runtime) ControlPath() string { return filepath.Join(r.Cfg.RuntimeDir, "control.json") }

func (r *Runtime) RecordPath(id string) string { return filepath.Join(r.recordsDir(), id+".json") }

func (r *Runtime) RequestPath(id string) string {
	return filepath.Join(r.Cfg.RuntimeDir, "jobs", id, "request.json")
}

func (r *Runtime) WorkerPath(id string) string { return filepath.Join(r.Cfg.RuntimeDir, "worker", id) }

func (r *Runtime) JobDir(id string) string { return filepath.Join(r.jobsDir(), id) }

func (r *Runtime) ProgressPath(id string) string { return filepath.Join(r.JobDir(id), "progress.json") }

func (r *Runtime) DiagnosticsPath(id string) string {
	return filepath.Join(r.JobDir(id), "diagnostics.json")
}

func (r *Runtime) maintenancePath() string { return filepath.Join(r.controlDir(), "maintenance.json") }

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
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
		return r.saveControl(&ControlState{IdleSince: r.now().Unix()})
	}
	return nil
}

func (r *Runtime) readProgress(id string) (Progress, error) {
	if !validJobID(id) {
		return Progress{}, fail("invalid_request")
	}
	if !exists(r.ProgressPath(id)) {
		return Progress{JobResult: JobResult{Phase: "preparing", Code: "accepted"}, Events: []ProgressEvent{}}, nil
	}
	data, err := readJSON(r.ProgressPath(id))
	if err != nil {
		return Progress{}, err
	}
	return progressFromObject(data)
}

func (r *Runtime) Progress(id string) (Object, error) {
	progress, err := r.readProgress(id)
	if err != nil {
		return nil, err
	}
	return progress.Object(), nil
}

func (r *Runtime) Update(id, phase, code string, fields Object) error {
	progress, err := r.readProgress(id)
	if err != nil {
		return err
	}
	progress.JobResult = JobResult{Phase: phase, Code: code}
	if fields != nil {
		if value, ok := fields["sha256"]; ok {
			digest, valid := value.(string)
			if !valid || !shaPattern.MatchString(digest) {
				return fail("invalid_state")
			}
			progress.SHA256 = digest
		}
	}
	progress.Events = append(progress.Events, ProgressEvent{Phase: phase, Code: code, Time: r.now().Unix()})
	if len(progress.Events) > 100 {
		progress.Events = progress.Events[len(progress.Events)-100:]
	}
	if err := progress.JobResult.Validate(); err != nil {
		return err
	}
	return writeJSON(r.ProgressPath(id), progress.Object(), 0600)
}

func (r *Runtime) safeJob(id string) (Object, error) {
	if !validJobID(id) {
		return nil, fail("invalid_request")
	}
	record, err := readJobRecord(r.RecordPath(id))
	if err != nil {
		return nil, err
	}
	var result JobResult
	if record.Finished {
		if record.Result == nil {
			return nil, fail("invalid_state")
		}
		result = *record.Result
	} else {
		progress, err := r.readProgress(id)
		if err != nil {
			return nil, err
		}
		result = progress.JobResult
	}
	return publicJobStatus(id, record, result), nil
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
