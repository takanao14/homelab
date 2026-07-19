package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildUptime defines the availability monitoring dashboard.
// blackbox-exporter probes return probe_success (1=up, 0=down).
// ScrapeConfig job label format: scrapeConfig/<namespace>/<name>.
func buildUptime() (*dashboard.Dashboard, error) {
	ds := promDatasource()
	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	const (
		icmpJob   = `job="scrapeConfig/monitoring/icmp-network-devices"`
		dnsExtJob = `job="scrapeConfig/monitoring/dns-external"`
		dnsIntJob = `job="scrapeConfig/monitoring/dns-internal"`
		dnsJobs   = `job=~"scrapeConfig/monitoring/dns-(external|internal)"`
	)

	// nil threshold Value means -Infinity (base step).
	probeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: float64Ptr(1), Color: "green"},
		})

	// Availability thresholds: red below 99%, yellow 99–99.9%, green at/above 99.9%.
	availabilityThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: float64Ptr(99), Color: "yellow"},
			{Value: float64Ptr(99.9), Color: "green"},
		})

	// downThresholds: green at 0, red once any probe is down.
	downThresholds := issueThresholds()

	probeValueMappings := []dashboard.ValueMapping{
		{ValueMap: &dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				"0": {Text: strPtr("DOWN"), Color: strPtr("red")},
				"1": {Text: strPtr("UP"), Color: strPtr("green")},
			},
		}},
	}

	d, err := dashboard.NewDashboardBuilder("Uptime").
		Uid("uptime").
		Tags([]string{"uptime", "infrastructure"}).
		Timezone("browser").
		Time("now-30d", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("ICMP Devices Down").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(downThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(probe_success{` + icmpJob + `} == 0) or vector(0)`).
					LegendFormat("down"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("DNS Devices Down").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(downThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(probe_success{` + dnsJobs + `} == 0) or vector(0)`).
					LegendFormat("down"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("ICMP Availability (range)").
				Datasource(ds).
				Span(8).Height(4).
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(avg_over_time(probe_success{` + icmpJob + `}[$__range])) * 100`).
					LegendFormat("availability"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Current Status")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("ICMP Status").
				Datasource(ds).
				Span(24).Height(4).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + icmpJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("DNS External Status").
				Datasource(ds).
				Span(12).Height(4).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsExtJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("DNS Internal Status").
				Datasource(ds).
				Span(12).Height(4).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				GraphMode(common.BigValueGraphModeNone).
				Orientation(common.VizOrientationAuto).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsIntJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Availability")).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("ICMP Availability by Device").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg_over_time(probe_success{` + icmpJob + `}[$__range]) * 100`).
					Instant().
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			bargauge.NewPanelBuilder().
				Title("DNS Availability by Device").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				Orientation(common.VizOrientationHorizontal).
				ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true)).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg_over_time(probe_success{` + dnsExtJob + `}[$__range]) * 100`).
					Instant().
					LegendFormat("{{instance}} External"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg_over_time(probe_success{` + dnsIntJob + `}[$__range]) * 100`).
					Instant().
					LegendFormat("{{instance}} Internal"),
				),
		).
		WithRow(dashboard.NewRowBuilder("ICMP Diagnostics")).
		WithPanel(
			statetimeline.NewPanelBuilder().
				Title("ICMP Status History").
				Datasource(ds).
				Span(12).Height(8).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + icmpJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("ICMP Response Time").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_duration_seconds{` + icmpJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithRow(dashboard.NewRowBuilder("DNS Diagnostics")).
		WithPanel(
			statetimeline.NewPanelBuilder().
				Title("DNS Status History").
				Datasource(ds).
				Span(12).Height(8).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsExtJob + `}`).
					LegendFormat("{{instance}} External"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_success{` + dnsIntJob + `}`).
					LegendFormat("{{instance}} Internal"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("DNS Response Time").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_dns_lookup_time_seconds{` + dnsExtJob + `}`).
					LegendFormat("{{instance}} External"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_dns_lookup_time_seconds{` + dnsIntJob + `}`).
					LegendFormat("{{instance}} Internal"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
