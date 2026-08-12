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
			{Value: new(float64(1)), Color: "green"},
		})

	// Availability thresholds: red below 99%, yellow 99–99.9%, green at/above 99.9%.
	availabilityThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(99)), Color: "yellow"},
			{Value: new(float64(99.9)), Color: "green"},
		})

	// downThresholds: green at 0, red once any probe is down.
	downThresholds := issueThresholds()

	probeValueMappings := []dashboard.ValueMapping{
		{ValueMap: &dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				"0": {Text: new("DOWN"), Color: new("red")},
				"1": {Text: new("UP"), Color: new("green")},
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
				Span(6).Height(4).
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
				Span(6).Height(4).
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
				Description("Mean probe success across all ICMP targets over the dashboard's time range, so it moves with the zoom.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(avg_over_time(probe_success{` + icmpJob + `}[$__range])) * 100`).
					LegendFormat("availability"),
				),
		).
		// The Summary row was three tiles of span 8, with a headline availability
		// figure for ICMP and none for DNS -- the two "Devices Down" tiles beside it
		// treat the probe families as equals, so answering "how has DNS been?"
		// meant scrolling to the per-device bar gauge further down. Four tiles of
		// span 6 make the row symmetric and still total 24.
		WithPanel(
			stat.NewPanelBuilder().
				Title("DNS Availability (range)").
				Description("Mean probe success across all DNS probes, external and internal together, over the dashboard's time range.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Min(0).Max(100).
				Thresholds(availabilityThresholds).
				ColorMode(common.BigValueColorModeBackground).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`avg(avg_over_time(probe_success{` + dnsJobs + `}[$__range])) * 100`).
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
				Description("Round-trip time of the echo request itself, excluding the time blackbox spent resolving the target name and opening its socket.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					// phase="rtt", not probe_duration_seconds. The latter is the sum of
					// every phase, and the other two are not network latency: averaged
					// across the fifteen targets, probe_duration read 2.53 ms against an
					// actual rtt of 1.10 ms, with 0.58 ms of setup and 0.07 ms of
					// resolve making up the difference. A panel named for response time
					// was reporting 2.3x the round trip.
					Expr(`probe_icmp_duration_seconds{` + icmpJob + `, phase="rtt"}`).
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
				Description("How long each resolver took to answer the test query. External resolves a public name, internal an in-fleet one, so the two are expected to differ.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").
				Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				// probe_duration_seconds, not probe_dns_lookup_time_seconds. The
				// lookup_time metric is emitted by every blackbox prober and times the
				// resolution of the target's own hostname; for the dns prober that is
				// a step before the query under test, not the query. Measured together
				// it was 36 to 249 times smaller: dist2's external probe answered in
				// 16.0 ms while lookup_time reported 0.064 ms. That gap matters here,
				// because 16 ms against roughly 3 ms everywhere else is the one real
				// difference between these four probes, and the old metric flattened
				// it out of sight.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_duration_seconds{` + dnsExtJob + `}`).
					LegendFormat("{{instance}} External"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_duration_seconds{` + dnsIntJob + `}`).
					LegendFormat("{{instance}} Internal"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
