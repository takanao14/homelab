package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildNetworkOverview defines the network device dashboard using SNMP MIB-II metrics.
// Targets: 192.168.10.1 (router), 192.168.10.2 (switch).
// ifHC* counters are 64-bit, avoiding wrap-around on high-speed interfaces.
func buildNetworkOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	// Exclude virtual/management interfaces to show only physical ports.
	const ifFilter = `ifDescr!~"Loopback.*|Null.*|bluetooth.*", instance=~"$instance"`

	d, err := dashboard.NewDashboardBuilder("Network Overview").
		Uid("network-overview").
		Tags([]string{"network", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("60s"). // SNMP scrapes are expensive; 60s is a reasonable interval.
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("instance").
				Label("Device").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(ifHCInOctets, instance)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Interfaces Up").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (ifOperStatus{` + ifFilter + `} == 1)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Traffic In").
				Datasource(ds).
				Span(8).Height(4).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCInOctets{` + ifFilter + `}[5m]) * 8)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Traffic Out").
				Datasource(ds).
				Span(8).Height(4).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCOutOctets{` + ifFilter + `}[5m]) * 8)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Traffic In (bps)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifHCInOctets{` + ifFilter + `}[5m]) * 8`).
					LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Traffic Out (bps)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifHCOutOctets{` + ifFilter + `}[5m]) * 8`).
					LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Interface Errors").
				Datasource(ds).
				Span(12).Height(8).
				Unit("pps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInErrors{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutErrors{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} Out"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Interface Discards").
				Datasource(ds).
				Span(12).Height(8).
				Unit("pps").
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInDiscards{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutDiscards{` + ifFilter + `}[5m])`).
					LegendFormat("{{instance}} {{ifDescr}} Out"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
