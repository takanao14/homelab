package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// buildGpuOverview defines the AMD GPU dashboard for the single RX 9060 XT on gpuvm.
// No variables needed; job label is sufficient to target the single GPU.
func buildGpuOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const gpuFilter = `job="scrapeConfig/monitoring/amd-gpu-external"`

	tooltipAll := common.NewVizTooltipOptionsBuilder().Mode(common.TooltipDisplayModeMulti)

	gfxThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(80), Color: "yellow"},
			{Value: float64Ptr(95), Color: "red"},
		})

	vramThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(80), Color: "yellow"},
			{Value: float64Ptr(90), Color: "red"},
		})

	powerThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(120), Color: "yellow"},
			{Value: float64Ptr(150), Color: "red"},
		})

	// Edge temperature thresholds (°C): normal < 85, warm < 100, hot >= 100.
	tempThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: float64Ptr(85), Color: "yellow"},
			{Value: float64Ptr(100), Color: "red"},
		})

	d, err := dashboard.NewDashboardBuilder("GPU Overview").
		Uid("gpu-overview").
		Tags([]string{"gpu", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("GFX Activity").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Thresholds(gfxThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_gfx_activity{` + gpuFilter + `}`).
					LegendFormat("GFX Activity"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("VRAM Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Thresholds(vramThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_used_vram{` + gpuFilter + `} / amd_gpu_total_vram{` + gpuFilter + `} * 100`).
					LegendFormat("VRAM Usage"),
				).Decimals(1),
		).
		// Edge temperature is the standard GPU die temperature metric.
		WithPanel(
			stat.NewPanelBuilder().
				Title("Temperature (Edge)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("celsius").
				Thresholds(tempThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_edge_temperature{` + gpuFilter + `}`).
					LegendFormat("Edge Temp"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power (Avg)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("watt").
				Thresholds(powerThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_average_package_power{` + gpuFilter + `}`).
					LegendFormat("Power"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Metrics")).
		// gfx=graphics/compute, umc=memory controller, vcn=video codec engine
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("GPU Activity (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_gfx_activity{` + gpuFilter + `}`).
					LegendFormat("GFX"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_umc_activity{` + gpuFilter + `}`).
					LegendFormat("Memory Controller"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_vcn_activity{` + gpuFilter + `}`).
					LegendFormat("Video Codec"),
				),
		).
		// Metrics are in MiB; multiply to bytes so Grafana auto-scales the unit.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("VRAM").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_used_vram{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_total_vram{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Total"),
				),
		).
		// GTT = GPU-accessible system RAM (graphics translation table).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("GTT Memory").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_used_gtt{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_total_gtt{` + gpuFilter + `} * 1024 * 1024`).
					LegendFormat("Total"),
				),
		).
		// edge=die edge, junction=hotspot (highest temp point), memory=VRAM temperature
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Temperature").
				Datasource(ds).
				Span(24).Height(8).
				Unit("celsius").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_edge_temperature{` + gpuFilter + `}`).
					LegendFormat("Edge"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_junction_temperature{` + gpuFilter + `}`).
					LegendFormat("Junction (Hotspot)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_memory_temperature{` + gpuFilter + `}`).
					LegendFormat("Memory"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Power").
				Datasource(ds).
				Span(12).Height(8).
				Unit("watt").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_power_usage{` + gpuFilter + `}`).
					LegendFormat("Current"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_average_package_power{` + gpuFilter + `}`).
					LegendFormat("Average"),
				),
		).
		// Metrics are in MHz; multiply to Hz for Grafana unit auto-scaling.
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Clock Speed").
				Datasource(ds).
				Span(12).Height(8).
				Unit("hertz").
				Tooltip(tooltipAll).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_clock{` + gpuFilter + `, clock_type="GPU_CLOCK_TYPE_SYSTEM"} * 1000 * 1000`).
					LegendFormat("GPU Core"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`amd_gpu_clock{` + gpuFilter + `, clock_type="GPU_CLOCK_TYPE_MEMORY"} * 1000 * 1000`).
					LegendFormat("Memory"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
