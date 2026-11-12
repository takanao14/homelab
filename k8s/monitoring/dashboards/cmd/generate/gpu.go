package main

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Panel IDs stay fixed across title and layout changes; do not reuse retired IDs.
const (
	gpuPanelGFXActivity            = 1
	gpuPanelVRAMUsage              = 2
	gpuPanelTemperatureEdge        = 3
	gpuPanelPowerAvg               = 4
	gpuPanelGPUHealthIssues        = 5
	gpuPanelECCUncorrectableErrors = 6
	gpuPanelTemperatureJunction    = 7
	gpuPanelGPUActivity            = 8
	gpuPanelVRAM                   = 9
	gpuPanelGTTMemory              = 10
	gpuPanelTemperature            = 11
	gpuPanelPower                  = 12
	gpuPanelClockSpeed             = 13
)

// buildGpuOverview covers the single AMD GPU.
// byGpu groups on stable gpu_id, hiding process and driver label churn that
// would split one card into discontinuous series. max is correct for gauges
// and prevents health counts from counting exporter label variants.
func buildGpuOverview() (*dashboardv2.Dashboard, error) {
	const gpuFilter = `job="scrapeConfig/monitoring/amd-gpu-external"`

	byGpu := func(expr string) string {
		return `max by (gpu_id) (` + expr + `)`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	gfxThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(80)), Color: "yellow"},
			{Value: new(float64(95)), Color: "red"},
		})

	vramThresholds := capacityThresholds()

	powerThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(120)), Color: "yellow"},
			{Value: new(float64(150)), Color: "red"},
		})

	// Edge temperature thresholds (°C): normal < 85, warm < 100, hot >= 100.
	tempThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(85)), Color: "yellow"},
			{Value: new(float64(100)), Color: "red"},
		})

	// Junction (hotspot) temperature is expected to run hotter than edge temperature.
	junctionTempThresholds := dashboardv2.NewThresholdsConfigBuilder().
		Mode(dashboardv2.ThresholdsModeAbsolute).
		Steps([]dashboardv2.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(95)), Color: "yellow"},
			{Value: new(float64(105)), Color: "red"},
		})

	issueThresholds := issueThresholds()

	d := dashboardv2.NewDashboardBuilder("GPU Overview").
		Tags([]string{"gpu", "infrastructure"}).
		TimeSettings(dashboardv2.NewTimeSettingsBuilder().Timezone("browser").From("now-1d").To("now").AutoRefresh("30s")).
		CursorSync(dashboardv2.DashboardCursorSyncCrosshair).
		DatasourceVariable(promDatasourceVariable())

	return newLayout(d).
		Row("Summary", statGrid(4)).
		Panel(gpuPanelGFXActivity, dashboardv2.NewPanelBuilder().
			Title("GFX Activity").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Thresholds(gfxThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_gfx_activity{`+gpuFilter+`}`)).
				LegendFormat("GFX Activity")))).
		Panel(gpuPanelVRAMUsage, dashboardv2.NewPanelBuilder().
			Title("VRAM Usage").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("percent").
				Min(0).
				Thresholds(vramThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				Decimals(1)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_used_vram{`+gpuFilter+`}`)+` / `+byGpu(`amd_gpu_total_vram{`+gpuFilter+`}`)+` * 100`).
				LegendFormat("VRAM Usage")))).
		// Edge temperature is the standard GPU die temperature metric.
		Panel(gpuPanelTemperatureEdge, dashboardv2.NewPanelBuilder().
			Title("Temperature (Edge)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("celsius").
				Thresholds(tempThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_edge_temperature{`+gpuFilter+`}`)).
				LegendFormat("Edge Temp")))).
		Panel(gpuPanelPowerAvg, dashboardv2.NewPanelBuilder().
			Title("Power (Avg)").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("watt").
				Min(0).
				Thresholds(powerThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_average_package_power{`+gpuFilter+`}`)).
				LegendFormat("Power")))).
		Row("GPU Health", statGrid(3)).
		Panel(gpuPanelGPUHealthIssues, dashboardv2.NewPanelBuilder().
			Title("GPU Health Issues").
			Description("Number of GPUs whose exporter health value is not healthy (1).").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`count(`+byGpu(`amd_gpu_health{`+gpuFilter+`}`)+` != 1) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Health Issues")))).
		Panel(gpuPanelECCUncorrectableErrors, dashboardv2.NewPanelBuilder().
			Title("ECC Uncorrectable Errors").
			Description("Total uncorrectable GPU ECC errors reported by the exporter.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(`sum(`+byGpu(`amd_gpu_ecc_uncorrect_total{`+gpuFilter+`}`)+`) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("ECC Errors")))).
		Panel(gpuPanelTemperatureJunction, dashboardv2.NewPanelBuilder().
			Title("Temperature (Junction)").
			Description("Current GPU hotspot temperature.").
			Visualization(stat.NewVisualizationV2Builder().
				Unit("celsius").
				Thresholds(junctionTempThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_junction_temperature{`+gpuFilter+`}`)).
				Instant(true).Range(false).
				LegendFormat("Junction Temp")))).
		Row("GPU Activity (%)", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// gfx=graphics/compute, umc=memory controller, vcn=video codec engine
		Panel(gpuPanelGPUActivity, dashboardv2.NewPanelBuilder().
			Title("GPU Activity (%)").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("percent").
				// Pin bounded utilization so idle zero remains meaningful.
				Min(0).
				Max(100).
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_gfx_activity{`+gpuFilter+`}`)).
				LegendFormat("GFX"),
				promQuery().
					Expr(byGpu(`amd_gpu_umc_activity{`+gpuFilter+`}`)).
					LegendFormat("Memory Controller"),
				promQuery().
					Expr(byGpu(`amd_gpu_vcn_activity{`+gpuFilter+`}`)).
					LegendFormat("Video Codec")))).
		Row("Memory", chartGrid()).
		// Metrics are in MiB; multiply to bytes so Grafana auto-scales the unit.
		Panel(gpuPanelVRAM, dashboardv2.NewPanelBuilder().
			Title("VRAM").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_used_vram{`+gpuFilter+`}`)+` * 1024 * 1024`).
				LegendFormat("Used"),
				promQuery().
					Expr(byGpu(`amd_gpu_total_vram{`+gpuFilter+`}`)+` * 1024 * 1024`).
					LegendFormat("Total")))).
		// GTT = GPU-accessible system RAM (graphics translation table).
		Panel(gpuPanelGTTMemory, dashboardv2.NewPanelBuilder().
			Title("GTT Memory").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("bytes").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_used_gtt{`+gpuFilter+`}`)+` * 1024 * 1024`).
				LegendFormat("Used"),
				promQuery().
					Expr(byGpu(`amd_gpu_total_gtt{`+gpuFilter+`}`)+` * 1024 * 1024`).
					LegendFormat("Total")))).
		Row("Temperature", wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard)).
		// edge=die edge, junction=hotspot (highest temp point), memory=VRAM temperature
		Panel(gpuPanelTemperature, dashboardv2.NewPanelBuilder().
			Title("Temperature").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("celsius").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_edge_temperature{`+gpuFilter+`}`)).
				LegendFormat("Edge"),
				promQuery().
					Expr(byGpu(`amd_gpu_junction_temperature{`+gpuFilter+`}`)).
					LegendFormat("Junction (Hotspot)"),
				promQuery().
					Expr(byGpu(`amd_gpu_memory_temperature{`+gpuFilter+`}`)).
					LegendFormat("Memory")))).
		Row("Power & Clocks", chartGrid()).
		Panel(gpuPanelPower, dashboardv2.NewPanelBuilder().
			Title("Power").
			Description("Average package power. The exporter publishes no instantaneous figure, so this is the only power reading available.").
			Visualization(timeseries.NewVisualizationV2Builder().
					Unit("watt").
					Tooltip(tooltipAll).
					Legend(legend)).
			Data(queries( // Only amd_gpu_average_package_power exists; label it as average rather
				// than implying a missing instantaneous series.
				promQuery().
					Expr(byGpu(`amd_gpu_average_package_power{`+gpuFilter+`}`)).
					LegendFormat("Average package power")))).
		// Metrics are in MHz; multiply to Hz for Grafana unit auto-scaling.
		Panel(gpuPanelClockSpeed, dashboardv2.NewPanelBuilder().
			Title("Clock Speed").
			Visualization(timeseries.NewVisualizationV2Builder().
				Unit("hertz").
				Tooltip(tooltipAll).
				Legend(legend)).
			Data(queries(promQuery().
				Expr(byGpu(`amd_gpu_clock{`+gpuFilter+`, clock_type="system"}`)+` * 1000 * 1000`).
				LegendFormat("GPU Core"),
				promQuery().
					Expr(byGpu(`amd_gpu_clock{`+gpuFilter+`, clock_type="memory"}`)+` * 1000 * 1000`).
					LegendFormat("Memory")))).
		Build()
}
