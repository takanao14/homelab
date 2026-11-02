package main

import "regexp"

// PublicJobStatus cannot carry credentials or private control state.
type PublicJobStatus struct {
	JobID    string `json:"job_id"`
	Revision string `json:"revision"`
	Target   string `json:"target"`
	Branch   string `json:"branch,omitempty"`
	JobResult
	Publish  bool  `json:"publish"`
	Created  int64 `json:"created"`
	Finished bool  `json:"finished"`
}

func publicJobStatus(id string, record JobRecord, result JobResult) Object {
	status := PublicJobStatus{JobID: id, Revision: record.Revision, Target: record.Target, Branch: record.Branch,
		JobResult: result, Publish: record.Publish, Created: record.Created, Finished: record.Finished}
	return status.Object()
}
func (status PublicJobStatus) Object() Object {
	data := status.JobResult.Object()
	data["ok"], data["job_id"], data["revision"], data["target"] = true, status.JobID, status.Revision, status.Target
	data["publish"], data["created"], data["finished"] = status.Publish, status.Created, status.Finished
	if status.Branch != "" {
		data["branch"] = status.Branch
	}
	return data
}

func controllerSafeResponse(data map[string]any) (map[string]any, error) {
	ok, valid := data["ok"].(bool)
	if !valid {
		return nil, controlFail("invalid_builder_response")
	}
	if !ok {
		code, valid := data["error"].(string)
		if !valid || !validResultCode(code) {
			return nil, controlFail("invalid_builder_response")
		}
		return nil, controlFail(code)
	}
	result := map[string]any{"ok": true}
	for _, key := range []string{"maintenance", "publish", "finished", "truncated", "has_more"} {
		if raw, exists := data[key]; exists {
			value, valid := raw.(bool)
			if !valid {
				return nil, controlFail("invalid_builder_response")
			}
			result[key] = value
		}
	}
	if raw, exists := data["branch"]; exists {
		branch, err := validateBranch(raw)
		if err != nil {
			return nil, controlFail("invalid_builder_response")
		}
		result["branch"] = branch
	}
	for _, rule := range []struct {
		key     string
		pattern *regexp.Regexp
	}{
		{"job_id", jobPattern}, {"revision", revisionPattern}, {"target", targetPattern}, {"sha256", shaPattern},
	} {
		if raw, exists := data[rule.key]; exists {
			value, valid := raw.(string)
			if !valid || !rule.pattern.MatchString(value) {
				return nil, controlFail("invalid_builder_response")
			}
			result[rule.key] = value
		}
	}
	if raw, exists := data["phase"]; exists {
		value, valid := raw.(string)
		if !valid || (!validJobPhase(value) && value != "idle" && value != "ready" && value != "stopping") {
			return nil, controlFail("invalid_builder_response")
		}
		result["phase"] = value
	}
	if raw, exists := data["code"]; exists {
		value, valid := raw.(string)
		if !valid || !validResultCode(value) {
			return nil, controlFail("invalid_builder_response")
		}
		result["code"] = value
	}
	if raw, exists := data["created"]; exists {
		value, valid := controllerInteger(raw)
		if !valid {
			return nil, controlFail("invalid_builder_response")
		}
		result["created"] = value
	}
	if raw, exists := data["events"]; exists {
		events, err := progressEventsFromValue(raw)
		if err != nil {
			return nil, controlFail("invalid_builder_response")
		}
		result["events"] = progressEventsObject(events)
	}
	if raw, exists := data["cursor"]; exists {
		value, valid := controllerInteger(raw)
		if !valid || value < 0 {
			return nil, controlFail("invalid_builder_response")
		}
		result["cursor"] = value
	}
	if raw, exists := data["records"]; exists {
		records, valid := raw.([]any)
		if !valid || len(records) > 256 {
			return nil, controlFail("invalid_builder_response")
		}
		projected := []any{}
		for _, item := range records {
			record, valid := item.(map[string]any)
			if !valid {
				return nil, controlFail("invalid_builder_response")
			}
			safe, err := validateLogRecord(record)
			if err != nil {
				return nil, controlFail("invalid_builder_response")
			}
			projected = append(projected, safe)
		}
		result["records"] = projected
	}
	return result, nil
}
