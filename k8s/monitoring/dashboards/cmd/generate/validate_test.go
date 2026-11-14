package main

import (
	"testing"

	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
)

func TestDashboardsValidate(t *testing.T) {
	t.Chdir("../..")
	seen := map[string]bool{}
	for _, definition := range dashboards {
		if seen[definition.name] || seen["uid:"+definition.uid] {
			t.Fatalf("%s: duplicate name or UID", definition.name)
		}
		seen[definition.name], seen["uid:"+definition.uid] = true, true
		d, err := definition.build()
		if err != nil {
			t.Fatalf("%s: %v", definition.name, err)
		}
		if err := validateDashboard(d); err != nil {
			t.Errorf("%s: %v", definition.name, err)
		}
	}
}

func TestValidationRejectsBrokenDashboard(t *testing.T) {
	cases := map[string]func(*dashboardv2.Dashboard){
		"empty": func(d *dashboardv2.Dashboard) { d.Elements = nil },
		"missing reference": func(d *dashboardv2.Dashboard) {
			d.Layout.RowsLayoutKind.Spec.Rows[0].Spec.Layout.AutoGridLayoutKind.Spec.Items[0].Spec.Element.Name = "missing"
		},
		"duplicate ID": func(d *dashboardv2.Dashboard) {
			d.Elements["dns-devices-down"].PanelKind.Spec.Id = d.Elements["icmp-devices-down"].PanelKind.Spec.Id
		},
		"missing thresholds": func(d *dashboardv2.Dashboard) {
			d.Elements["icmp-devices-down"].PanelKind.Spec.VizConfig.Spec.FieldConfig.Defaults.Thresholds = nil
		},
		"missing query reference": func(d *dashboardv2.Dashboard) {
			d.Elements["icmp-devices-down"].PanelKind.Spec.Data.Spec.Queries[0].Spec.RefId = ""
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

func TestValidationQueryOverrides(t *testing.T) {
	for _, tc := range []struct {
		name    string
		option  any
		wantErr bool
	}{
		{name: "SDK reference", option: new("query-0")},
		{name: "string reference", option: "query-0"},
		{name: "missing reference", option: new("missing"), wantErr: true},
		{name: "invalid reference", option: 42, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := buildUptime()
			if err != nil {
				t.Fatal(err)
			}
			p := d.Elements["icmp-devices-down"].PanelKind
			p.Spec.VizConfig.Spec.FieldConfig.Overrides = append(p.Spec.VizConfig.Spec.FieldConfig.Overrides,
				dashboardv2.Dashboardv2FieldConfigSourceOverrides{
					Matcher: dashboardv2.MatcherConfig{Id: "byFrameRefID", Options: tc.option},
				})
			if err := validateDashboard(d); (err != nil) != tc.wantErr {
				t.Errorf("validateDashboard error = %v, want error: %v", err, tc.wantErr)
			}
		})
	}
}
