package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildSlackBotOverview covers the failures the pod's own health cannot show:
// the process stays Ready while the Slack connection or the LED send is broken.
func buildSlackBotOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const clusterFilter = `cluster=~"$cluster"`
	const jobFilter = `job="slack-bot",cluster=~"$cluster"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()
	measurementThresholds := measurementThresholds()
	connectionThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})
	connectionMappings := []dashboard.ValueMapping{
		{ValueMap: &dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				"0": {Text: new("DISCONNECTED"), Color: new("red")},
				"1": {Text: new("CONNECTED"), Color: new("green")},
			},
		}},
	}

	d, err := dashboard.NewDashboardBuilder("slack-bot Overview").
		Uid("slack-bot-overview").
		Tags([]string{"slack-bot", "slack", "led"}).
		Timezone("browser").
		Time("now-7d", "now").
		Refresh("5m").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(promDatasourceVariable()).
		WithVariable(
			dashboard.NewQueryVariableBuilder("cluster").
				Label("Cluster").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: new(`label_values(kube_node_info, cluster)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(true).
				IncludeAll(true),
		).
		WithRow(dashboard.NewRowBuilder("Status")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Slack Connection").
				Description("Whether the Socket Mode connection is established. The pod stays Ready and /healthz keeps returning 200 while this is 0, so nothing else reports the outage. No data means the bot is not deployed to the selected cluster.").
				Datasource(ds).
				Span(6).Height(4).
				Thresholds(connectionThresholds).
				Mappings(connectionMappings).
				ColorMode(common.BigValueColorModeBackground).
				GraphMode(common.BigValueGraphModeNone).
				WithTarget(prometheus.NewDataqueryBuilder().
					// Single replica: max avoids double counting an overlapping rollout.
					Expr(`max by (cluster) (slack_bot_socket_connected{` + jobFilter + `})`).
					Instant().
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Description("Time since the process started. A reset that nobody deployed points at a liveness probe failure or an OOM kill.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				Thresholds(measurementThresholds).
				ColorMode(common.BigValueColorModeNone).
				GraphMode(common.BigValueGraphModeNone).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`time() - max by (cluster) (process_start_time_seconds{` + jobFilter + `})`).
					Instant().
					LegendFormat("{{cluster}}"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("LED Send Failures (24h)").
				Description("Failed image sends in the last 24 hours. Check the led-rpi3 TCP probe first: if that is healthy the port is open and the failure is in the send itself.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(slack_bot_led_send_total{` + jobFilter + `,result="error"}[24h]))) or vector(0)`).
					LegendFormat("Failures"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Render Failures (24h)").
				Description("Messages that could not be rendered to an image in the last 24 hours, before any LED send was attempted. Font and emoji problems land here.").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`ceil(sum(increase(slack_bot_render_failures_total{` + jobFilter + `}[24h]))) or vector(0)`).
					LegendFormat("Failures"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Message Delivery")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Rendering & Delivery").
				Description("Messages rendered and images delivered per interval. Rendered without a matching delivery means the LED path dropped the image; the two lines otherwise track each other.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("short").
				// Channel traffic is sparse; count per bucket instead of a per-second
				// rate, and floor the bucket so single messages stay visible.
				Interval("1h").
				DrawStyle(common.GraphDrawStyleBars).
				FillOpacity(100).
				GradientMode(common.GraphGradientModeHue).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`round(sum by (cluster) (increase(slack_bot_messages_rendered_total{`+jobFilter+`}[$__interval])))`).
					LegendFormat("{{cluster}} rendered"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`round(sum by (cluster) (increase(slack_bot_led_send_total{`+jobFilter+`,result="ok"}[$__interval])))`).
					LegendFormat("{{cluster}} delivered"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`round(sum by (cluster) (increase(slack_bot_led_send_total{`+jobFilter+`,result="error"}[$__interval])))`).
					LegendFormat("{{cluster}} failed"),
				).
				WithOverride(dashboard.MatcherConfig{Id: "byRegexp", Options: ".* failed"}, []dashboard.DynamicConfigValue{
					{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
				}),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("LED Send Latency").
				Description("Time spent in an image send. Sends are rare, so samples are drawn as points and a gap means no message was sent, not a failure. The histogram buckets reach 640ms while observed sends complete in single-digit milliseconds; keep the range until a slow device proves which resolution matters.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("s").Min(0).
				// Floor the window so a sparse histogram keeps samples per bucket.
				Interval("1h").
				ShowPoints(common.VisibilityModeAlways).
				PointSize(6).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`histogram_quantile(0.95, sum by (cluster, le) (rate(slack_bot_led_send_duration_seconds_bucket{` + jobFilter + `}[$__rate_interval])))`).
					LegendFormat("{{cluster}} p95"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					// The mean stays readable at one or two samples, where a quantile
					// only reports its bucket boundary.
					Expr(`sum by (cluster) (rate(slack_bot_led_send_duration_seconds_sum{` + jobFilter + `}[$__rate_interval]))` +
						` / sum by (cluster) (rate(slack_bot_led_send_duration_seconds_count{` + jobFilter + `}[$__rate_interval]))`).
					LegendFormat("{{cluster}} mean"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Process")).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory").
				Description("Heap in use against the process resident set. The emoji list and image caches hold entries for 24 hours, so a sawtooth is expected and a monotonic climb is not.").
				Datasource(ds).
				Span(12).Height(6).
				Unit("bytes").Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max by (cluster) (go_memstats_heap_inuse_bytes{` + jobFilter + `})`).
					LegendFormat("{{cluster}} heap in use"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max by (cluster) (process_resident_memory_bytes{` + jobFilter + `})`).
					LegendFormat("{{cluster}} resident"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Goroutines").
				Description("Live goroutines. Each Slack event is handled on the event loop, so a count that grows with traffic and never returns marks a leaked handler.").
				Datasource(ds).
				Span(12).Height(6).
				Unit("short").Min(0).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`max by (cluster) (go_goroutines{` + jobFilter + `})`).
					LegendFormat("{{cluster}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
