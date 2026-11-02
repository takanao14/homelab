package builder

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

func (r *Runtime) shutdown(state *ControlState) (protocol.Object, error) {
	if state.Active != "" {
		return nil, protocol.Fail("busy")
	}
	if state.Stopping {
		return nil, protocol.Fail("stopping")
	}
	state.Stopping = true
	if err := r.saveControl(state); err != nil {
		return nil, err
	}
	if _, err := r.Invoke([]string{"systemctl", "poweroff", "--no-block"}, command.Options{}); err != nil {
		state.Stopping = false
		if err := r.saveControl(state); err != nil {
			return nil, err
		}
		return nil, protocol.Fail("shutdown_failed")
	}
	return protocol.Object{"ok": true, "phase": "stopping"}, nil
}

func (r *Runtime) finishLocked(id string, state *ControlState, serviceFailed bool) error {
	record, err := readJobRecord(r.RecordPath(id))
	if err != nil {
		return err
	}
	var result JobResult
	if record.Finished {
		if record.Result == nil {
			return protocol.Fail("invalid_state")
		}
		result = *record.Result
	} else {
		progress, err := r.readProgress(id)
		if err != nil {
			result = JobResult{Phase: "failed", Code: "invalid_progress"}
		} else if !protocol.TerminalSet[progress.Phase] {
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
	if !protocol.ValidJobID(id) {
		return protocol.Fail("invalid_request")
	}
	// Only systemd's fixed success tuple may confirm a successful process exit.
	serviceResult, exitCode, exitStatus := os.Getenv("SERVICE_RESULT"), os.Getenv("EXIT_CODE"), os.Getenv("EXIT_STATUS")
	provided := serviceResult != "" || exitCode != "" || exitStatus != ""
	serviceFailed := provided && (serviceResult != "success" || exitCode != "exited" || exitStatus != "0")
	_, err := r.locked(func(state *ControlState) (protocol.Object, error) {
		return nil, r.finishLocked(id, state, serviceFailed)
	})
	return err
}

func (r *Runtime) Boot() error {
	if err := r.Initialize(); err != nil {
		return err
	}
	_, err := r.locked(func(state *ControlState) (protocol.Object, error) {
		if id := state.Active; id != "" {
			result, err := r.Invoke([]string{"systemctl", "show", "imagebuilder-job@" + id + ".service", "--property=ActiveState", "--value"}, command.Options{Capture: true})
			if err != nil {
				return nil, err
			}
			active := strings.TrimSpace(string(result.Output))
			switch active {
			case "inactive", "failed":
			case "active", "activating", "reloading", "deactivating":
				return nil, protocol.Fail("busy")
			default:
				return nil, protocol.Fail("invalid_state")
			}
		}
		paths, err := filepath.Glob(filepath.Join(r.recordsDir(), "*.json"))
		if err != nil {
			return nil, protocol.Fail("invalid_state")
		}
		for _, path := range paths {
			id := strings.TrimSuffix(filepath.Base(path), ".json")
			if !protocol.ValidJobID(id) {
				return nil, protocol.Fail("invalid_state")
			}
			record, err := readJobRecord(path)
			if err != nil {
				// An unreadable record must not block initialization; status reports it as invalid_state.
				continue
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
				return nil, protocol.Fail("invalid_state")
			}
			for _, path := range paths {
				if protocol.ValidJobID(path.Name()) {
					if err := os.RemoveAll(filepath.Join(r.Cfg.RuntimeDir, parent, path.Name())); err != nil {
						return nil, protocol.Fail("invalid_state")
					}
				}
			}
		}
		maintenance := false
		if exists(r.maintenancePath()) {
			saved, err := store.ReadJSON(r.maintenancePath())
			if err != nil {
				return nil, err
			}
			var ok bool
			maintenance, ok = saved["enabled"].(bool)
			if !ok {
				return nil, protocol.Fail("invalid_state")
			}
		}
		state.Active, state.Maintenance, state.Stopping, state.IdleSince = "", maintenance, false, r.now().Unix()
		return nil, r.saveControl(state)
	})
	if err != nil {
		return err
	}
	// Cache eviction is best effort and must not block job admission after boot.
	_ = r.pruneCache()
	return nil
}

func (r *Runtime) Idle() error {
	_, err := r.locked(func(state *ControlState) (protocol.Object, error) {
		// Retention failures must not keep an idle builder powered on.
		pruneErr := r.prune(state)
		if state.Active == "" && !state.Maintenance && !state.Stopping && r.now().Unix()-state.IdleSince >= int64(r.Cfg.IdleSeconds) {
			if _, err := r.shutdown(state); err != nil {
				return nil, err
			}
		}
		return nil, pruneErr
	})
	return err
}
