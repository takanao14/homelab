package main

import (
	"reflect"
	"testing"

	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
)

func panel(title string) *dashboardv2.PanelBuilder {
	return dashboardv2.NewPanelBuilder().Title(title)
}

func TestLayoutRowsAndNames(t *testing.T) {
	d, err := newLayout(dashboardv2.NewDashboardBuilder("test")).
		Row("First", statGrid(4)).
		Panel(42, panel("Devices Down")).
		Panel(7, panel("Availability (range)")).
		Row("Second", chartGrid()).
		Panel(3, panel("p99 Latency / s")).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	rows := d.Layout.RowsLayoutKind.Spec.Rows
	want := [][]string{{"devices-down", "availability-range"}, {"p99-latency-s"}}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	ids := map[string]float64{"devices-down": 42, "availability-range": 7, "p99-latency-s": 3}
	for r, row := range rows {
		grid := row.Spec.Layout.AutoGridLayoutKind
		if grid == nil {
			t.Fatalf("row %d: expected an auto grid", r)
		}
		var names []string
		for _, item := range grid.Spec.Items {
			names = append(names, item.Spec.Element.Name)
			id := ids[item.Spec.Element.Name]
			if got := d.Elements[item.Spec.Element.Name].PanelKind.Spec.Id; got != id {
				t.Errorf("%s: id %v, want %v", item.Spec.Element.Name, got, id)
			}
		}
		if !reflect.DeepEqual(names, want[r]) {
			t.Errorf("row %d: got %v, want %v", r, names, want[r])
		}
	}
	if got := *rows[0].Spec.Layout.AutoGridLayoutKind.Spec.MaxColumnCount; got != 4 {
		t.Errorf("first row: max columns %v, want 4", got)
	}
}

func TestLayoutRejectsInvalidPanels(t *testing.T) {
	cases := map[string]func(*dashboardLayout) *dashboardLayout{
		"panel before row": func(l *dashboardLayout) *dashboardLayout { return l.Panel(1, panel("a")) },
		"empty title":      func(l *dashboardLayout) *dashboardLayout { return l.Row("r", chartGrid()).Panel(1, panel("")) },
		"zero ID":          func(l *dashboardLayout) *dashboardLayout { return l.Row("r", chartGrid()).Panel(0, panel("a")) },
		"negative ID":      func(l *dashboardLayout) *dashboardLayout { return l.Row("r", chartGrid()).Panel(-1, panel("a")) },
		"duplicate ID": func(l *dashboardLayout) *dashboardLayout {
			return l.Row("r", chartGrid()).Panel(1, panel("a")).Panel(1, panel("b"))
		},
		"duplicate title": func(l *dashboardLayout) *dashboardLayout {
			return l.Row("r", chartGrid()).Panel(1, panel("Same Name")).Panel(2, panel("same name"))
		},
	}
	for name, declare := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := declare(newLayout(dashboardv2.NewDashboardBuilder("test"))).Build(); err == nil {
				t.Fatal("invalid layout accepted")
			}
		})
	}
}

func TestRefIDs(t *testing.T) {
	for i, want := range map[int]string{0: "A", 1: "B", 25: "Z", 26: "AA", 27: "AB", 52: "BA"} {
		if got := refID(i); got != want {
			t.Errorf("refID(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestQueriesPreserveNamedReferences(t *testing.T) {
	group, err := queries(
		queryWithRef("Used", promQuery().RefId("Used").Expr("used")),
		queryWithRef("Total", promQuery().RefId("Total").Expr("total")),
		promQuery().Expr("other"),
	).Build()
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, query := range group.Spec.Queries {
		refs = append(refs, query.Spec.RefId)
	}
	if want := []string{"Used", "Total", "C"}; !reflect.DeepEqual(refs, want) {
		t.Errorf("query references = %v, want %v", refs, want)
	}
}
