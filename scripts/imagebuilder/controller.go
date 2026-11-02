package main

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// All controller failures use fixed codes shared with the runtime protocol.
func controlFail(code string) error { return fail(code) }

type imageController struct {
	env                                map[string]string
	url, node, token, host, user, vmid string
	client                             *http.Client
	ssh                                controllerSSH
	now                                func() time.Time
	sleep                              func(time.Duration)
	out                                io.Writer
}

func (c *imageController) wake(buildReady bool) (map[string]any, error) {
	deadline := c.now().Add(900 * time.Second)
	last := "builder_start_timeout"
	for c.now().Before(deadline) {
		power, err := c.power()
		if err != nil {
			return nil, err
		}
		if power == "stopped" {
			if _, err = c.api("qemu/"+c.vmid+"/status/start", true); err != nil {
				power, checkErr := c.power()
				if checkErr != nil {
					return nil, checkErr
				}
				if power != "running" {
					return nil, err
				}
			}
		}
		request := map[string]any{"op": "ready"}
		if !buildReady {
			request["build_ready"] = false
		}
		response, err := c.remote(request)
		if err == nil {
			return response, nil
		}
		last = err.Error()
		switch last {
		case "builder_unreachable", "internal_error", "stopping", "work_disk_not_mounted", "command_failed", "command_timeout":
		default:
			return nil, err
		}
		c.sleep(5 * time.Second)
	}
	return nil, controlFail(last)
}

func (c *imageController) secrets(publish bool) (map[string]string, error) {
	values := map[string]string{}
	for _, key := range []string{"PKR_VAR_user_password", "SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY", "SEAWEEDFS_CLOUD_IMAGES_BUCKET"} {
		if value := c.env[key]; value != "" {
			values[key] = value
		}
	}
	if publish {
		for _, key := range []string{"SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY"} {
			if values[key] == "" {
				return nil, controlFail("missing_publish_credentials")
			}
		}
	} else {
		for key := range values {
			if key != "PKR_VAR_user_password" {
				delete(values, key)
			}
		}
	}
	return values, nil
}

func (c *imageController) emit(value map[string]any) error {
	if err := json.NewEncoder(c.out).Encode(value); err != nil {
		return controlFail("controller_failed")
	}
	return nil
}

func (c *imageController) followLogs(identifier string, after int64) (map[string]any, error) {
	deadline := c.now().Add(14700 * time.Second)
	for c.now().Before(deadline) {
		page, err := c.remote(map[string]any{"op": "logs", "job_id": identifier, "after": after})
		if err != nil {
			return nil, err
		}
		if page["job_id"] != identifier {
			return nil, controlFail("unexpected_job_identity")
		}
		cursor, valid := controllerInteger(page["cursor"])
		hasMore, validMore := page["has_more"].(bool)
		if !valid || cursor < after || !validMore {
			return nil, controlFail("invalid_builder_response")
		}
		if truncated, _ := page["truncated"].(bool); truncated {
			if err := c.emit(map[string]any{"ok": true, "job_id": identifier, "truncated": true, "cursor": after}); err != nil {
				return nil, err
			}
		}
		records, _ := page["records"].([]any)
		for _, raw := range records {
			record, ok := raw.(map[string]any)
			if !ok {
				return nil, controlFail("invalid_builder_response")
			}
			sequence, valid := controllerInteger(record["sequence"])
			if !valid || sequence <= after {
				return nil, controlFail("invalid_builder_response")
			}
			result := map[string]any{"job_id": identifier}
			for key, value := range record {
				result[key] = value
			}
			if err := c.emit(result); err != nil {
				return nil, err
			}
			after = sequence
		}
		if cursor != after {
			return nil, controlFail("invalid_builder_response")
		}
		if hasMore {
			if len(records) == 0 {
				return nil, controlFail("invalid_builder_response")
			}
			continue
		}
		if finished, _ := page["finished"].(bool); finished {
			if !controllerTerminal(page["phase"]) {
				return nil, controlFail("invalid_builder_response")
			}
			result := map[string]any{}
			for _, key := range []string{"ok", "job_id", "phase", "code", "finished", "cursor", "branch", "revision"} {
				if value, exists := page[key]; exists {
					result[key] = value
				}
			}
			return result, nil
		}
		c.sleep(2 * time.Second)
	}
	return nil, controlFail("log_wait_timeout")
}

func (c *imageController) waitStopped() (map[string]any, error) {
	deadline := c.now().Add(900 * time.Second)
	for c.now().Before(deadline) {
		state, err := c.power()
		if err != nil {
			return nil, err
		}
		if state == "stopped" {
			return map[string]any{"ok": true, "power": "stopped"}, nil
		}
		c.sleep(5 * time.Second)
	}
	return nil, controlFail("shutdown_timeout")
}

func (c *imageController) execute(args controllerArgs) (map[string]any, error) {
	if args.command == "status" || args.command == "shutdown" {
		power, err := c.power()
		if err != nil {
			return nil, err
		}
		if power == "stopped" {
			return map[string]any{"ok": true, "power": "stopped"}, nil
		}
		request := map[string]any{"op": args.command}
		if args.jobID != "" {
			request["job_id"] = args.jobID
		}
		result, err := c.remote(request)
		if err != nil {
			return nil, err
		}
		if args.command == "status" {
			return result, nil
		}
		return c.waitStopped()
	}
	var request map[string]any
	if args.command == "run" || args.command == "publish-retry" {
		values, err := c.secrets(args.publish || args.command == "publish-retry")
		if err != nil {
			return nil, err
		}
		request = map[string]any{"credentials": values}
		if args.command == "run" {
			request["op"] = "submit"
			request["revision"] = args.revision
			request["target"] = args.target
			request["publish"] = args.publish
		} else {
			request["op"] = "publish-retry"
			request["job_id"] = args.jobID
		}
	}
	if _, err := c.wake(args.command == "run" || args.command == "publish-retry"); err != nil {
		return nil, err
	}
	switch args.command {
	case "start":
		if args.keepRunning {
			return c.remote(map[string]any{"op": "maintenance", "enabled": true})
		}
		return map[string]any{"ok": true, "power": "running"}, nil
	case "maintenance":
		return c.remote(map[string]any{"op": "maintenance", "enabled": args.enabled == "on"})
	case "logs":
		if args.follow {
			return c.followLogs(args.jobID, args.after)
		}
		return c.remote(map[string]any{"op": "logs", "job_id": args.jobID, "after": args.after})
	}
	if args.command == "run" && args.branch != "" {
		resolved, err := c.remote(map[string]any{"op": "resolve", "branch": args.branch})
		if err != nil {
			return nil, err
		}
		revision, ok := resolved["revision"].(string)
		if resolved["branch"] != args.branch || !ok || !revisionPattern.MatchString(revision) {
			return nil, controlFail("invalid_builder_response")
		}
		request["revision"] = revision
		request["branch"] = args.branch
		if err := c.emit(resolved); err != nil {
			return nil, err
		}
	}
	result, err := c.remote(request)
	if err != nil {
		return nil, err
	}
	identifier, valid := result["job_id"].(string)
	if !valid || !jobPattern.MatchString(identifier) {
		return nil, controlFail("invalid_builder_response")
	}
	if args.detach {
		return result, nil
	}
	if err := c.emit(result); err != nil {
		return nil, err
	}
	state, err := c.followLogs(identifier, 0)
	if err != nil {
		return nil, err
	}
	if state["phase"] != "succeeded" {
		if err := c.emit(state); err != nil {
			return nil, err
		}
		return nil, controlFail("job_failed")
	}
	return state, nil
}

func controllerInteger(value any) (int64, bool) { return num(value) }
func controllerValidBranch(value string) bool   { _, err := validateBranch(value); return err == nil }
func controllerTerminal(value any) bool         { text, ok := value.(string); return ok && terminalSet[text] }
