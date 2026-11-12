package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
)

// dashboardLayout declares a dashboard as rows of AutoGrid-placed panels in
// display order. Grafana sizes and wraps panels per row from the row's grid
// settings, so panels carry no coordinates. Element names derive from panel
// titles, which must be unique within a dashboard.
type dashboardLayout struct {
	dashboard *dashboardv2.DashboardBuilder
	rows      []layoutRow
	names     map[string]bool
	ids       map[int]bool
	err       error
}

type layoutRow struct {
	title string
	grid  *dashboardv2.AutoGridBuilder
}

func newLayout(d *dashboardv2.DashboardBuilder) *dashboardLayout {
	return &dashboardLayout{dashboard: d, names: map[string]bool{}, ids: map[int]bool{}}
}

// Row starts a row laid out by grid; following panels belong to it.
func (l *dashboardLayout) Row(title string, grid *dashboardv2.AutoGridBuilder) *dashboardLayout {
	l.rows = append(l.rows, layoutRow{title: title, grid: grid})
	return l
}

// Panel appends a panel to the current row.
func (l *dashboardLayout) Panel(id int, panel *dashboardv2.PanelBuilder) *dashboardLayout {
	if l.err != nil {
		return l
	}
	if len(l.rows) == 0 {
		l.err = fmt.Errorf("panel declared before any row")
		return l
	}
	if id <= 0 || l.ids[id] {
		l.err = fmt.Errorf("panel ID %d: must be positive and unique", id)
		return l
	}
	built, err := panel.Build()
	if err != nil {
		l.err = err
		return l
	}
	name := elementName(built.Spec.Title)
	if name == "" || l.names[name] {
		l.err = fmt.Errorf("panel %q: empty or duplicate element name %q", built.Spec.Title, name)
		return l
	}
	l.names[name] = true
	l.ids[id] = true
	l.dashboard.Panel(name, panel.Id(float64(id)))
	l.rows[len(l.rows)-1].grid.WithItem(name)
	return l
}

func (l *dashboardLayout) Build() (*dashboardv2.Dashboard, error) {
	if l.err != nil {
		return nil, l.err
	}
	rows := dashboardv2.NewRowsBuilder()
	for _, r := range l.rows {
		rows.Row(dashboardv2.NewRowBuilder().Title(r.title).AutoGridLayout(r.grid))
	}
	d, err := l.dashboard.RowsLayout(rows).Build()
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// elementName turns a panel title into a lowercase, hyphen-separated key.
func elementName(title string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			hyphen = false
			continue
		}
		hyphen = true
	}
	return b.String()
}
