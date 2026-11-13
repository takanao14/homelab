package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	ciliumPanelEndpointsNotReady              = 1
	ciliumPanelControllersFailing             = 2
	ciliumPanelUnreachableNodes               = 3
	ciliumPanelAgentErrors5m                  = 4
	ciliumPanelHubbleLostEvents5m             = 5
	ciliumPanelHubbleLostEventsByNodeSource5m = 6
	ciliumPanelDropRateByReason               = 7
	ciliumPanelHubbleFlowVerdicts             = 8
	ciliumPanelHubbleDropsByReason            = 9
	ciliumPanelEndpointState                  = 10
	ciliumPanelEndpointRegenerations          = 11
	ciliumPanelAgentErrorsWarnings            = 12
	ciliumPanelBPFMapPressure                 = 13
	ciliumPanelConntrackGCEntries             = 14
	ciliumPanelHubbleFlowsByType              = 15
)

// buildCiliumOverview covers Cilium health, drops, policy, BPF maps, endpoints,
// and Hubble. Trends use $__rate_interval; named five-minute KPIs stay fixed.
func buildCiliumOverview() (*dashboardv2.Dashboard, error) {
	const (
		elementEndpointsNotReady              = "endpoints-not-ready"
		elementControllersFailing             = "controllers-failing"
		elementUnreachableNodes               = "unreachable-nodes"
		elementAgentErrors5m                  = "agent-errors-5m"
		elementHubbleLostEvents5m             = "hubble-lost-events-5m"
		elementHubbleLostEventsByNodeSource5m = "hubble-lost-events-by-node-source-5m"
		elementDropRateByReason               = "drop-rate-by-reason"
		elementHubbleFlowVerdicts             = "hubble-flow-verdicts"
		elementHubbleDropsByReason            = "hubble-drops-by-reason"
		elementEndpointState                  = "endpoint-state"
		elementEndpointRegenerations          = "endpoint-regenerations"
		elementAgentErrorsWarnings            = "agent-errors-warnings"
		elementBPFMapPressure                 = "bpf-map-pressure"
		elementConntrackGCEntries             = "conntrack-gc-entries"
		elementHubbleFlowsByType              = "hubble-flows-by-type"
	)

	const clusterFilter = `cluster=~"$cluster"`

	// Render dominant benign unsupported-L3 drops grey and policy denials red
	// so operationally relevant drops stand out.
	const benignDropColor = "#808080"

	tooltipAll := defaultTooltip()
	legend := defaultLegend()
	issueThresholds := issueThresholds()
	capacityThresholds := capacityThresholds()

	d := dashboardv2.NewDashboardBuilder("Cilium Overview").
		Tags([]string{"cilium", "cni", "network", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-6h").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable()).
		QueryVariable(dashboardv2.NewQueryVariableBuilder("cluster").
			Label("Cluster").
			Query(promVariableQuery(`label_values(cilium_endpoint_state, cluster)`)).
			Refresh(dashboardv2.VariableRefreshOnTimeRangeChanged).
			Sort(dashboardv2.VariableSortAlphabeticalAsc).
			Multi(true).
			IncludeAll(true))

	d.Panel(elementEndpointsNotReady, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelEndpointsNotReady).
		Title("Endpoints Not Ready").
		Description("Cilium endpoints not in the ready state.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(cilium_endpoint_state{`+clusterFilter+`,endpoint_state!="ready"}) or vector(0)`).
				LegendFormat("Not ready")))))

	d.Panel(elementControllersFailing, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelControllersFailing).
		Title("Controllers Failing").
		Description("Cilium controllers currently failing their reconciliation runs.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(cilium_controllers_failing{`+clusterFilter+`}) or vector(0)`).
				LegendFormat("Failing")))))

	d.Panel(elementUnreachableNodes, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelUnreachableNodes).
		Title("Unreachable Nodes").
		Description("Nodes Cilium cannot reach via the health checker.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum(cilium_unreachable_nodes{`+clusterFilter+`}) or vector(0)`).
				LegendFormat("Unreachable")))))

	d.Panel(elementAgentErrors5m, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelAgentErrors5m).
		Title("Agent Errors (5m)").
		Description("Cilium agent log messages at ERROR level in the last 5 minutes.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`ceil(sum(increase(cilium_errors_warnings_total{`+clusterFilter+`,level="ERROR"}[5m]))) or vector(0)`).
				LegendFormat("Errors")))))

	d.Panel(elementHubbleLostEvents5m, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelHubbleLostEvents5m).
		Title("Hubble Lost Events (5m)").
		Description("Hubble events lost at any processing stage in the last 5 minutes. See the node/source breakdown below for the cause.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`ceil(sum(increase(hubble_lost_events_total{`+clusterFilter+`}[5m]))) or vector(0)`).
				LegendFormat("Lost")))))

	d.Panel(elementHubbleLostEventsByNodeSource5m, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelHubbleLostEventsByNodeSource5m).
		Title("Hubble Lost Events by Node / Source (5m)").
		Description("Rolling 5-minute lost event count by node and pipeline stage. hubble_ring_buffer indicates a slow reader; observer_events_queue indicates Hubble processing backlog; perf_event_ring_buffer indicates datapath-to-userspace loss.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`ceil(sum by (cluster, node, source) (increase(hubble_lost_events_total{`+clusterFilter+`}[5m])))`).
				LegendFormat("{{cluster}} {{node}} {{source}}")))))

	d.Panel(elementDropRateByReason, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelDropRateByReason).
		Title("Drop Rate by Reason").
		Description("Packets dropped by the datapath, by reason. Unsupported L3 protocol is benign background noise and is drawn in grey; any coloured line is worth reading.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("pps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Unsupported L3 protocol"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": benignDropColor}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* Policy denied"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster, reason) (rate(cilium_drop_count_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{reason}}")))))

	d.Panel(elementHubbleFlowVerdicts, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelHubbleFlowVerdicts).
		Title("Hubble Flow Verdicts").
		Description("Observed network flows by verdict. DROPPED flows indicate policy denials or datapath drops.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("pps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster, verdict) (rate(hubble_flows_processed_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{verdict}}")))))

	d.Panel(elementHubbleDropsByReason, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelHubbleDropsByReason).
		Title("Hubble Drops by Reason").
		Description("Dropped flows observed by Hubble, grouped by reason. Same colour rule as Drop Rate by Reason: grey is benign, coloured is not. Hubble spells the reasons in upper snake case.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("pps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* UNSUPPORTED_L3_PROTOCOL"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": benignDropColor}},
			}).
			Override(dashboardv2.MatcherConfig{Id: "byRegexp", Options: ".* POLICY_DENIED"}, []dashboardv2.DynamicConfigValue{
				{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
			})).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster, reason) (rate(hubble_drop_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{reason}}")))))

	d.Panel(elementEndpointState, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelEndpointState).
		Title("Endpoint State").
		Description("Cilium endpoints by state. Anything other than 'ready' should be transient.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster, endpoint_state) (cilium_endpoint_state{`+clusterFilter+`})`).
				LegendFormat("{{cluster}} {{endpoint_state}}")))))

	d.Panel(elementEndpointRegenerations, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelEndpointRegenerations).
		Title("Endpoint Regenerations").
		Description("Rate of endpoint datapath regenerations. Spikes accompany policy or identity churn.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster) (rate(cilium_endpoint_regenerations_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementAgentErrorsWarnings, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelAgentErrorsWarnings).
		Title("Agent Errors & Warnings").
		Description("Rate of Cilium agent log messages by level.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("ops").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster, level) (rate(cilium_errors_warnings_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{level}}")))))

	d.Panel(elementBPFMapPressure, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelBPFMapPressure).
		// topk bounds future map growth; the title does not promise ten results.
		Title("BPF Map Pressure").
		Description("Fill ratio of the busiest eBPF maps. Sustained high pressure (>80%) risks map exhaustion and packet drops.").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Max(100).
			Orientation(common.VizOrientationHorizontal).
			Thresholds(capacityThresholds).
			Decimals(2)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sort_desc(topk(10, 100 * max by (cluster, map_name) (cilium_bpf_map_pressure{`+clusterFilter+`})))`).
				Instant(true).Range(false).
				LegendFormat("{{cluster}} {{map_name}}")))))

	d.Panel(elementConntrackGCEntries, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelConntrackGCEntries).
		Title("Conntrack GC Entries").
		Description("Connection-tracking entries seen by the last garbage-collection run.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster) (cilium_datapath_conntrack_gc_entries{`+clusterFilter+`})`).
				LegendFormat("{{cluster}}")))))

	d.Panel(elementHubbleFlowsByType, dashboardv2.NewPanelBuilder().
		Id(ciliumPanelHubbleFlowsByType).
		Title("Hubble Flows by Type").
		Description("Rate of observed flows by type (Trace, Drop, L7, etc.).").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("pps").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").Query(promQuery().
				Expr(`sum by (cluster, type) (rate(hubble_flows_processed_total{`+clusterFilter+`}[$__rate_interval]))`).
				LegendFormat("{{cluster}} {{type}}")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(3).
			WithItem(elementEndpointsNotReady).
			WithItem(elementControllersFailing).
			WithItem(elementUnreachableNodes))).
		Row(dashboardv2.NewRowBuilder().Title("Event Errors").AutoGridLayout(statGrid(2).
			WithItem(elementAgentErrors5m).
			WithItem(elementHubbleLostEvents5m))).
		Row(dashboardv2.NewRowBuilder().Title("Hubble Diagnostics").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementHubbleLostEventsByNodeSource5m))).
		Row(dashboardv2.NewRowBuilder().Title("Drops & Policy").AutoGridLayout(chartGrid().
			WithItem(elementDropRateByReason).
			WithItem(elementHubbleFlowVerdicts))).
		Row(dashboardv2.NewRowBuilder().Title("Hubble Drops by Reason").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementHubbleDropsByReason))).
		Row(dashboardv2.NewRowBuilder().Title("Endpoints & Agent Health").AutoGridLayout(chartGrid().
			WithItem(elementEndpointState).
			WithItem(elementEndpointRegenerations).
			WithItem(elementAgentErrorsWarnings))).
		Row(dashboardv2.NewRowBuilder().Title("BPF Datapath & Hubble Flows").AutoGridLayout(chartGrid().
			WithItem(elementBPFMapPressure).
			WithItem(elementConntrackGCEntries).
			WithItem(elementHubbleFlowsByType)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
