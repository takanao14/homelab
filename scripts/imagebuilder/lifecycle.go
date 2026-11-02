package main

import (
	"os"
	"path/filepath"
	"strings"
)

func (r *Runtime) shutdown(state *ControlState) (Object, error) {
	if state.Active != "" {
		return nil, fail("busy")
	}
	if state.Stopping {
		return nil, fail("stopping")
	}
	state.Stopping = true
	if err := r.saveControl(state); err != nil {
		return nil, err
	}
	if _, err := r.Invoke([]string{"systemctl", "poweroff", "--no-block"}, CommandOptions{}); err != nil {
		state.Stopping = false
		if err := r.saveControl(state); err != nil {
			return nil, err
		}
		return nil, fail("shutdown_failed")
	}
	return Object{"ok": true, "phase": "stopping"}, nil
}

func (r *Runtime) finishLocked(id string, state *ControlState, serviceFailed bool) error {
	record, err := readJobRecord(r.RecordPath(id))
	if err != nil {
		return err
	}
	var result JobResult
	if record.Finished {
		if record.Result == nil {
			return fail("invalid_state")
		}
		result = *record.Result
	} else {
		progress, err := r.readProgress(id)
		if err != nil {
			result = JobResult{Phase: "failed", Code: "invalid_progress"}
		} else if !terminalSet[progress.Phase] {
			result = JobResult{Phase: "failed", Code: "service_failed"}
		} else {
			result = progress.JobResult
		}
		if serviceFailed && result.Phase == "succeeded" {
			result = JobResult{Phase: "failed", Code: "service_failed"}
		}
	}
	return r.completeLocked(id, record, state, result)
}

func (r *Runtime) completeLocked(id string, record JobRecord, state *ControlState, result JobResult) error {
	if err := r.cleanupSecrets(id); err != nil {
		return err
	}
	record.Finished, record.Result = true, &result
	if err := writeJobRecord(r.RecordPath(id), record); err != nil {
		return err
	}
	if state.Active == id {
		state.Active, state.IdleSince = "", r.now().Unix()
		return r.saveControl(state)
	}
	return nil
}

func (r *Runtime) Finish(id string) error {
	if !validJobID(id) {
		return fail("invalid_request")
	}
	// Only systemd's fixed success tuple may confirm a successful process exit.
	serviceResult, exitCode, exitStatus := os.Getenv("SERVICE_RESULT"), os.Getenv("EXIT_CODE"), os.Getenv("EXIT_STATUS")
	provided := serviceResult != "" || exitCode != "" || exitStatus != ""
	serviceFailed := provided && (serviceResult != "success" || exitCode != "exited" || exitStatus != "0")
	_, err := r.locked(func(state *ControlState) (Object, error) { return nil, r.finishLocked(id, state, serviceFailed) })
	return err
}

func (r *Runtime) Boot() error {
	if err := r.Initialize(); err != nil {
		return err
	}
	_, err := r.locked(func(state *ControlState) (Object, error) {
		if id := state.Active; id != "" {
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
			record, err := readJobRecord(path)
			if err != nil {
				return nil, err
			}
			if !record.Finished {
				record.Finished, record.Result = true, &JobResult{Phase: "interrupted", Code: "host_restarted"}
				if err := writeJobRecord(path, record); err != nil {
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
		state.Active, state.Maintenance, state.Stopping, state.IdleSince = "", maintenance, false, r.now().Unix()
		return nil, r.saveControl(state)
	})
	return err
}

func (r *Runtime) Idle() error {
	_, err := r.locked(func(state *ControlState) (Object, error) {
		if err := r.prune(state); err != nil {
			return nil, err
		}
		if state.Active == "" && !state.Maintenance && !state.Stopping && r.now().Unix()-state.IdleSince >= int64(r.Cfg.IdleSeconds) {
			return r.shutdown(state)
		}
		return nil, nil
	})
	return err
}
