package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	networkPanelInterfacesUp              = 1
	networkPanelInterfacesDown            = 2
	networkPanelInterfacesShutdown        = 3
	networkPanelTotalTraffic              = 4
	networkPanelTrafficBps                = 5
	networkPanelInterfaceErrors           = 6
	networkPanelInterfaceDiscards         = 7
	networkPanelSubInterfaceErrorsPerHour = 8
	networkPanelCurrentNAPTEntries        = 9
	networkPanelNAPTUtilization           = 10
	networkPanelPeakNAPTEntries           = 11
	networkPanelNAPTCacheActivity         = 12
	networkPanelIPv4ForwardingDiagnostics = 13
)

// buildNetworkOverview covers physical SNMP MIB-II interfaces. Probe relabeling
// supplies hostnames, and 64-bit ifHC counters avoid high-speed wraparound.
func buildNetworkOverview() (*dashboardv2.Dashboard, error) {
	// Match physical ports and exclude virtual/vendor interfaces.
	const ifFilter = `ifDescr=~"GigaEthernet[0-9]+|GigabitEthernet[0-9]+", instance=~"$instance"`

	// Intersect with administratively enabled ports so Interfaces Down means
	// lost link, not intentional shutdown. Keep a separate disabled-port count.
	const adminUp = ` and on(instance, ifIndex) (ifAdminStatus{` + ifFilter + `} == 1)`

	// SNMP scrapes every minute; floor rate panels at two minutes so
	// $__rate_interval reliably contains two samples regardless of panel width.
	const snmpMinInterval = "2m"

	// Match logical L3 sub-interfaces; exclude non-forwarding Loopback and Null.
	const subIfFilter = `ifDescr=~".+\\.[0-9]+", ifDescr!~"Loopback.*|Null.*", instance=~"$instance"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	zeroLineThresholds := zeroLineThresholds()
	zeroLineStyle := zeroLineStyle()
	issueThresholds := issueThresholds()

	d := dashboardv2.NewDashboardBuilder("Network Overview").
		Tags([]string{"network", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-24h").To("now").AutoRefresh("60s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		CustomVariable(dashboardv2.NewCustomVariableBuilder("instance").
			Label("Device").
			Query("bgw1,c1200").
			Multi(true).
			IncludeAll(true))

	return newLayout(d).
		Row("Summary", statGrid(4)).
		Panel(networkPanelInterfacesUp, dashboardv2.NewPanelBuilder().
			Title("Interfaces Up").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count by (instance) (ifOperStatus{`+ifFilter+`} == 1`+adminUp+`)`).
				LegendFormat("{{instance}}")))).
		Panel(networkPanelInterfacesDown, dashboardv2.NewPanelBuilder().
			Title("Interfaces Down").
			Description("Ports that are enabled but have no link. Ports shut down on "+
				"purpose are excluded and counted separately, so anything here is a fault.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				// Key zero on the unfiltered selector so all-healthy devices remain visible.
				Expr(`count by (instance) (ifOperStatus{`+ifFilter+`} != 1`+adminUp+`) or count by (instance) (ifOperStatus{`+ifFilter+`}) * 0`).
				LegendFormat("{{instance}}")))).
		// Show administratively disabled ports separately so mistaken shutdowns
		// cannot disappear behind the operational-status filter.
		Panel(networkPanelInterfacesShutdown, dashboardv2.NewPanelBuilder().
			Title("Interfaces Shutdown").
			Description("Ports administratively disabled (ifAdminStatus=down). Not a "+
				"fault, but these are excluded from Interfaces Up and Down, so the count "+
				"is here to make the exclusion visible.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count by (instance) (ifAdminStatus{`+ifFilter+`} == 2) or count by (instance) (ifAdminStatus{`+ifFilter+`}) * 0`).
				LegendFormat("{{instance}}")))).
		Panel(networkPanelTotalTraffic, dashboardv2.NewPanelBuilder().
			Title("Total Traffic").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("bps").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum by (instance) (rate(ifHCInOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
				LegendFormat("{{instance}} In"),
				promQuery().
					Expr(`sum by (instance) (rate(ifHCOutOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
					LegendFormat("{{instance}} Out")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval)))).
		Row("Traffic", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		Panel(networkPanelTrafficBps, dashboardv2.NewPanelBuilder().
			Title("Traffic (bps)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bps").
				Tooltip(tooltipAll).
				Legend(legend).
				Thresholds(zeroLineThresholds).
				ThresholdsStyle(zeroLineStyle).
				OverrideByQuery("Out", []dashboardv2.DynamicConfigValue{
					{Id: "custom.transform", Value: "negative-Y"},
				})).
			Data(queries(queryWithRef("In", promQuery().
				RefId("In").
				Expr(`sum by (instance) (rate(ifHCInOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
				LegendFormat("{{instance}} In")),
				queryWithRef("Out", promQuery().
					RefId("Out").
					Expr(`sum by (instance) (rate(ifHCOutOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
					LegendFormat("{{instance}} Out"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval)))).
		Row("Errors & Discards", chartGrid()).
		Panel(networkPanelInterfaceErrors, dashboardv2.NewPanelBuilder().
			Title("Interface Errors").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("pps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`rate(ifInErrors{`+ifFilter+`}[$__rate_interval])`+adminUp).
				LegendFormat("{{instance}} {{ifDescr}} In"),
				promQuery().
					Expr(`rate(ifOutErrors{`+ifFilter+`}[$__rate_interval])`+adminUp).
					LegendFormat("{{instance}} {{ifDescr}} Out")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval)))).
		Panel(networkPanelInterfaceDiscards, dashboardv2.NewPanelBuilder().
			Title("Interface Discards").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("pps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`rate(ifInDiscards{`+ifFilter+`}[$__rate_interval])`+adminUp).
				LegendFormat("{{instance}} {{ifDescr}} In"),
				promQuery().
					Expr(`rate(ifOutDiscards{`+ifFilter+`}[$__rate_interval])`+adminUp).
					LegendFormat("{{instance}} {{ifDescr}} Out")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval)))).
		Row("Sub-interface Errors (per hour)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// Physical-port filters exclude these active logical-interface counters.
		// Display without alerting because device semantics determine whether
		// transmit errors are routine. A fixed hour keeps counts comparable.
		Panel(networkPanelSubInterfaceErrorsPerHour, dashboardv2.NewPanelBuilder().
			Title("Sub-interface Errors (per hour)").
			Description("Errors on logical L3 sub-interfaces, which every other panel on "+
				"this dashboard excludes. Physical ports have held at zero for seven days; "+
				"these have not. Not alerted -- transmit errors here may be routine for the "+
				"device, so the count is presented for judgement rather than paging.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`increase(ifInErrors{`+subIfFilter+`}[1h])`).
				LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}} In"),
				promQuery().
					Expr(`increase(ifOutErrors{`+subIfFilter+`}[1h])`).
					LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}} Out")))).
		Row("IX2106 NAPT", statGrid(3)).
		Panel(networkPanelCurrentNAPTEntries, dashboardv2.NewPanelBuilder().
			Title("Current NAPT Entries").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`naptCacheEntries{instance="bgw1"}`).
				LegendFormat("{{ifDescr}} {{ifAlias}}")))).
		Panel(networkPanelNAPTUtilization, dashboardv2.NewPanelBuilder().
			Title("NAPT Utilization").
			Description("Current entries divided by the IX2106 default 65,535-entry capacity. Update the chart value if the router config overrides it.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percentunit").
				Min(0).
				Max(1).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`naptCacheEntries{instance="bgw1"} / 65535`).
				LegendFormat("{{ifDescr}} {{ifAlias}}")))).
		Panel(networkPanelPeakNAPTEntries, dashboardv2.NewPanelBuilder().
			Title("Peak NAPT Entries").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(measurementThresholds()).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`naptCachePeak{instance="bgw1"}`).
				LegendFormat("{{ifDescr}} {{ifAlias}}")))).
		Row("NAPT Cache Activity", chartGrid()).
		Panel(networkPanelNAPTCacheActivity, dashboardv2.NewPanelBuilder().
			Title("NAPT Cache Activity").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("ops").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`rate(naptCacheCreates{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("{{ifDescr}} creates"),
				promQuery().
					Expr(`rate(naptCacheOverflows{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("{{ifDescr}} overflows")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval)))).
		Panel(networkPanelIPv4ForwardingDiagnostics, dashboardv2.NewPanelBuilder().
			Title("IPv4 Forwarding Diagnostics").
			Description("Device-wide MIB-II counters. Compare failures with sub-interface errors; these counters do not identify the affected interface, ACL, or destination.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("pps").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(`rate(ipInDiscards{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("IPv4 input discards"),
				promQuery().
					Expr(`rate(ipOutDiscards{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("IPv4 output discards"),
				promQuery().
					Expr(`rate(ipOutNoRoutes{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("IPv4 no route"),
				promQuery().
					Expr(`rate(ipFragFails{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("IPv4 fragmentation failures"),
				promQuery().
					Expr(`rate(ipFragCreates{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("IPv4 fragments created"),
				promQuery().
					Expr(`rate(ipReasmFails{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("IPv4 reassembly failures"),
				promQuery().
					Expr(`rate(icmpOutDestUnreachs{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("ICMP destination unreachable"),
				promQuery().
					Expr(`rate(udpInErrors{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("UDP input errors"),
				promQuery().
					Expr(`rate(udpNoPorts{instance="bgw1"}[$__rate_interval])`).
					LegendFormat("UDP no port")).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval)))).
		Build()
}
