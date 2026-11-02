package builder

import (
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

// Root-owned records remain separate from worker-owned progress.
type ControlState struct {
	Active                string
	Maintenance, Stopping bool
	IdleSince             int64
}

type JobResult struct {
	Phase  string `json:"phase"`
	Code   string `json:"code"`
	SHA256 string `json:"sha256,omitempty"`
}

type JobRecord struct {
	Revision string     `json:"revision"`
	Target   string     `json:"target"`
	Publish  bool       `json:"publish"`
	Created  int64      `json:"created"`
	Finished bool       `json:"finished"`
	Branch   string     `json:"branch,omitempty"`
	Result   *JobResult `json:"result,omitempty"`
}

type Progress struct {
	JobResult
	Events []protocol.ProgressEvent
}

func controlStateFromObject(data protocol.Object) (ControlState, error) {
	active := protocol.String(data, "active")
	maintenance, mok := data["maintenance"].(bool)
	stopping, sok := data["stopping"].(bool)
	idle, iok := protocol.Int(data["idle_since"])
	if (data["active"] != nil && !protocol.ValidJobID(active)) || !mok || !sok || !iok {
		return ControlState{}, protocol.Fail("invalid_state")
	}
	return ControlState{active, maintenance, stopping, idle}, nil
}

func (state ControlState) Object() protocol.Object {
	var active any
	if state.Active != "" {
		active = state.Active
	}
	return protocol.Object{"active": active, "maintenance": state.Maintenance, "stopping": state.Stopping, "idle_since": state.IdleSince}
}

func (result JobResult) Validate() error {
	if !protocol.ValidJobPhase(result.Phase) || !protocol.ValidResultCode(result.Code) || (result.SHA256 != "" && !protocol.ValidImageDigest(result.SHA256)) {
		return protocol.Fail("invalid_state")
	}
	return nil
}

func jobResultFromObject(data protocol.Object) (JobResult, error) {
	result := JobResult{Phase: protocol.String(data, "phase"), Code: protocol.String(data, "code")}
	if raw, exists := data["sha256"]; exists {
		value, ok := raw.(string)
		if !ok || !protocol.ValidImageDigest(value) {
			return JobResult{}, protocol.Fail("invalid_state")
		}
		result.SHA256 = value
	}
	return result, result.Validate()
}

func (result JobResult) Object() protocol.Object {
	data := protocol.Object{"phase": result.Phase, "code": result.Code}
	if result.SHA256 != "" {
		data["sha256"] = result.SHA256
	}
	return data
}

func progressFromObject(data protocol.Object) (Progress, error) {
	result, err := jobResultFromObject(data)
	if err != nil {
		return Progress{}, err
	}
	progress := Progress{JobResult: result, Events: []protocol.ProgressEvent{}}
	if value, exists := data["events"]; exists {
		progress.Events, err = protocol.ProgressEventsFromValue(value)
	}
	return progress, err
}

func (progress Progress) Object() protocol.Object {
	data := progress.JobResult.Object()
	data["events"] = protocol.ProgressEventsObject(progress.Events)
	return data
}

func jobRecordFromObject(data protocol.Object) (JobRecord, error) {
	record := JobRecord{Revision: protocol.String(data, "revision"), Target: protocol.String(data, "target")}
	var pok, fok, cok bool
	record.Publish, pok = data["publish"].(bool)
	record.Finished, fok = data["finished"].(bool)
	record.Created, cok = protocol.Int(data["created"])
	if !protocol.RevisionPattern.MatchString(record.Revision) || !protocol.TargetPattern.MatchString(record.Target) || !pok || !fok || !cok {
		return JobRecord{}, protocol.Fail("invalid_state")
	}
	if value, exists := data["branch"]; exists {
		branch, err := protocol.ValidateBranch(value)
		if err != nil {
			return JobRecord{}, protocol.Fail("invalid_state")
		}
		record.Branch = branch
	}
	if value, exists := data["result"]; exists {
		object, ok := value.(map[string]any)
		if !ok {
			return JobRecord{}, protocol.Fail("invalid_state")
		}
		result, err := jobResultFromObject(object)
		if err != nil {
			return JobRecord{}, err
		}
		record.Result = &result
	}
	if record.Finished && (record.Result == nil || !protocol.TerminalSet[record.Result.Phase]) {
		return JobRecord{}, protocol.Fail("invalid_state")
	}
	return record, nil
}

func readJobRecord(path string) (JobRecord, error) {
	data, err := store.ReadJSON(path)
	if err != nil {
		return JobRecord{}, err
	}
	return jobRecordFromObject(data)
}

func writeJobRecord(path string, record JobRecord) error { return store.WriteJSON(path, record, 0600) }
