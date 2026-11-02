package controller

import (
	"regexp"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func safeResponse(data map[string]any) (map[string]any, error) {
	ok, valid := data["ok"].(bool)
	if !valid {
		return nil, protocol.Fail("invalid_builder_response")
	}
	if !ok {
		code, valid := data["error"].(string)
		if !valid || !protocol.ValidResultCode(code) {
			return nil, protocol.Fail("invalid_builder_response")
		}
		return nil, protocol.Fail(code)
	}
	result := map[string]any{"ok": true}
	for _, key := range []string{"maintenance", "publish", "finished", "truncated", "has_more"} {
		if raw, exists := data[key]; exists {
			value, valid := raw.(bool)
			if !valid {
				return nil, protocol.Fail("invalid_builder_response")
			}
			result[key] = value
		}
	}
	if raw, exists := data["branch"]; exists {
		branch, err := protocol.ValidateBranch(raw)
		if err != nil {
			return nil, protocol.Fail("invalid_builder_response")
		}
		result["branch"] = branch
	}
	for _, rule := range []struct {
		key     string
		pattern *regexp.Regexp
	}{
		{"job_id", protocol.JobPattern}, {"revision", protocol.RevisionPattern}, {"target", protocol.TargetPattern}, {"sha256", protocol.SHAPattern},
	} {
		if raw, exists := data[rule.key]; exists {
			value, valid := raw.(string)
			if !valid || !rule.pattern.MatchString(value) {
				return nil, protocol.Fail("invalid_builder_response")
			}
			result[rule.key] = value
		}
	}
	if raw, exists := data["phase"]; exists {
		value, valid := raw.(string)
		if !valid || (!protocol.ValidJobPhase(value) && value != "idle" && value != "ready" && value != "stopping") {
			return nil, protocol.Fail("invalid_builder_response")
		}
		result["phase"] = value
	}
	if raw, exists := data["code"]; exists {
		value, valid := raw.(string)
		if !valid || !protocol.ValidResultCode(value) {
			return nil, protocol.Fail("invalid_builder_response")
		}
		result["code"] = value
	}
	if raw, exists := data["created"]; exists {
		value, valid := protocol.Int(raw)
		if !valid {
			return nil, protocol.Fail("invalid_builder_response")
		}
		result["created"] = value
	}
	if raw, exists := data["events"]; exists {
		events, err := protocol.ProgressEventsFromValue(raw)
		if err != nil {
			return nil, protocol.Fail("invalid_builder_response")
		}
		result["events"] = protocol.ProgressEventsObject(events)
	}
	if raw, exists := data["cursor"]; exists {
		value, valid := protocol.Int(raw)
		if !valid || value < 0 {
			return nil, protocol.Fail("invalid_builder_response")
		}
		result["cursor"] = value
	}
	if raw, exists := data["records"]; exists {
		records, valid := raw.([]any)
		if !valid || len(records) > 256 {
			return nil, protocol.Fail("invalid_builder_response")
		}
		projected := []any{}
		for _, item := range records {
			record, valid := item.(map[string]any)
			if !valid {
				return nil, protocol.Fail("invalid_builder_response")
			}
			safe, err := protocol.ValidateLogRecord(record)
			if err != nil {
				return nil, protocol.Fail("invalid_builder_response")
			}
			projected = append(projected, safe)
		}
		result["records"] = projected
	}
	return result, nil
}
