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

	return newLayout(d).
		Row("Summary", statGrid(4)).
		Panel(uptimePanelICMPDevicesDown, dashboardv2.NewPanelBuilder().
			Title("ICMP Devices Down").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(downThresholds).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`count(probe_success{`+icmpJob+`} == 0) or vector(0)`).
				LegendFormat("down")))).
		Panel(uptimePanelDNSDevicesDown, dashboardv2.NewPanelBuilder().
			Title("DNS Devices Down").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(downThresholds).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`count(probe_success{`+dnsJobs+`} == 0) or vector(0)`).
				LegendFormat("down")))).
		Panel(uptimePanelICMPAvailabilityRange, dashboardv2.NewPanelBuilder().
			Title("ICMP Availability (range)").
			Description("Mean probe success across all ICMP targets over the dashboard's time range, so it moves with the zoom.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`avg(avg_over_time(probe_success{`+icmpJob+`}[$__range])) * 100`).
				LegendFormat("availability")))).
		// Give ICMP and DNS symmetric range availability and current-down tiles.
		Panel(uptimePanelDNSAvailabilityRange, dashboardv2.NewPanelBuilder().
			Title("DNS Availability (range)").
			Description("Mean probe success across all DNS probes, external and internal together, over the dashboard's time range.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`avg(avg_over_time(probe_success{`+dnsJobs+`}[$__range])) * 100`).
				LegendFormat("availability")))).
		Row("ICMP Status", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeShort)).
		Panel(uptimePanelICMPStatus, dashboardv2.NewPanelBuilder().
			Title("ICMP Status").
			Visualization(stat.NewVisualizationV2Builder().
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`probe_success{`+icmpJob+`}`).
				LegendFormat("{{instance}}")))).
		Row("DNS Status", statGrid(2)).
		Panel(uptimePanelDNSExternalStatus, dashboardv2.NewPanelBuilder().
			Title("DNS External Status").
			Visualization(stat.NewVisualizationV2Builder().
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`probe_success{`+dnsExtJob+`}`).
				LegendFormat("{{instance}}")))).
		Panel(uptimePanelDNSInternalStatus, dashboardv2.NewPanelBuilder().
			Title("DNS Internal Status").
			Visualization(stat.NewVisualizationV2Builder().
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground)).
			Data(queries(promQuery().
				Expr(`probe_success{`+dnsIntJob+`}`).
				LegendFormat("{{instance}}")))).
		Row("Availability", chartGrid()).
		Panel(uptimePanelICMPAvailabilityByDevice, dashboardv2.NewPanelBuilder().
			Title("ICMP Availability by Device").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))).
			Data(queries(promQuery().
				Expr(`avg_over_time(probe_success{`+icmpJob+`}[$__range]) * 100`).
				Instant(true).Range(false).
				LegendFormat("{{instance}}")))).
		Panel(uptimePanelDNSAvailabilityByDevice, dashboardv2.NewPanelBuilder().
			Title("DNS Availability by Device").
			Visualization(bargauge.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))).
			Data(queries(
				promQuery().
					Expr(`avg_over_time(probe_success{`+dnsExtJob+`}[$__range]) * 100`).
					Instant(true).Range(false).
					LegendFormat("{{instance}} External"),
				promQuery().
					Expr(`avg_over_time(probe_success{`+dnsIntJob+`}[$__range]) * 100`).
					Instant(true).Range(false).
					LegendFormat("{{instance}} Internal"),
			))).
		Row("ICMP Diagnostics", chartGrid()).
		Panel(uptimePanelICMPStatusHistory, dashboardv2.NewPanelBuilder().
			Title("ICMP Status History").
			Visualization(statetimeline.NewVisualizationV2Builder().
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
				Tooltip(tooltipAll)).
			Data(queries(promQuery().
				Expr(`probe_success{`+icmpJob+`}`).
				LegendFormat("{{instance}}")))).
		Panel(uptimePanelICMPResponseTime, dashboardv2.NewPanelBuilder().
			Title("ICMP Response Time").
			Description("Round-trip time of the echo request itself, excluding the time blackbox spent resolving the target name and opening its socket.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			// Use the ICMP rtt phase; total probe duration also includes setup and DNS.
			Data(queries(promQuery().
				Expr(`probe_icmp_duration_seconds{`+icmpJob+`, phase="rtt"}`).
				LegendFormat("{{instance}}")))).
		Row("DNS Diagnostics", chartGrid()).
		Panel(uptimePanelDNSStatusHistory, dashboardv2.NewPanelBuilder().
			Title("DNS Status History").
			Visualization(statetimeline.NewVisualizationV2Builder().
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
				Tooltip(tooltipAll)).
			Data(queries(
				promQuery().
					Expr(`probe_success{`+dnsExtJob+`}`).
					LegendFormat("{{instance}} External"),
				promQuery().
					Expr(`probe_success{`+dnsIntJob+`}`).
					LegendFormat("{{instance}} Internal"),
			))).
		Panel(uptimePanelDNSResponseTime, dashboardv2.NewPanelBuilder().
			Title("DNS Response Time").
			Description("How long each resolver took to answer the test query. External resolves a public name, internal an in-fleet one, so the two are expected to differ.").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend)).
			// Use total DNS probe duration; lookup_time measures resolution of the
			// probe target itself, not the test query.
			Data(queries(
				promQuery().
					Expr(`probe_duration_seconds{`+dnsExtJob+`}`).
					LegendFormat("{{instance}} External"),
				promQuery().
					Expr(`probe_duration_seconds{`+dnsIntJob+`}`).
					LegendFormat("{{instance}} Internal"),
			))).
		Build()
}
