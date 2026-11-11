package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestDnsOverviewClassicQueries(t *testing.T) {
	d, err := buildDnsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDashboard(d); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../charts/dashboards/dashboards/dns-overview.json")
	if err != nil {
		t.Fatal(err)
	}
	var classic struct {
		Panels []map[string]any `json:"panels"`
	}
	if err := json.Unmarshal(data, &classic); err != nil {
		t.Fatal(err)
	}
	panels := map[string]map[string]any{}
	for _, p := range classic.Panels {
		panels[p["type"].(string)+":"+p["title"].(string)] = p
	}
	for _, element := range d.Elements {
		actual := jsonObject(t, element.PanelKind.Spec)
		viz := actual["vizConfig"].(map[string]any)
		kind := viz["group"].(string)
		key := kind + ":" + actual["title"].(string)
		expected, ok := panels[key]
		if !ok {
			t.Fatalf("no Classic reference for %s", key)
		}
		gotFields := viz["spec"].(map[string]any)["fieldConfig"].(map[string]any)
		wantFields := expected["fieldConfig"].(map[string]any)
		gotDefaults := gotFields["defaults"].(map[string]any)
		wantDefaults := wantFields["defaults"].(map[string]any)
		for _, field := range []string{"unit", "decimals", "thresholds"} {
			if !reflect.DeepEqual(gotDefaults[field], wantDefaults[field]) {
				t.Errorf("%s: %s changed", key, field)
			}
		}
		if !reflect.DeepEqual(gotFields["overrides"], wantFields["overrides"]) {
			t.Errorf("%s: series overrides changed", key)
		}
		queries := actual["data"].(map[string]any)["spec"].(map[string]any)["queries"].([]any)
		targets := expected["targets"].([]any)
		if len(queries) != len(targets) {
			t.Fatalf("%s: query count changed", key)
		}
		for i, query := range queries {
			spec := query.(map[string]any)["spec"].(map[string]any)
			got := spec["query"].(map[string]any)["spec"].(map[string]any)
			want := targets[i].(map[string]any)
			for _, field := range []string{"expr", "legendFormat"} {
				if !reflect.DeepEqual(got[field], want[field]) {
					t.Errorf("%s: query %d %s changed", key, i, field)
				}
			}
			if want["refId"] != nil && spec["refId"] != want["refId"] {
				t.Errorf("%s: query reference changed", key)
			}
			if got["instant"] != (kind == "stat") || got["range"] != (kind != "stat") {
				t.Errorf("%s: wrong query evaluation mode", key)
			}
		}
	}
}
