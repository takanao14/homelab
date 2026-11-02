package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	return r.locked(func(state *ControlState) (Object, error) {
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
			if state.Stopping {
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
			if state.Active == "" {
				state.IdleSince = r.now().Unix()
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
				id = state.Active
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
				if state.Stopping {
					phase = "stopping"
				}
				return Object{"ok": true, "phase": phase, "maintenance": state.Maintenance}, nil
			}
			result, err := r.safeJob(id)
			if err != nil {
				return nil, err
			}
			result["maintenance"] = state.Maintenance
			if op == "logs" {
				progress, err := r.readProgress(id)
				if err != nil {
					return nil, err
				}
				result["events"] = progressEventsObject(progress.Events)
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
			if state.Stopping {
				return nil, fail("stopping")
			}
			state.Maintenance, state.IdleSince = enabled, r.now().Unix()
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
