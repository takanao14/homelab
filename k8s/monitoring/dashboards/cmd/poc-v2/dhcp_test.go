package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestDhcpClassicParity(t *testing.T) {
	d, err := buildDhcpLeases()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDashboard(d); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../charts/dashboards/dashboards/dhcp-leases.json")
	if err != nil {
		t.Fatal(err)
	}
	var classic struct {
		Panels []map[string]any `json:"panels"`
	}
	if err := json.Unmarshal(data, &classic); err != nil {
		t.Fatal(err)
	}
	panels := map[string]any{}
	for _, element := range d.Elements {
		panels[element.PanelKind.Spec.Title] = element.PanelKind.Spec
	}
	panelCount, rowCount := 0, 0
	for _, expected := range classic.Panels {
		if expected["type"] == "row" {
			rowCount++
			continue
		}
		panelCount++
		title := expected["title"].(string)
		actualValue, ok := panels[title]
		if !ok {
			t.Fatalf("missing panel %s", title)
		}
		actual := jsonObject(t, actualValue)
		viz := actual["vizConfig"].(map[string]any)
		if viz["group"] != expected["type"] {
			t.Errorf("%s: visualization changed", title)
		}
		spec := viz["spec"].(map[string]any)
		for _, key := range []string{"options", "fieldConfig"} {
			want := expected[key]
			if key == "fieldConfig" && want == nil {
				want = map[string]any{"defaults": map[string]any{}, "overrides": []any{}}
			}
			if !reflect.DeepEqual(want, spec[key]) {
				t.Errorf("%s: %s changed", title, key)
			}
		}
	}
	if panelCount != len(d.Elements) || rowCount != len(d.Layout.RowsLayoutKind.Spec.Rows) {
		t.Fatal("panel or row count changed")
	}
	if len(d.Variables) != 4 || d.Variables[0].DatasourceVariableKind == nil || d.Variables[1].DatasourceVariableKind == nil || d.Variables[2].QueryVariableKind == nil || d.Variables[3].QueryVariableKind == nil {
		t.Fatal("dashboard variables changed")
	}
	if d.Variables[0].DatasourceVariableKind.Spec.PluginId != "prometheus" || d.Variables[1].DatasourceVariableKind.Spec.PluginId != "loki" {
		t.Fatal("datasource variables changed")
	}
	for _, variable := range d.Variables[2:] {
		spec := variable.QueryVariableKind.Spec
		if !spec.Multi || !spec.IncludeAll || spec.AllValue == nil || *spec.AllValue != ".+" || spec.Query.Group != "prometheus" {
			t.Fatalf("%s: query variable changed", spec.Name)
		}
	}
	tablePanel := d.Elements["panel-7"].PanelKind.Spec
	if len(tablePanel.Data.Spec.Transformations) != 2 || tablePanel.Data.Spec.Transformations[0].Group != "organize" || tablePanel.Data.Spec.Transformations[1].Group != "sortBy" {
		t.Fatal("table transformations changed")
	}
	if d.Elements["panel-12"].PanelKind.Spec.Data.Spec.Queries[0].Spec.Query.Group != "loki" || d.Elements["panel-13"].PanelKind.Spec.Data.Spec.Queries[0].Spec.Query.Group != "loki" {
		t.Fatal("Loki queries changed")
	}
}
