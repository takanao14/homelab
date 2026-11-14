package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	slackBotPanelSlackConnection    = 1
	slackBotPanelUptime             = 2
	slackBotPanelLEDSendFailures24h = 3
	slackBotPanelRenderFailures24h  = 4
	slackBotPanelRenderingDelivery  = 5
	slackBotPanelLEDSendLatency     = 6
	slackBotPanelMemory             = 7
	slackBotPanelGoroutines         = 8
)

// buildSlackBotOverview covers the failures the pod's own health cannot show:
// the process stays Ready while the Slack connection or the LED send is broken.
func buildSlackBotOverview() (*dashboardv2.Dashboard, error) {
	const (
		elementSlackConnection    = "slack-connection"
		elementUptime             = "uptime"
		elementLEDSendFailures24h = "led-send-failures-24h"
		elementRenderFailures24h  = "render-failures-24h"
		elementRenderingDelivery  = "rendering-delivery"
		elementLEDSendLatency     = "led-send-latency"
		elementMemory             = "memory"
		elementGoroutines         = "goroutines"
	)

	const clusterFilter = `cluster=~"$cluster"`
	const jobFilter = `job="slack-bot",cluster=~"$cluster"`

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()
	measurementThresholds := measurementThresholds()
	connectionThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "red"},
			{Value: new(float64(1)), Color: "green"},
		})
	connectionMappings := []dashboardv2.ValueMapping{
		{ValueMap: &dashboardv2.ValueMap{
			Type: dashboardv2.MappingTypeValue,
			Options: map[string]dashboardv2.ValueMappingResult{
				"0": {Text: new("DISCONNECTED"), Color: new("red")},
				"1": {Text: new("CONNECTED"), Color: new("green")},
			},
		}},
	}

	d := dashboardv2.NewDashboardBuilder("slack-bot Overview").
		Tags([]string{"slack-bot", "slack", "led"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-7d").To("now").AutoRefresh("5m")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").
			Label("Cluster").
			Query(promVariableQuery(`label_values(kube_node_info, cluster)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	d.Panel(elementSlackConnection, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelSlackConnection).
		Title("Slack Connection").
		Description("Whether the Socket Mode connection is established. The pod stays Ready and /healthz keeps returning 200 while this is 0, so nothing else reports the outage. No data means the bot is not deployed to the selected cluster.").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(connectionThresholds).
			Mappings(connectionMappings).
			ColorMode(common.BigValueColorModeBackground).
			GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// Single replica: max avoids double counting an overlapping rollout.
				Expr(`max by (cluster) (slack_bot_socket_connected{`+jobFilter+`})`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementUptime, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelUptime).
		Title("Uptime").
		Description("Time since the process started. A reset that nobody deployed points at a liveness probe failure or an OOM kill.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("s").
			Thresholds(measurementThresholds).
			ColorMode(common.BigValueColorModeNone).
			GraphMode(common.BigValueGraphModeNone)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`time() - max by (cluster) (process_start_time_seconds{`+jobFilter+`})`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementLEDSendFailures24h, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelLEDSendFailures24h).
		Title("LED Send Failures (24h)").
		Description("Failed image sends in the last 24 hours. Check the led-rpi3 TCP probe first: if that is healthy the port is open and the failure is in the send itself.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(slack_bot_led_send_total{`+jobFilter+`,result="error"}[24h]))) or vector(0)`).
				LegendFormat("Failures")))))

	d.Panel(elementRenderFailures24h, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelRenderFailures24h).
		Title("Render Failures (24h)").
		Description("Messages that could not be rendered to an image in the last 24 hours, before any LED send was attempted. Font and emoji problems land here.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`ceil(sum(increase(slack_bot_render_failures_total{`+jobFilter+`}[24h]))) or vector(0)`).
				LegendFormat("Failures")))))

	d.Panel(elementRenderingDelivery, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelRenderingDelivery).
		Title("Rendering & Delivery").
		Description("Messages rendered and images delivered per interval. Rendered without a matching delivery means the LED path dropped the image; the two lines otherwise track each other.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			DrawStyle(common.GraphDrawStyleBars).
			FillOpacity(100).
			GradientMode(common.GraphGradientModeHue).
			Tooltip(tooltipAll).
			Legend(legend).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* failed"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`round(sum by (cluster) (increase(slack_bot_messages_rendered_total{`+jobFilter+`}[$__interval])))`).
				LegendFormat("{{cluster}} rendered"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`round(sum by (cluster) (increase(slack_bot_led_send_total{`+jobFilter+`,result="ok"}[$__interval])))`).
				LegendFormat("{{cluster}} delivered"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`round(sum by (cluster) (increase(slack_bot_led_send_total{`+jobFilter+`,result="error"}[$__interval])))`).
				LegendFormat("{{cluster}} failed"))). // Channel traffic is sparse; count per bucket instead of a per-second
			// rate, and floor the bucket so single messages stay visible.
			QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1h"))))

	d.Panel(elementLEDSendLatency, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelLEDSendLatency).
		Title("LED Send Latency").
		Description("Time spent in an image send. Sends are rare, so samples are drawn as points and a gap means no message was sent, not a failure. The histogram buckets reach 640ms while observed sends complete in single-digit milliseconds; keep the range until a slow device proves which resolution matters.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			ShowPoints(common.VisibilityModeAlways).
			PointSize(6).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`histogram_quantile(0.95, sum by (cluster, le) (rate(slack_bot_led_send_duration_seconds_bucket{`+jobFilter+`}[$__rate_interval])))`).
				LegendFormat("{{cluster}} p95"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				// The mean stays readable at one or two samples, where a quantile
				// only reports its bucket boundary.
				Expr(`sum by (cluster) (rate(slack_bot_led_send_duration_seconds_sum{`+jobFilter+`}[$__rate_interval]))`+
									` / sum by (cluster) (rate(slack_bot_led_send_duration_seconds_count{`+jobFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} mean"))). // Floor the window so a sparse histogram keeps samples per bucket.
			QueryOptions(dashboardv2.NewQueryOptionsSpecBuilder().Interval("1h"))))

	d.Panel(elementMemory, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelMemory).
		Title("Memory").
		Description("Heap in use against the process resident set. The emoji list and image caches hold entries for 24 hours, so a sawtooth is expected and a monotonic climb is not.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("bytes").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`max by (cluster) (go_memstats_heap_inuse_bytes{`+jobFilter+`})`).
				LegendFormat("{{cluster}} heap in use"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`max by (cluster) (process_resident_memory_bytes{`+jobFilter+`})`).
				LegendFormat("{{cluster}} resident")))))

	d.Panel(elementGoroutines, dashboardv2.NewPanelBuilder().
		Id(slackBotPanelGoroutines).
		Title("Goroutines").
		Description("Live goroutines. Each Slack event is handled on the event loop, so a count that grows with traffic and never returns marks a leaked handler.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`max by (cluster) (go_goroutines{`+jobFilter+`})`).
				LegendFormat("{{cluster}}")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Status").AutoGridLayout(statGrid(4).
			WithItem(elementSlackConnection).
			WithItem(elementUptime).
			WithItem(elementLEDSendFailures24h).
			WithItem(elementRenderFailures24h))).
		Row(dashboardv2.NewRowBuilder().Title("Message Delivery").AutoGridLayout(chartGrid().
			WithItem(elementRenderingDelivery).
			WithItem(elementLEDSendLatency))).
		Row(dashboardv2.NewRowBuilder().Title("Process").AutoGridLayout(chartGrid().
			WithItem(elementMemory).
			WithItem(elementGoroutines)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
