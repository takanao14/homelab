package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestCertManagerClassicParity(t *testing.T) {
	d, err := buildCertManager()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDashboard(d); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../charts/dashboards/dashboards/cert-manager-overview.json")
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
	for _, element := range d.Elements {
		panels[element.PanelKind.Spec.Title] = jsonObject(t, element.PanelKind.Spec)
	}
	panelCount, rowIndex, itemIndex := 0, -1, 0
	var rowY float64
	rows := d.Layout.RowsLayoutKind.Spec.Rows
	for _, expected := range classic.Panels {
		pos := expected["gridPos"].(map[string]any)
		if expected["type"] == "row" {
			rowIndex++
			itemIndex = 0
			rowY = pos["y"].(float64)
			if rows[rowIndex].Spec.Title == nil || *rows[rowIndex].Spec.Title != expected["title"] {
				t.Fatal("row mismatch")
			}
			continue
		}
		panelCount++
		title := expected["title"].(string)
		actual, ok := panels[title]
		if !ok {
			t.Fatalf("missing panel %s", title)
		}
		viz := actual["vizConfig"].(map[string]any)
		if viz["group"] != expected["type"] {
			t.Errorf("%s: visualization changed", title)
		}
		vizSpec := viz["spec"].(map[string]any)
		for _, key := range []string{"options", "fieldConfig"} {
			if !reflect.DeepEqual(expected[key], vizSpec[key]) {
				t.Errorf("%s: %s changed", title, key)
			}
		}
		if description, _ := expected["description"].(string); description != actual["description"] {
			t.Errorf("%s: description changed", title)
		}
		dataSpec := actual["data"].(map[string]any)["spec"].(map[string]any)
		queries := dataSpec["queries"].([]any)
		targets := expected["targets"].([]any)
		if len(queries) != len(targets) {
			t.Fatalf("%s: query count changed", title)
		}
		for i := range targets {
			want := targets[i].(map[string]any)
			delete(want, "refId")
			query := queries[i].(map[string]any)["spec"].(map[string]any)["query"].(map[string]any)
			if !reflect.DeepEqual(want, query["spec"]) {
				t.Errorf("%s: query %d changed", title, i)
			}
		}
		wantTransforms, _ := expected["transformations"].([]any)
		gotTransforms, _ := dataSpec["transformations"].([]any)
		if len(wantTransforms) != len(gotTransforms) {
			t.Errorf("%s: transformation count changed", title)
		} else {
			for i, wantValue := range wantTransforms {
				want := wantValue.(map[string]any)
				got := gotTransforms[i].(map[string]any)
				if want["id"] != got["group"] || !reflect.DeepEqual(want["options"], got["spec"].(map[string]any)["options"]) {
					t.Errorf("%s: transformation %d changed", title, i)
				}
			}
		}
		item := rows[rowIndex].Spec.Layout.GridLayoutKind.Spec.Items[itemIndex].Spec
		itemIndex++
		if d.Elements[item.Element.Name].PanelKind.Spec.Title != title || float64(item.X) != pos["x"] || float64(item.Y) != pos["y"].(float64)-rowY-1 || float64(item.Width) != pos["w"] || float64(item.Height) != pos["h"] {
			t.Errorf("%s: layout changed", title)
		}
	}
	if panelCount != len(d.Elements) || rowIndex+1 != len(rows) {
		t.Fatal("panel or row count changed")
	}
	if len(d.Variables) != 2 || d.Variables[1].QueryVariableKind == nil {
		t.Fatal("cluster variable missing")
	}
	cluster := d.Variables[1].QueryVariableKind.Spec
	if cluster.Name != "cluster" || !cluster.Multi || !cluster.IncludeAll || cluster.Query.Group != "prometheus" {
		t.Fatal("cluster variable changed")
	}
}
