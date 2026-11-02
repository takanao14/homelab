package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJobModelsPreserveWireShapeAndIntegerPrecision(t *testing.T) {
	state := ControlState{IdleSince: 9223372036854775807}
	raw, err := json.Marshal(state.Object())
	if err != nil {
		t.Fatal("state serialization failed")
	}
	data, err := decodeObject(raw)
	if err != nil {
		t.Fatal("state decoding failed")
	}
	got, err := controlStateFromObject(data)
	if err != nil || got != state || data["active"] != nil {
		t.Fatal("state shape changed")
	}
	record := JobRecord{Revision: strings.Repeat("a", 40), Target: "ubuntu26-base", Created: 9223372036854775807, Finished: true, Result: &JobResult{Phase: "succeeded", Code: "verified"}}
	path := filepath.Join(t.TempDir(), "job.json")
	if writeJobRecord(path, record) != nil {
		t.Fatal("record save failed")
	}
	actual, err := readJobRecord(path)
	if err != nil || !reflect.DeepEqual(actual, record) {
		t.Fatal("record round trip changed")
	}
	progress, err := progressFromObject(Object{"phase": "building", "code": "building", "secret": "SECRET_MARKER", "events": []any{Object{"phase": "preparing", "code": "accepted", "time": json.Number("9223372036854775807"), "secret": "SECRET_MARKER"}}})
	if err != nil || progress.Events[0].Time != 9223372036854775807 {
		t.Fatal("progress decoding failed")
	}
	raw, err = json.Marshal(progress.Object())
	if err != nil || strings.Contains(string(raw), "SECRET_MARKER") {
		t.Fatal("private progress data escaped")
	}
}

func TestJobModelsRejectMissingNullAndInvalidScalars(t *testing.T) {
	for _, field := range []string{"maintenance", "stopping", "idle_since"} {
		for _, omit := range []bool{true, false} {
			data := ControlState{}.Object()
			if omit {
				delete(data, field)
			} else {
				data[field] = nil
			}
			if _, err := controlStateFromObject(data); err == nil {
				t.Fatal("invalid control field accepted")
			}
		}
	}
	valid := Object{"revision": strings.Repeat("a", 40), "target": "ubuntu26-base", "publish": false, "created": int64(0), "finished": false}
	for _, field := range []string{"publish", "created", "finished", "revision", "target"} {
		for _, omit := range []bool{true, false} {
			data := Object{}
			for k, v := range valid {
				data[k] = v
			}
			if omit {
				delete(data, field)
			} else {
				data[field] = nil
			}
			if _, err := jobRecordFromObject(data); err == nil {
				t.Fatal("invalid record field accepted")
			}
		}
	}
	for _, field := range []string{"branch", "result"} {
		data := Object{}
		for k, v := range valid {
			data[k] = v
		}
		data[field] = nil
		if _, err := jobRecordFromObject(data); err == nil {
			t.Fatal("explicit null optional field accepted")
		}
	}
	for _, value := range []any{nil, json.Number("1.0"), json.Number("1e2"), json.Number("9223372036854775808")} {
		if _, err := progressFromObject(Object{"phase": "building", "code": "building", "events": []any{Object{"phase": "building", "code": "building", "time": value}}}); err == nil {
			t.Fatal("invalid event integer accepted")
		}
	}
	if _, err := progressFromObject(Object{"phase": "building", "code": "building", "sha256": nil}); err == nil {
		t.Fatal("null digest accepted")
	}
	if _, err := progressFromObject(Object{"phase": "building", "code": "building", "events": nil}); err == nil {
		t.Fatal("null events accepted")
	}
}
