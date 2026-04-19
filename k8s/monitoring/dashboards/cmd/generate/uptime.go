package main

import (
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

	const (
		icmpJob   = `job="scrapeConfig/monitoring/icmp-network-devices"`
		dnsExtJob = `job="scrapeConfig/monitoring/dns-external"`
		dnsIntJob = `job="scrapeConfig/monitoring/dns-internal"`
	)

	// nil threshold Value means -Infinity (base step).
	probeThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: float64Ptr(1), Color: "green"},
		})

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
		Time("now-1d", "now").
		Refresh("60s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
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
		WithPanel(
			statetimeline.NewPanelBuilder().
				Title("ICMP Status History").
				Datasource(ds).
				Span(12).Height(8).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
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
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`probe_duration_seconds{` + icmpJob + `}`).
					LegendFormat("{{instance}}"),
				),
		).
		WithPanel(
			statetimeline.NewPanelBuilder().
				Title("DNS Status History").
				Datasource(ds).
				Span(12).Height(8).
				Thresholds(probeThresholds).
				Mappings(probeValueMappings).
				ShowValue(common.VisibilityModeNever).
				MergeValues(true).
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
