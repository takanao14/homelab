package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	uptimePanelICMPDevicesDown          = 1
	uptimePanelDNSDevicesDown           = 2
	uptimePanelICMPAvailabilityRange    = 3
	uptimePanelDNSAvailabilityRange     = 4
	uptimePanelICMPStatus               = 5
	uptimePanelDNSExternalStatus        = 6
	uptimePanelDNSInternalStatus        = 7
	uptimePanelICMPAvailabilityByDevice = 8
	uptimePanelDNSAvailabilityByDevice  = 9
	uptimePanelICMPStatusHistory        = 10
	uptimePanelICMPResponseTime         = 11
	uptimePanelDNSStatusHistory         = 12
	uptimePanelDNSResponseTime          = 13
)

// buildUptime covers blackbox ICMP and DNS probe success.
// ScrapeConfig jobs use scrapeConfig/<namespace>/<name>.
func buildUptime() (*dashboardv2.Dashboard, error) {
	const (
		elementICMPDevicesDown          = "icmp-devices-down"
		elementDNSDevicesDown           = "dns-devices-down"
		elementICMPAvailabilityRange    = "icmp-availability-range"
		elementDNSAvailabilityRange     = "dns-availability-range"
		elementICMPStatus               = "icmp-status"
		elementDNSExternalStatus        = "dns-external-status"
		elementDNSInternalStatus        = "dns-internal-status"
		elementICMPAvailabilityByDevice = "icmp-availability-by-device"
		elementDNSAvailabilityByDevice  = "dns-availability-by-device"
		elementICMPStatusHistory        = "icmp-status-history"
		elementICMPResponseTime         = "icmp-response-time"
		elementDNSStatusHistory         = "dns-status-history"
		elementDNSResponseTime          = "dns-response-time"
	)

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		icmpJob   = `job="scrapeConfig/monitoring/icmp-network-devices"`
		dnsExtJob = `job="scrapeConfig/monitoring/dns-external"`
		dnsIntJob = `job="scrapeConfig/monitoring/dns-internal"`
		dnsJobs   = `job=~"scrapeConfig/monitoring/dns-(external|internal)"`
	)

	// nil threshold Value means -Infinity (base step).
	probeThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})

	// Availability thresholds: red below 99%, yellow 99–99.9%, green at/above 99.9%.
	availabilityThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(99)), Color: "yellow"},
			{Value: new(float64(99.9)), Color: "green"},
		})

	// downThresholds: green at 0, red once any probe is down.
	downThresholds := issueThresholds()

	probeValueMappings := []dashboardv2.ValueMapping{
		{ValueMap: &dashboardv2.ValueMap{
			Type: dashboardv2.MappingTypeValue,
			Options: map[string]dashboardv2.ValueMappingResult{
				"0": {Text: new("DOWN"), Color: new("red")},
				"1": {Text: new("UP"), Color: new("green")},
			},
		}},
	}

	d := dashboardv2.NewDashboardBuilder("Uptime").
		Tags([]string{"uptime", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().
			Timezone("browser").
			From("now-30d").To("now").
			AutoRefresh("60s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable())

	d.Panel(elementICMPDevicesDown, dashboardv2.NewPanelBuilder().
		Id(uptimePanelICMPDevicesDown).
		Title("ICMP Devices Down").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(downThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`count(probe_success{`+icmpJob+`} == 0) or vector(0)`).
				LegendFormat("down")))))

	d.Panel(elementDNSDevicesDown, dashboardv2.NewPanelBuilder().
		Id(uptimePanelDNSDevicesDown).
		Title("DNS Devices Down").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(downThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`count(probe_success{`+dnsJobs+`} == 0) or vector(0)`).
				LegendFormat("down")))))

	d.Panel(elementICMPAvailabilityRange, dashboardv2.NewPanelBuilder().
		Id(uptimePanelICMPAvailabilityRange).
		Title("ICMP Availability (range)").
		Description("Mean probe success across all ICMP targets over the dashboard's time range, so it moves with the zoom.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`avg(avg_over_time(probe_success{`+icmpJob+`}[$__range])) * 100`).
				LegendFormat("availability")))))

	// Give ICMP and DNS symmetric range availability and current-down tiles.
	d.Panel(elementDNSAvailabilityRange, dashboardv2.NewPanelBuilder().
		Id(uptimePanelDNSAvailabilityRange).
		Title("DNS Availability (range)").
		Description("Mean probe success across all DNS probes, external and internal together, over the dashboard's time range.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`avg(avg_over_time(probe_success{`+dnsJobs+`}[$__range])) * 100`).
				LegendFormat("availability")))))

	d.Panel(elementICMPStatus, dashboardv2.NewPanelBuilder().
		Id(uptimePanelICMPStatus).
		Title("ICMP Status").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			GraphMode(common.BigValueGraphModeNone).
			Orientation(common.VizOrientationAuto).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_success{`+icmpJob+`}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementDNSExternalStatus, dashboardv2.NewPanelBuilder().
		Id(uptimePanelDNSExternalStatus).
		Title("DNS External Status").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			GraphMode(common.BigValueGraphModeNone).
			Orientation(common.VizOrientationAuto).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_success{`+dnsExtJob+`}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementDNSInternalStatus, dashboardv2.NewPanelBuilder().
		Id(uptimePanelDNSInternalStatus).
		Title("DNS Internal Status").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			GraphMode(common.BigValueGraphModeNone).
			Orientation(common.VizOrientationAuto).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_success{`+dnsIntJob+`}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementICMPAvailabilityByDevice, dashboardv2.NewPanelBuilder().
		Id(uptimePanelICMPAvailabilityByDevice).
		Title("ICMP Availability by Device").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`avg_over_time(probe_success{`+icmpJob+`}[$__range]) * 100`).
				Instant(true).Range(false).
				LegendFormat("{{instance}}")))))

	d.Panel(elementDNSAvailabilityByDevice, dashboardv2.NewPanelBuilder().
		Id(uptimePanelDNSAvailabilityByDevice).
		Title("DNS Availability by Device").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`avg_over_time(probe_success{`+dnsExtJob+`}[$__range]) * 100`).
				Instant(true).Range(false).
				LegendFormat("{{instance}} External"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`avg_over_time(probe_success{`+dnsIntJob+`}[$__range]) * 100`).
				Instant(true).Range(false).
				LegendFormat("{{instance}} Internal")))))

	d.Panel(elementICMPStatusHistory, dashboardv2.NewPanelBuilder().
		Id(uptimePanelICMPStatusHistory).
		Title("ICMP Status History").
		Visualization(statetimeline.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			ShowValue(common.VisibilityModeNever).
			MergeValues(true).
			Tooltip(tooltipAll)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_success{`+icmpJob+`}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementICMPResponseTime, dashboardv2.NewPanelBuilder().
		Id(uptimePanelICMPResponseTime).
		Title("ICMP Response Time").
		Description("Round-trip time of the echo request itself, excluding the time blackbox spent resolving the target name and opening its socket.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		// Use the ICMP rtt phase; total probe duration also includes setup and DNS.
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_icmp_duration_seconds{`+icmpJob+`, phase="rtt"}`).
				LegendFormat("{{instance}}")))))

	d.Panel(elementDNSStatusHistory, dashboardv2.NewPanelBuilder().
		Id(uptimePanelDNSStatusHistory).
		Title("DNS Status History").
		Visualization(statetimeline.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			ShowValue(common.VisibilityModeNever).
			MergeValues(true).
			Tooltip(tooltipAll)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_success{`+dnsExtJob+`}`).
				LegendFormat("{{instance}} External"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`probe_success{`+dnsIntJob+`}`).
				LegendFormat("{{instance}} Internal")))))

	d.Panel(elementDNSResponseTime, dashboardv2.NewPanelBuilder().
		Id(uptimePanelDNSResponseTime).
		Title("DNS Response Time").
		Description("How long each resolver took to answer the test query. External resolves a public name, internal an in-fleet one, so the two are expected to differ.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		// Use total DNS probe duration; lookup_time measures resolution of the
		// probe target itself, not the test query.
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`probe_duration_seconds{`+dnsExtJob+`}`).
				LegendFormat("{{instance}} External"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").Query(promQuery().
				Expr(`probe_duration_seconds{`+dnsIntJob+`}`).
				LegendFormat("{{instance}} Internal")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementICMPDevicesDown).
			WithItem(elementDNSDevicesDown).
			WithItem(elementICMPAvailabilityRange).
			WithItem(elementDNSAvailabilityRange))).
		Row(dashboardv2.NewRowBuilder().Title("ICMP Status").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeShort).
			WithItem(elementICMPStatus))).
		Row(dashboardv2.NewRowBuilder().Title("DNS Status").AutoGridLayout(statGrid(2).
			WithItem(elementDNSExternalStatus).
			WithItem(elementDNSInternalStatus))).
		Row(dashboardv2.NewRowBuilder().Title("Availability").AutoGridLayout(chartGrid().
			WithItem(elementICMPAvailabilityByDevice).
			WithItem(elementDNSAvailabilityByDevice))).
		Row(dashboardv2.NewRowBuilder().Title("ICMP Diagnostics").AutoGridLayout(chartGrid().
			WithItem(elementICMPStatusHistory).
			WithItem(elementICMPResponseTime))).
		Row(dashboardv2.NewRowBuilder().Title("DNS Diagnostics").AutoGridLayout(chartGrid().
			WithItem(elementDNSStatusHistory).
			WithItem(elementDNSResponseTime)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
