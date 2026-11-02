package protocol

type ProgressEvent struct {
	Phase string `json:"phase"`
	Code  string `json:"code"`
	Time  int64  `json:"time"`
}

func ValidJobPhase(value string) bool { return phaseSet[value] }

func ValidResultCode(value string) bool { return codeSet[value] }

func ValidImageDigest(value string) bool { return SHAPattern.MatchString(value) }

func ProgressEventsFromValue(value any) ([]ProgressEvent, error) {
	items, ok := value.([]any)
	if !ok || len(items) > 100 {
		return nil, Fail("invalid_state")
	}
	events := make([]ProgressEvent, 0, len(items))
	for _, item := range items {
		data, ok := item.(map[string]any)
		if !ok {
			return nil, Fail("invalid_state")
		}
		event := ProgressEvent{Phase: String(data, "phase"), Code: String(data, "code")}
		stamp, valid := Int(data["time"])
		if !ValidJobPhase(event.Phase) || !ValidResultCode(event.Code) || !valid {
			return nil, Fail("invalid_state")
		}
		event.Time = stamp
		events = append(events, event)
	}
	return events, nil
}

func ProgressEventsObject(events []ProgressEvent) []any {
	items := make([]any, 0, len(events))
	for _, event := range events {
		items = append(items, Object{"phase": event.Phase, "code": event.Code, "time": event.Time})
	}
	return items
}
