package main

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

type ProgressEvent struct {
	Phase string `json:"phase"`
	Code  string `json:"code"`
	Time  int64  `json:"time"`
}

type Progress struct {
	JobResult
	Events []ProgressEvent
}

func controlStateFromObject(data Object) (ControlState, error) {
	active := s(data, "active")
	maintenance, mok := data["maintenance"].(bool)
	stopping, sok := data["stopping"].(bool)
	idle, iok := num(data["idle_since"])
	if (data["active"] != nil && !validJobID(active)) || !mok || !sok || !iok {
		return ControlState{}, fail("invalid_state")
	}
	return ControlState{active, maintenance, stopping, idle}, nil
}
func (state ControlState) Object() Object {
	var active any
	if state.Active != "" {
		active = state.Active
	}
	return Object{"active": active, "maintenance": state.Maintenance, "stopping": state.Stopping, "idle_since": state.IdleSince}
}
func validJobPhase(value string) bool    { return phaseSet[value] }
func validResultCode(value string) bool  { return codeSet[value] }
func validImageDigest(value string) bool { return shaPattern.MatchString(value) }

func (result JobResult) Validate() error {
	if !validJobPhase(result.Phase) || !validResultCode(result.Code) || (result.SHA256 != "" && !validImageDigest(result.SHA256)) {
		return fail("invalid_state")
	}
	return nil
}
func jobResultFromObject(data Object) (JobResult, error) {
	result := JobResult{Phase: s(data, "phase"), Code: s(data, "code")}
	if raw, exists := data["sha256"]; exists {
		value, ok := raw.(string)
		if !ok || !validImageDigest(value) {
			return JobResult{}, fail("invalid_state")
		}
		result.SHA256 = value
	}
	return result, result.Validate()
}
func (result JobResult) Object() Object {
	data := Object{"phase": result.Phase, "code": result.Code}
	if result.SHA256 != "" {
		data["sha256"] = result.SHA256
	}
	return data
}
func progressEventsFromValue(value any) ([]ProgressEvent, error) {
	items, ok := value.([]any)
	if !ok || len(items) > 100 {
		return nil, fail("invalid_state")
	}
	events := make([]ProgressEvent, 0, len(items))
	for _, item := range items {
		data, ok := item.(map[string]any)
		if !ok {
			return nil, fail("invalid_state")
		}
		event := ProgressEvent{Phase: s(data, "phase"), Code: s(data, "code")}
		stamp, valid := num(data["time"])
		if !validJobPhase(event.Phase) || !validResultCode(event.Code) || !valid {
			return nil, fail("invalid_state")
		}
		event.Time = stamp
		events = append(events, event)
	}
	return events, nil
}
func progressEventsObject(events []ProgressEvent) []any {
	items := make([]any, 0, len(events))
	for _, event := range events {
		items = append(items, Object{"phase": event.Phase, "code": event.Code, "time": event.Time})
	}
	return items
}
func progressFromObject(data Object) (Progress, error) {
	result, err := jobResultFromObject(data)
	if err != nil {
		return Progress{}, err
	}
	progress := Progress{JobResult: result, Events: []ProgressEvent{}}
	if value, exists := data["events"]; exists {
		progress.Events, err = progressEventsFromValue(value)
	}
	return progress, err
}
func (progress Progress) Object() Object {
	data := progress.JobResult.Object()
	data["events"] = progressEventsObject(progress.Events)
	return data
}
func jobRecordFromObject(data Object) (JobRecord, error) {
	record := JobRecord{Revision: s(data, "revision"), Target: s(data, "target")}
	var pok, fok, cok bool
	record.Publish, pok = data["publish"].(bool)
	record.Finished, fok = data["finished"].(bool)
	record.Created, cok = num(data["created"])
	if !revisionPattern.MatchString(record.Revision) || !targetPattern.MatchString(record.Target) || !pok || !fok || !cok {
		return JobRecord{}, fail("invalid_state")
	}
	if value, exists := data["branch"]; exists {
		branch, err := validateBranch(value)
		if err != nil {
			return JobRecord{}, fail("invalid_state")
		}
		record.Branch = branch
	}
	if value, exists := data["result"]; exists {
		object, ok := value.(map[string]any)
		if !ok {
			return JobRecord{}, fail("invalid_state")
		}
		result, err := jobResultFromObject(object)
		if err != nil {
			return JobRecord{}, err
		}
		record.Result = &result
	}
	if record.Finished && (record.Result == nil || !terminalSet[record.Result.Phase]) {
		return JobRecord{}, fail("invalid_state")
	}
	return record, nil
}
func readJobRecord(path string) (JobRecord, error) {
	data, err := readJSON(path)
	if err != nil {
		return JobRecord{}, err
	}
	return jobRecordFromObject(data)
}
func writeJobRecord(path string, record JobRecord) error { return writeJSON(path, record, 0600) }
