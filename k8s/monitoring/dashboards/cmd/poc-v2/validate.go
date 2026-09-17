package main

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
)

func validateDashboard(d *dashboardv2.Dashboard) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if len(d.Elements) == 0 {
		return fmt.Errorf("dashboard has no elements")
	}
	ids := map[float64]bool{}
	for name, element := range d.Elements {
		if element.PanelKind == nil {
			return fmt.Errorf("%s: expected a panel", name)
		}
		p := element.PanelKind.Spec
		if p.Id <= 0 || math.Trunc(p.Id) != p.Id || ids[p.Id] {
			return fmt.Errorf("%s: invalid or duplicate panel ID", name)
		}
		ids[p.Id] = true
		data, err := json.Marshal(p.VizConfig.Spec.Options)
		if err != nil {
			return err
		}
		var options struct {
			ColorMode string `json:"colorMode"`
		}
		if err := json.Unmarshal(data, &options); err != nil {
			return err
		}
		thresholds := p.VizConfig.Spec.FieldConfig.Defaults.Thresholds
		if options.ColorMode != "" && options.ColorMode != "none" && (thresholds == nil || len(thresholds.Steps) == 0) {
			return fmt.Errorf("%s: colored panel requires thresholds", name)
		}
		refs := map[string]bool{}
		for _, q := range p.Data.Spec.Queries {
			if q.Spec.RefId == "" || refs[q.Spec.RefId] {
				return fmt.Errorf("%s: missing or duplicate query reference", name)
			}
			refs[q.Spec.RefId] = true
		}
	}
	if d.Layout.RowsLayoutKind == nil {
		return fmt.Errorf("expected rows layout")
	}
	seen := map[string]bool{}
	for _, row := range d.Layout.RowsLayoutKind.Spec.Rows {
		grid := row.Spec.Layout.GridLayoutKind
		if grid == nil {
			return fmt.Errorf("expected grid layout in every row")
		}
		for _, item := range grid.Spec.Items {
			name := item.Spec.Element.Name
			if _, ok := d.Elements[name]; !ok {
				return fmt.Errorf("unknown element %s", name)
			}
			if seen[name] {
				return fmt.Errorf("duplicate layout reference %s", name)
			}
			seen[name] = true
			g := item.Spec
			if g.X < 0 || g.Y < 0 || g.Width <= 0 || g.Height <= 0 || g.X+g.Width > 24 {
				return fmt.Errorf("%s: invalid grid bounds", name)
			}
		}
	}
	if len(seen) != len(d.Elements) {
		return fmt.Errorf("layout omits elements")
	}
	return nil
}
