package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildNetworkOverview defines the network device dashboard using SNMP MIB-II metrics.
// The snmp-exporter Probe relabels instance to the device hostname
// (bgw1 = router, c1200 = switch), so panels use the instance label directly.
// ifHC* counters are 64-bit, avoiding wrap-around on high-speed interfaces.
func buildNetworkOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	// Physical ports on the router and switch use GigaEthernetN/GigabitEthernetN.
	// Match them explicitly to exclude loopbacks, tunnels, VLANs, port channels,
	// subinterfaces, and vendor-internal interfaces.
	const ifFilter = `ifDescr=~"GigaEthernet[0-9]+|GigabitEthernet[0-9]+", instance=~"$instance"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()
	issueThresholds := issueThresholds()

	d, err := dashboard.NewDashboardBuilder("Network Overview").
		Uid("network-overview").
		Tags([]string{"network", "infrastructure"}).
		Timezone("browser").
		Time("now-30d", "now").
		Refresh("60s"). // SNMP scrapes are expensive; 60s is a reasonable interval.
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
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (ifOperStatus{` + ifFilter + `} == 1)`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Interfaces Down").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count by (instance) (ifOperStatus{` + ifFilter + `} != 1) or count by (instance) (ifOperStatus{` + ifFilter + `}) * 0`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Total Traffic").
				Datasource(ds).
				Span(12).Height(4).
				Unit("bps").
				Min(0).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCInOctets{` + ifFilter + `}[$__rate_interval]) * 8)`).
					LegendFormat("{{instance}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum by (instance) (rate(ifHCOutOctets{` + ifFilter + `}[$__rate_interval]) * 8)`).
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
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("In").
					Expr(`sum by (instance) (rate(ifHCInOctets{`+ifFilter+`}[$__rate_interval]) * 8)`).
					LegendFormat("{{instance}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					RefId("Out").
					Expr(`sum by (instance) (rate(ifHCOutOctets{`+ifFilter+`}[$__rate_interval]) * 8)`).
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
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInErrors{` + ifFilter + `}[$__rate_interval])`).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutErrors{` + ifFilter + `}[$__rate_interval])`).
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
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifInDiscards{` + ifFilter + `}[$__rate_interval])`).
					LegendFormat("{{instance}} {{ifDescr}} In"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`rate(ifOutDiscards{` + ifFilter + `}[$__rate_interval])`).
					LegendFormat("{{instance}} {{ifDescr}} Out"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
