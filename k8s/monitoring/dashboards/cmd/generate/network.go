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
	const (
		elementInterfacesUp              = "interfaces-up"
		elementInterfacesDown            = "interfaces-down"
		elementInterfacesShutdown        = "interfaces-shutdown"
		elementTotalTraffic              = "total-traffic"
		elementTrafficBps                = "traffic-bps"
		elementInterfaceErrors           = "interface-errors"
		elementInterfaceDiscards         = "interface-discards"
		elementSubInterfaceErrorsPerHour = "sub-interface-errors-per-hour"
		elementCurrentNAPTEntries        = "current-napt-entries"
		elementNAPTUtilization           = "napt-utilization"
		elementPeakNAPTEntries           = "peak-napt-entries"
		elementNAPTCacheActivity         = "napt-cache-activity"
		elementIPv4ForwardingDiagnostics = "ipv4-forwarding-diagnostics"
	)

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

	d.Panel(elementInterfacesUp, dashboardv2.NewPanelBuilder().
		Id(networkPanelInterfacesUp).
		Title("Interfaces Up").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count by (instance) (ifOperStatus{`+ifFilter+`} == 1`+adminUp+`)`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementInterfacesDown, dashboardv2.NewPanelBuilder().
		Id(networkPanelInterfacesDown).
		Title("Interfaces Down").
		Description("Ports that are enabled but have no link. Ports shut down on "+
			"purpose are excluded and counted separately, so anything here is a fault.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Key zero on the unfiltered selector so all-healthy devices remain visible.
				Expr(`count by (instance) (ifOperStatus{`+ifFilter+`} != 1`+adminUp+`) or count by (instance) (ifOperStatus{`+ifFilter+`}) * 0`).
				LegendFormat("{{instance}}")))))

	// Show administratively disabled ports separately so mistaken shutdowns
	// cannot disappear behind the operational-status filter.
	d.Panel(elementInterfacesShutdown, dashboardv2.NewPanelBuilder().
		Id(networkPanelInterfacesShutdown).
		Title("Interfaces Shutdown").
		Description("Ports administratively disabled (ifAdminStatus=down). Not a "+
			"fault, but these are excluded from Interfaces Up and Down, so the count "+
			"is here to make the exclusion visible.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count by (instance) (ifAdminStatus{`+ifFilter+`} == 2) or count by (instance) (ifAdminStatus{`+ifFilter+`}) * 0`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementTotalTraffic, dashboardv2.NewPanelBuilder().
		Id(networkPanelTotalTraffic).
		Title("Total Traffic").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("bps").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (instance) (rate(ifHCInOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
				LegendFormat("{{instance}} In"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (instance) (rate(ifHCOutOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
				LegendFormat("{{instance}} Out"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval))))

	d.Panel(elementTrafficBps, dashboardv2.NewPanelBuilder().
		Id(networkPanelTrafficBps).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum by (instance) (rate(ifHCInOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
				LegendFormat("{{instance}} In"))).
			Target(dashboardv2.NewTargetBuilder().RefId("Out").Query(promQuery().
				Expr(`sum by (instance) (rate(ifHCOutOctets{`+ifFilter+`}[$__rate_interval]) * 8`+adminUp+`)`).
				LegendFormat("{{instance}} Out"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval))))

	d.Panel(elementInterfaceErrors, dashboardv2.NewPanelBuilder().
		Id(networkPanelInterfaceErrors).
		Title("Interface Errors").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("pps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ifInErrors{`+ifFilter+`}[$__rate_interval])`+adminUp).
				LegendFormat("{{instance}} {{ifDescr}} In"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ifOutErrors{`+ifFilter+`}[$__rate_interval])`+adminUp).
				LegendFormat("{{instance}} {{ifDescr}} Out"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval))))

	d.Panel(elementInterfaceDiscards, dashboardv2.NewPanelBuilder().
		Id(networkPanelInterfaceDiscards).
		Title("Interface Discards").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("pps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ifInDiscards{`+ifFilter+`}[$__rate_interval])`+adminUp).
				LegendFormat("{{instance}} {{ifDescr}} In"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ifOutDiscards{`+ifFilter+`}[$__rate_interval])`+adminUp).
				LegendFormat("{{instance}} {{ifDescr}} Out"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval))))

	// Physical-port filters exclude these active logical-interface counters.
	// Display without alerting because device semantics determine whether
	// transmit errors are routine. A fixed hour keeps counts comparable.
	d.Panel(elementSubInterfaceErrorsPerHour, dashboardv2.NewPanelBuilder().
		Id(networkPanelSubInterfaceErrorsPerHour).
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
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`increase(ifInErrors{`+subIfFilter+`}[1h])`).
				LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}} In"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`increase(ifOutErrors{`+subIfFilter+`}[1h])`).
				LegendFormat("{{instance}} {{ifDescr}} {{ifAlias}} Out")))))

	d.Panel(elementCurrentNAPTEntries, dashboardv2.NewPanelBuilder().
		Id(networkPanelCurrentNAPTEntries).
		Title("Current NAPT Entries").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`naptCacheEntries{instance="bgw1"}`).
				LegendFormat("{{ifDescr}} {{ifAlias}}")))))

	d.Panel(elementNAPTUtilization, dashboardv2.NewPanelBuilder().
		Id(networkPanelNAPTUtilization).
		Title("NAPT Utilization").
		Description("Current entries divided by the IX2106 default 65,535-entry capacity. Update the chart value if the router config overrides it.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percentunit").
			Min(0).
			Max(1).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`naptCacheEntries{instance="bgw1"} / 65535`).
				LegendFormat("{{ifDescr}} {{ifAlias}}")))))

	d.Panel(elementPeakNAPTEntries, dashboardv2.NewPanelBuilder().
		Id(networkPanelPeakNAPTEntries).
		Title("Peak NAPT Entries").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(measurementThresholds()).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`naptCachePeak{instance="bgw1"}`).
				LegendFormat("{{ifDescr}} {{ifAlias}}")))))

	d.Panel(elementNAPTCacheActivity, dashboardv2.NewPanelBuilder().
		Id(networkPanelNAPTCacheActivity).
		Title("NAPT Cache Activity").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(naptCacheCreates{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("{{ifDescr}} creates"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(naptCacheOverflows{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("{{ifDescr}} overflows"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval))))

	d.Panel(elementIPv4ForwardingDiagnostics, dashboardv2.NewPanelBuilder().
		Id(networkPanelIPv4ForwardingDiagnostics).
		Title("IPv4 Forwarding Diagnostics").
		Description("Device-wide MIB-II counters. Compare failures with sub-interface errors; these counters do not identify the affected interface, ACL, or destination.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("pps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ipInDiscards{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("IPv4 input discards"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ipOutDiscards{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("IPv4 output discards"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ipOutNoRoutes{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("IPv4 no route"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ipFragFails{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("IPv4 fragmentation failures"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ipFragCreates{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("IPv4 fragments created"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(ipReasmFails{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("IPv4 reassembly failures"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(icmpOutDestUnreachs{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("ICMP destination unreachable"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(udpInErrors{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("UDP input errors"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`rate(udpNoPorts{instance="bgw1"}[$__rate_interval])`).
				LegendFormat("UDP no port"))).QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval(snmpMinInterval))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementInterfacesUp).
			WithItem(elementInterfacesDown).
			WithItem(elementInterfacesShutdown).
			WithItem(elementTotalTraffic))).
		Row(dashboardv2.NewRowBuilder().Title("Traffic").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementTrafficBps))).
		Row(dashboardv2.NewRowBuilder().Title("Errors & Discards").AutoGridLayout(chartGrid().
			WithItem(elementInterfaceErrors).
			WithItem(elementInterfaceDiscards))).
		Row(dashboardv2.NewRowBuilder().Title("Sub-interface Errors (per hour)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementSubInterfaceErrorsPerHour))).
		Row(dashboardv2.NewRowBuilder().Title("IX2106 NAPT").AutoGridLayout(statGrid(3).
			WithItem(elementCurrentNAPTEntries).
			WithItem(elementNAPTUtilization).
			WithItem(elementPeakNAPTEntries))).
		Row(dashboardv2.NewRowBuilder().Title("NAPT Cache Activity").AutoGridLayout(chartGrid().
			WithItem(elementNAPTCacheActivity).
			WithItem(elementIPv4ForwardingDiagnostics)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
