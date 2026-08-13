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

	// adminUp drops ports that are administratively shut down. ifAdminStatus is a
	// separate series from the counters and from ifOperStatus, so this cannot be a
	// label selector -- it has to be a set intersection on ifIndex. Appending it
	// needs no extra parentheses: `and` binds looser than both `*` and the
	// comparison operators, and it sits inside sum by (instance) because the
	// aggregation is what drops ifIndex.
	//
	// Without it, Interfaces Down conflates two different facts: a port nobody has
	// enabled, and a port that is enabled and has lost its link. Only the second is
	// worth reacting to. c1200 keeps a couple of ports shut at any given time and
	// they were being counted as faults; which ports those are changes as the
	// switch is reconfigured, so no port is named here on purpose.
	//
	// Interfaces Up does not change value -- a shut port is never operationally up
	// -- but carries the filter so the pair means the same thing.
	const adminUp = ` and on(instance, ifIndex) (ifAdminStatus{` + ifFilter + `} == 1)`

	// Min interval for every rate() panel here. SNMP is probed once a minute
	// (values/snmp-exporter.yaml, interval: 60s -- measured, count_over_time over
	// ten minutes returns exactly 10 samples), but the Prometheus datasource has
	// no timeInterval set, so Grafana assumes the 15s default when it builds
	// $__rate_interval = max($__interval + scrape, 4 x scrape). Whenever
	// $__interval falls below 45s that collapses to a 60s window, and a 60s window
	// on a 60s scrape does not reliably contain the two samples rate() needs:
	// measured over the last 30 minutes, rate(...[60s]) returned no series at
	// every single step, while [2m] returned all of them.
	//
	// The failure therefore depends on panel width and time range together, which
	// is why it looked arbitrary. $__interval is roughly range / pixel-width, so
	// the full-width Traffic (bps) panel broke even at the dashboard's default 24h
	// while the half-width Total Traffic beside it still worked, and Total Traffic
	// then broke too once the range was pulled in below a day.
	//
	// 2m floors $__interval so $__rate_interval lands at 135s, verified to return
	// every series at every step. A 1m floor would give 75s, which straddles the
	// two-sample boundary depending on scrape alignment -- not worth the risk to
	// save a minute of smoothing on a link that is sampled once a minute anyway.
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
		// 24h, not the 30d this used to open at. The rate windows follow the zoom:
		// at 30d $__rate_interval resolved to roughly 63 minutes, which averages a
		// link's busiest minute into an hour of quiet either side. bgw1 peaked at
		// 427 Mbps over the last 30 days measured in 10-minute windows and idles
		// near 1.8 Mbps, so the panel that exists to show how hard the line is
		// working was smoothing away the only part worth seeing. At 24h the window
		// is about 4 minutes -- $__interval of 2 minutes against a 60s scrape, so
		// the 4 x scrape floor decides it -- and bursts survive. The 30-day view is
		// still one zoom away when the question is a monthly trend.
		Time("now-24h", "now").
		// Matches the 60s SNMP scrape interval: refreshing faster would redraw the
		// same points, and slower would leave the newest scrape off the screen.
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
					// The zero baseline stays on the unfiltered selector: keyed on the
					// filtered one it would vanish for a device whose ports are all
					// healthy, which is exactly when it needs to read 0.
					Expr(`count by (instance) (ifOperStatus{` + ifFilter + `} != 1` + adminUp + `) or count by (instance) (ifOperStatus{` + ifFilter + `}) * 0`).
					LegendFormat("{{instance}}"),
				),
		).
		// The counterpart to the adminUp filter, and the reason it is safe to apply.
		// Filtering shut ports out of Interfaces Down means a port disabled by
		// mistake would otherwise disappear from the dashboard entirely rather than
		// showing up as a fault -- turning a problem into nothing at all. This tile
		// keeps the count visible, so "Down 1" can always be read against how many
		// ports are intentionally off.
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
