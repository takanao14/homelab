package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func buildUptime() (*dashboardv2.Dashboard, error) {
	ds := dashboardv2.NewDashboardv2DataQueryKindDatasourceBuilder().Name("$datasource")
	tooltipAll := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)
	legend := common.NewVizLegendOptionsBuilder().ShowLegend(true).DisplayMode(common.LegendDisplayModeList).Placement(common.LegendPlacementBottom)

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

	downThresholds := dashboardv2.NewThresholdsConfigBuilder().Mode(dashboardv2.ThresholdsModeAbsolute).Steps([]dashboardv2.Threshold{
		{Value: nil, Color: "green"},
		{Value: new(float64(1)), Color: "red"},
	})

	probeValueMappings := []dashboardv2.ValueMapping{
		{ValueMap: &dashboardv2.ValueMap{
			Type: "value",
			Options: map[string]dashboardv2.ValueMappingResult{
				"0": {Text: new("DOWN"), Color: new("red")},
				"1": {Text: new("UP"), Color: new("green")},
			},
		}},
	}

	d := dashboardv2.NewDashboardBuilder("Uptime V2 PoC").
		Tags([]string{"uptime", "infrastructure", "poc"}).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-30d").To("now").AutoRefresh("60s")).
		DatasourceVariable(dashboardv2.NewDatasourceVariableBuilder("datasource").Label("Datasource").PluginId("prometheus"))
	d.Panel("panel-2", dashboardv2.NewPanelBuilder().Id(2).
		Title("ICMP Devices Down").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(downThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`count(probe_success{`+icmpJob+`} == 0) or vector(0)`).
					LegendFormat("down")))))
	d.Panel("panel-3", dashboardv2.NewPanelBuilder().Id(3).
		Title("DNS Devices Down").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(downThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`count(probe_success{`+dnsJobs+`} == 0) or vector(0)`).
					LegendFormat("down")))))
	d.Panel("panel-4", dashboardv2.NewPanelBuilder().Id(4).
		Title("ICMP Availability (range)").
		Description("Mean probe success across all ICMP targets over the dashboard's time range, so it moves with the zoom.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`avg(avg_over_time(probe_success{`+icmpJob+`}[$__range])) * 100`).
					LegendFormat("availability")))))
	d.Panel("panel-5", dashboardv2.NewPanelBuilder().Id(5).
		Title("DNS Availability (range)").
		Description("Mean probe success across all DNS probes, external and internal together, over the dashboard's time range.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`avg(avg_over_time(probe_success{`+dnsJobs+`}[$__range])) * 100`).
					LegendFormat("availability")))))
	d.Panel("panel-7", dashboardv2.NewPanelBuilder().Id(7).
		Title("ICMP Status").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			GraphMode(common.BigValueGraphModeNone).
			Orientation(common.VizOrientationAuto).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_success{`+icmpJob+`}`).
					LegendFormat("{{instance}}")))))
	d.Panel("panel-8", dashboardv2.NewPanelBuilder().Id(8).
		Title("DNS External Status").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			GraphMode(common.BigValueGraphModeNone).
			Orientation(common.VizOrientationAuto).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_success{`+dnsExtJob+`}`).
					LegendFormat("{{instance}}")))))
	d.Panel("panel-9", dashboardv2.NewPanelBuilder().Id(9).
		Title("DNS Internal Status").
		Visualization(stat.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			GraphMode(common.BigValueGraphModeNone).
			Orientation(common.VizOrientationAuto).
			ColorMode(common.BigValueColorModeBackground)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_success{`+dnsIntJob+`}`).
					LegendFormat("{{instance}}")))))
	d.Panel("panel-11", dashboardv2.NewPanelBuilder().Id(11).
		Title("ICMP Availability by Device").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`avg_over_time(probe_success{`+icmpJob+`}[$__range]) * 100`).
					Instant(true).Range(false).
					LegendFormat("{{instance}}")))))
	d.Panel("panel-12", dashboardv2.NewPanelBuilder().Id(12).
		Title("DNS Availability by Device").
		Visualization(bargauge.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).Max(100).
			Thresholds(availabilityThresholds).
			Orientation(common.VizOrientationHorizontal).
			ReduceOptions(common.NewReduceDataOptionsBuilder().Values(true))).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`avg_over_time(probe_success{`+dnsExtJob+`}[$__range]) * 100`).
					Instant(true).Range(false).
					LegendFormat("{{instance}} External"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`avg_over_time(probe_success{`+dnsIntJob+`}[$__range]) * 100`).
					Instant(true).Range(false).
					LegendFormat("{{instance}} Internal")))))
	d.Panel("panel-14", dashboardv2.NewPanelBuilder().Id(14).
		Title("ICMP Status History").
		Visualization(statetimeline.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			ShowValue(common.VisibilityModeNever).
			MergeValues(true).
			Tooltip(tooltipAll)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_success{`+icmpJob+`}`).
					LegendFormat("{{instance}}")))))
	d.Panel("panel-15", dashboardv2.NewPanelBuilder().Id(15).
		Title("ICMP Response Time").
		Description("Round-trip time of the echo request itself, excluding the time blackbox spent resolving the target name and opening its socket.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					// Use the ICMP rtt phase; total probe duration also includes setup and DNS.
					Expr(`probe_icmp_duration_seconds{`+icmpJob+`, phase="rtt"}`).
					LegendFormat("{{instance}}")))))
	d.Panel("panel-17", dashboardv2.NewPanelBuilder().Id(17).
		Title("DNS Status History").
		Visualization(statetimeline.NewVisualizationV2Builder().
			Thresholds(probeThresholds).
			Mappings(probeValueMappings).
			ShowValue(common.VisibilityModeNever).
			MergeValues(true).
			Tooltip(tooltipAll)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_success{`+dnsExtJob+`}`).
					LegendFormat("{{instance}} External"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_success{`+dnsIntJob+`}`).
					LegendFormat("{{instance}} Internal")))))
	d.Panel("panel-18", dashboardv2.NewPanelBuilder().Id(18).
		Title("DNS Response Time").
		Description("How long each resolver took to answer the test query. External resolves a public name, internal an in-fleet one, so the two are expected to differ.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("s").
			Min(0).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().RefId("A").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_duration_seconds{`+dnsExtJob+`}`).
					LegendFormat("{{instance}} External"))).
			Target(dashboardv2.NewTargetBuilder().RefId("B").
				Query(prometheus.NewQueryV2Builder().Datasource(ds).
					Expr(`probe_duration_seconds{`+dnsIntJob+`}`).
					LegendFormat("{{instance}} Internal")))))
	d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().
			Title("Summary").
			GridLayout(dashboardv2.NewGridBuilder().
				Item(dashboardv2.GridItem("panel-2").X(0).Y(0).Width(6).Height(4)).
				Item(dashboardv2.GridItem("panel-3").X(6).Y(0).Width(6).Height(4)).
				Item(dashboardv2.GridItem("panel-4").X(12).Y(0).Width(6).Height(4)).
				Item(dashboardv2.GridItem("panel-5").X(18).Y(0).Width(6).Height(4)))).
		Row(dashboardv2.NewRowBuilder().
			Title("Current Status").
			GridLayout(dashboardv2.NewGridBuilder().
				Item(dashboardv2.GridItem("panel-7").X(0).Y(0).Width(24).Height(4)).
				Item(dashboardv2.GridItem("panel-8").X(0).Y(4).Width(12).Height(4)).
				Item(dashboardv2.GridItem("panel-9").X(12).Y(4).Width(12).Height(4)))).
		Row(dashboardv2.NewRowBuilder().
			Title("Availability").
			GridLayout(dashboardv2.NewGridBuilder().
				Item(dashboardv2.GridItem("panel-11").X(0).Y(0).Width(12).Height(8)).
				Item(dashboardv2.GridItem("panel-12").X(12).Y(0).Width(12).Height(8)))).
		Row(dashboardv2.NewRowBuilder().
			Title("ICMP Diagnostics").
			GridLayout(dashboardv2.NewGridBuilder().
				Item(dashboardv2.GridItem("panel-14").X(0).Y(0).Width(12).Height(8)).
				Item(dashboardv2.GridItem("panel-15").X(12).Y(0).Width(12).Height(8)))).
		Row(dashboardv2.NewRowBuilder().
			Title("DNS Diagnostics").
			GridLayout(dashboardv2.NewGridBuilder().
				Item(dashboardv2.GridItem("panel-17").X(0).Y(0).Width(12).Height(8)).
				Item(dashboardv2.GridItem("panel-18").X(12).Y(0).Width(12).Height(8)))))
	result, err := d.Build()
	return &result, err
}
