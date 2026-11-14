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
	const (
		elementGFXActivity            = "gfx-activity"
		elementVRAMUsage              = "vram-usage"
		elementTemperatureEdge        = "temperature-edge"
		elementPowerAvg               = "power-avg"
		elementGPUHealthIssues        = "gpu-health-issues"
		elementECCUncorrectableErrors = "ecc-uncorrectable-errors"
		elementTemperatureJunction    = "temperature-junction"
		elementGPUActivity            = "gpu-activity"
		elementVRAM                   = "vram"
		elementGTTMemory              = "gtt-memory"
		elementTemperature            = "temperature"
		elementPower                  = "power"
		elementClockSpeed             = "clock-speed"
	)

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

	d.Panel(elementGFXActivity, dashboardv2.NewPanelBuilder().
		Id(gpuPanelGFXActivity).
		Title("GFX Activity").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Thresholds(gfxThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_gfx_activity{`+gpuFilter+`}`)).
				LegendFormat("GFX Activity")))))

	d.Panel(elementVRAMUsage, dashboardv2.NewPanelBuilder().
		Id(gpuPanelVRAMUsage).
		Title("VRAM Usage").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("percent").
			Min(0).
			Thresholds(vramThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto).
			Decimals(1)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_used_vram{`+gpuFilter+`}`)+` / `+byGpu(`amd_gpu_total_vram{`+gpuFilter+`}`)+` * 100`).
				LegendFormat("VRAM Usage")))))

	// Edge temperature is the standard GPU die temperature metric.
	d.Panel(elementTemperatureEdge, dashboardv2.NewPanelBuilder().
		Id(gpuPanelTemperatureEdge).
		Title("Temperature (Edge)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("celsius").
			Thresholds(tempThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_edge_temperature{`+gpuFilter+`}`)).
				LegendFormat("Edge Temp")))))

	d.Panel(elementPowerAvg, dashboardv2.NewPanelBuilder().
		Id(gpuPanelPowerAvg).
		Title("Power (Avg)").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("watt").
			Min(0).
			Thresholds(powerThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_average_package_power{`+gpuFilter+`}`)).
				LegendFormat("Power")))))

	d.Panel(elementGPUHealthIssues, dashboardv2.NewPanelBuilder().
		Id(gpuPanelGPUHealthIssues).
		Title("GPU Health Issues").
		Description("Number of GPUs whose exporter health value is not healthy (1).").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`count(`+byGpu(`amd_gpu_health{`+gpuFilter+`}`)+` != 1) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("Health Issues")))))

	d.Panel(elementECCUncorrectableErrors, dashboardv2.NewPanelBuilder().
		Id(gpuPanelECCUncorrectableErrors).
		Title("ECC Uncorrectable Errors").
		Description("Total uncorrectable GPU ECC errors reported by the exporter.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("short").
			Min(0).
			Thresholds(issueThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(`sum(`+byGpu(`amd_gpu_ecc_uncorrect_total{`+gpuFilter+`}`)+`) or vector(0)`).
				Instant(true).Range(false).
				LegendFormat("ECC Errors")))))

	d.Panel(elementTemperatureJunction, dashboardv2.NewPanelBuilder().
		Id(gpuPanelTemperatureJunction).
		Title("Temperature (Junction)").
		Description("Current GPU hotspot temperature.").
		Visualization(stat.NewVisualizationV2Builder().
			Unit("celsius").
			Thresholds(junctionTempThresholds).
			ColorMode(common.BigValueColorModeBackground).
			Orientation(common.VizOrientationAuto)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_junction_temperature{`+gpuFilter+`}`)).
				Instant(true).Range(false).
				LegendFormat("Junction Temp")))))

	// gfx=graphics/compute, umc=memory controller, vcn=video codec engine
	d.Panel(elementGPUActivity, dashboardv2.NewPanelBuilder().
		Id(gpuPanelGPUActivity).
		Title("GPU Activity (%)").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("percent").
			// Pin bounded utilization so idle zero remains meaningful.
			Min(0).
			Max(100).
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_gfx_activity{`+gpuFilter+`}`)).
				LegendFormat("GFX"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_umc_activity{`+gpuFilter+`}`)).
				LegendFormat("Memory Controller"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_vcn_activity{`+gpuFilter+`}`)).
				LegendFormat("Video Codec")))))

	// Metrics are in MiB; multiply to bytes so Grafana auto-scales the unit.
	d.Panel(elementVRAM, dashboardv2.NewPanelBuilder().
		Id(gpuPanelVRAM).
		Title("VRAM").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("bytes").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_used_vram{`+gpuFilter+`}`)+` * 1024 * 1024`).
				LegendFormat("Used"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_total_vram{`+gpuFilter+`}`)+` * 1024 * 1024`).
				LegendFormat("Total")))))

	// GTT = GPU-accessible system RAM (graphics translation table).
	d.Panel(elementGTTMemory, dashboardv2.NewPanelBuilder().
		Id(gpuPanelGTTMemory).
		Title("GTT Memory").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("bytes").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_used_gtt{`+gpuFilter+`}`)+` * 1024 * 1024`).
				LegendFormat("Used"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_total_gtt{`+gpuFilter+`}`)+` * 1024 * 1024`).
				LegendFormat("Total")))))

	// edge=die edge, junction=hotspot (highest temp point), memory=VRAM temperature
	d.Panel(elementTemperature, dashboardv2.NewPanelBuilder().
		Id(gpuPanelTemperature).
		Title("Temperature").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("celsius").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_edge_temperature{`+gpuFilter+`}`)).
				LegendFormat("Edge"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_junction_temperature{`+gpuFilter+`}`)).
				LegendFormat("Junction (Hotspot)"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_memory_temperature{`+gpuFilter+`}`)).
				LegendFormat("Memory")))))

	d.Panel(elementPower, dashboardv2.NewPanelBuilder().
		Id(gpuPanelPower).
		Title("Power").
		Description("Average package power. The exporter publishes no instantaneous figure, so this is the only power reading available.").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("watt").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			// Only amd_gpu_average_package_power exists; label it as average rather
			// than implying a missing instantaneous series.
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_average_package_power{`+gpuFilter+`}`)).
				LegendFormat("Average package power")))))

	// Metrics are in MHz; multiply to Hz for Grafana unit auto-scaling.
	d.Panel(elementClockSpeed, dashboardv2.NewPanelBuilder().
		Id(gpuPanelClockSpeed).
		Title("Clock Speed").
		Visualization(timeseries.NewVisualizationV2Builder().
			Unit("hertz").
			Tooltip(tooltipAll).
			Legend(legend)).
		Data(dashboardv2.NewQueryGroupBuilder().
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_clock{`+gpuFilter+`, clock_type="system"}`)+` * 1000 * 1000`).
				LegendFormat("GPU Core"))).
			Target(dashboardv2.NewTargetBuilder().Query(promQuery().
				Expr(byGpu(`amd_gpu_clock{`+gpuFilter+`, clock_type="memory"}`)+` * 1000 * 1000`).
				LegendFormat("Memory")))))

	built, err := d.RowsLayout(dashboardv2.NewRowsBuilder().
		Row(dashboardv2.NewRowBuilder().Title("Summary").AutoGridLayout(statGrid(4).
			WithItem(elementGFXActivity).
			WithItem(elementVRAMUsage).
			WithItem(elementTemperatureEdge).
			WithItem(elementPowerAvg))).
		Row(dashboardv2.NewRowBuilder().Title("GPU Health").AutoGridLayout(statGrid(3).
			WithItem(elementGPUHealthIssues).
			WithItem(elementECCUncorrectableErrors).
			WithItem(elementTemperatureJunction))).
		Row(dashboardv2.NewRowBuilder().Title("GPU Activity (%)").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementGPUActivity))).
		Row(dashboardv2.NewRowBuilder().Title("Memory").AutoGridLayout(chartGrid().
			WithItem(elementVRAM).
			WithItem(elementGTTMemory))).
		Row(dashboardv2.NewRowBuilder().Title("Temperature").AutoGridLayout(wideGrid(dashboardv2.AutoGridLayoutSpecRowHeightModeStandard).
			WithItem(elementTemperature))).
		Row(dashboardv2.NewRowBuilder().Title("Power & Clocks").AutoGridLayout(chartGrid().
			WithItem(elementPower).
			WithItem(elementClockSpeed)))).Build()
	if err != nil {
		return nil, err
	}
	return &built, nil
}
