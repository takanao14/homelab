package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildNetworkOverview covers physical SNMP MIB-II interfaces. Probe relabeling
// supplies hostnames, and 64-bit ifHC counters avoid high-speed wraparound.
func buildNetworkOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	// Match physical ports and exclude virtual/vendor interfaces.
	const ifFilter = `ifDescr=~"GigaEthernet[0-9]+|GigabitEthernet[0-9]+", instance=~"$instance"`

	// Intersect with administratively enabled ports so Interfaces Down means
	// lost link, not intentional shutdown. Keep a separate disabled-port count.
	const adminUp = ` and on(instance, ifIndex) (ifAdminStatus{` + ifFilter + `} == 1)`

	// SNMP scrapes every minute; floor rate panels at two minutes so
	// $__rate_interval reliably contains two samples regardless of panel width.
	const snmpMinInterval = "2m"

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()
	issueThresholds := issueThresholds()

	d, err := dashboard.NewDashboardBuilder("Network Overview").
		Uid("network-overview").
		Tags([]string{"network", "infrastructure"}).
		Timezone("browser").
		// A 24-hour default preserves bursts that 30-day rate windows smooth away.
		Time("now-24h", "now").
		// Refresh at the 60-second SNMP scrape interval.
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithVariable(
			dashboard.NewCustomVariableBuilder("instance").
				Label("Device").
				Values(dashboard.StringOrMap{String: new("bgw1,c1200")}).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Interfaces Up").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (ifOperStatus{` + ifFilter + `} == 1` + adminUp + `)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Interfaces Down").
				Description("Ports that are enabled but have no link. Ports shut down on " +
					"purpose are excluded and counted separately, so anything here is a fault.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Key zero on the unfiltered selector so all-healthy devices remain visible.
					Expr(`count by (instance) (ifOperStatus{` + ifFilter + `} != 1` + adminUp + `) or count by (instance) (ifOperStatus{` + ifFilter + `}) * 0`).
					LegendFormat("{{instance}}"),
				),
		).
		// Show administratively disabled ports separately so mistaken shutdowns
		// cannot disappear behind the operational-status filter.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Interfaces Shutdown").
				Description("Ports administratively disabled (ifAdminStatus=down). Not a " +
					"fault, but these are excluded from Interfaces Up and Down, so the count " +
					"is here to make the exclusion visible.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (ifAdminStatus{` + ifFilter + `} == 2) or count by (instance) (ifAdminStatus{` + ifFilter + `}) * 0`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Traffic").
				Datasource(ds).
				Span(24).Height(4).
				Unit("bps").
				Min(0).
				Interval(snmpMinInterval).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCInOctets{` + ifFilter + `}[$__rate_interval]) * 8` + adminUp + `)`).
					LegendFormat("{{instance}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCOutOctets{` + ifFilter + `}[$__rate_interval]) * 8` + adminUp + `)`).
					LegendFormat("{{instance}} Out"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Traffic")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Traffic (bps)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("bps").
				Interval(snmpMinInterval).
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("In").
					Expr(`sum by (instance) (rate(ifHCInOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
					LegendFormat("{{instance}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Out").
					Expr(`sum by (instance) (rate(ifHCOutOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
					LegendFormat("{{instance}} Out"),
				).
				OverrideByQuery("Out", []dashboard.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				}),
		).
		WithRow(dashboard.NewRowBuilder("Errors & Discards")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Interface Errors").
				Datasource(ds).
				Span(12).Height(8).
				Unit("pps").
				Min(0).
				Interval(snmpMinInterval).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInErrors{` + ifFilter + `}[$__rate_interval])` + adminUp).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutErrors{` + ifFilter + `}[$__rate_interval])` + adminUp).
					LegendFormat("{{instance}} {{ifDescr}} Out"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Interface Discards").
				Datasource(ds).
				Span(12).Height(8).
				Unit("pps").
				Min(0).
				Interval(snmpMinInterval).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInDiscards{` + ifFilter + `}[$__rate_interval])` + adminUp).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutDiscards{` + ifFilter + `}[$__rate_interval])` + adminUp).
					LegendFormat("{{instance}} {{ifDescr}} Out"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
