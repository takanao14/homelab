package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
)

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestClassicParity(t *testing.T) {
	d, err := buildUptime()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDashboard(d); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../charts/dashboards/dashboards/uptime.json")
	if err != nil {
		t.Fatal(err)
	}
	var classic struct {
		Panels []map[string]any `json:"panels"`
	}
	if err := json.Unmarshal(data, &classic); err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Time struct {
			From string
			To   string
		} `json:"time"`
		Timezone string `json:"timezone"`
		Refresh  string `json:"refresh"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if d.TimeSettings.From != settings.Time.From || d.TimeSettings.To != settings.Time.To || d.TimeSettings.AutoRefresh != settings.Refresh || d.TimeSettings.Timezone == nil || *d.TimeSettings.Timezone != settings.Timezone {
		t.Fatal("time settings changed")
	}
	if d.CursorSync != dashboardv2.DashboardCursorSyncCrosshair {
		t.Fatal("cursor sync changed")
	}
	if len(d.Variables) != 1 || d.Variables[0].DatasourceVariableKind == nil {
		t.Fatal("datasource variable missing")
	}
	variable := d.Variables[0].DatasourceVariableKind.Spec
	if variable.Name != "datasource" || variable.PluginId != "prometheus" {
		t.Fatal("datasource variable changed")
	}
	panels := map[string]dashboardv2.PanelSpec{}
	for _, e := range d.Elements {
		panels[e.PanelKind.Spec.Title] = e.PanelKind.Spec
	}
	rows := d.Layout.RowsLayoutKind.Spec.Rows
	rowIndex, itemIndex, count := -1, 0, 0
	var rowY float64
	for _, p := range classic.Panels {
		pos := p["gridPos"].(map[string]any)
		if p["type"] == "row" {
			rowIndex++
			itemIndex = 0
			rowY = pos["y"].(float64)
			if rowIndex >= len(rows) || (rows[rowIndex].Spec.Title == nil || *rows[rowIndex].Spec.Title != p["title"]) {
				t.Fatal("row mismatch")
			}
			continue
		}
		count++
		title := p["title"].(string)
		n, ok := panels[title]
		if !ok {
			t.Fatalf("missing panel %s", title)
		}
		if n.VizConfig.Group != p["type"] {
			t.Errorf("%s: visualization changed", title)
		}
		for key, actual := range map[string]any{"options": n.VizConfig.Spec.Options, "fieldConfig": n.VizConfig.Spec.FieldConfig} {
			if !reflect.DeepEqual(p[key], jsonObject(t, actual)) {
				t.Errorf("%s: %s changed", title, key)
			}
		}
		if description, _ := p["description"].(string); description != n.Description {
			t.Errorf("%s: description changed", title)
		}
		targets := p["targets"].([]any)
		if len(targets) != len(n.Data.Spec.Queries) {
			t.Fatalf("%s: query count changed", title)
		}
		for i, target := range targets {
			expected := target.(map[string]any)
			delete(expected, "refId")
			q := n.Data.Spec.Queries[i].Spec.Query
			if !reflect.DeepEqual(expected, jsonObject(t, q.Spec)) {
				t.Errorf("%s: query %d changed", title, i)
			}
			if q.Group != "prometheus" || q.Datasource == nil || q.Datasource.Name == nil || *q.Datasource.Name != "$datasource" {
				t.Errorf("%s: datasource changed", title)
			}
		}
		items := rows[rowIndex].Spec.Layout.GridLayoutKind.Spec.Items
		if itemIndex >= len(items) {
			t.Fatal("missing grid item")
		}
		item := items[itemIndex].Spec
		itemIndex++
		if d.Elements[item.Element.Name].PanelKind.Spec.Title != title || float64(item.X) != pos["x"] || float64(item.Y) != pos["y"].(float64)-rowY-1 || float64(item.Width) != pos["w"] || float64(item.Height) != pos["h"] {
			t.Errorf("%s: layout changed", title)
		}
	}
	if count != len(panels) || rowIndex+1 != len(rows) {
		t.Fatal("panel or row count changed")
	}
}

func TestValidationRejectsBrokenDashboard(t *testing.T) {
	cases := map[string]func(*dashboardv2.Dashboard){
		"empty": func(d *dashboardv2.Dashboard) { d.Elements = nil },
		"missing reference": func(d *dashboardv2.Dashboard) {
			d.Layout.RowsLayoutKind.Spec.Rows[0].Spec.Layout.GridLayoutKind.Spec.Items[0].Spec.Element.Name = "missing"
		},
		"duplicate ID": func(d *dashboardv2.Dashboard) {
			d.Elements["panel-3"].PanelKind.Spec.Id = d.Elements["panel-2"].PanelKind.Spec.Id
		},
		"missing thresholds": func(d *dashboardv2.Dashboard) {
			d.Elements["panel-2"].PanelKind.Spec.VizConfig.Spec.FieldConfig.Defaults.Thresholds = nil
		},
		"missing query reference": func(d *dashboardv2.Dashboard) {
			d.Elements["panel-2"].PanelKind.Spec.Data.Spec.Queries[0].Spec.RefId = ""
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := buildUptime()
			if err != nil {
				t.Fatal(err)
			}
			mutate(d)
			if validateDashboard(d) == nil {
				t.Fatal("invalid dashboard accepted")
			}
		})
	}
}
