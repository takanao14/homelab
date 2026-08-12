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
//
// Every query goes through byGpu, which collapses the labels the exporter varies
// underneath a metric that is supposed to describe one card. Two of them move:
//
// kfd_process_id appears when a compute process is running, and the exporter
// swaps label sets rather than adding to them -- while a process holds the GPU
// the aggregate series stops and a per-process one takes over. Measured across
// the switch on 2026-07-24: the series without kfd_process_id reported through
// 08:46 and stopped, and one carrying kfd_process_id="11558" started at 08:47,
// the two never overlapping. Left alone every panel breaks its line the moment
// the GPU starts working, and since LegendFormat here is a fixed string both
// halves are labelled identically, so the break reads as missing data rather
// than as a new series. It also means the value on screen during a workload --
// the only time this dashboard is worth opening -- describes one process rather
// than the card.
//
// driver_version and vbios_version move on every driver upgrade: four distinct
// driver_version values in 60 days, two of them the exporter putting a whole
// uname string where a version belongs ("Linuxversion7.0.0-28-generic(buildd@...").
// Same consequence, on a slower clock.
//
// gpu_id is the one label that identifies the thing being measured, so grouping
// by it keeps a per-card series if a second card is ever fitted. max is the
// reduction rather than sum because these are gauges describing one device, not
// quantities to add up; it also keeps count() and sum() in the health row
// counting cards instead of however many series the exporter happens to emit.
func buildGpuOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	const gpuFilter = `job="scrapeConfig/monitoring/amd-gpu-external"`

	byGpu := func(expr string) string {
		return `max by (gpu_id) (` + expr + `)`
	}

	tooltipAll := defaultTooltip()
	legend := defaultLegend()

	gfxThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(80)), Color: "yellow"},
			{Value: new(float64(95)), Color: "red"},
		})

	vramThresholds := capacityThresholds()

	powerThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(120)), Color: "yellow"},
			{Value: new(float64(150)), Color: "red"},
		})

	// Edge temperature thresholds (°C): normal < 85, warm < 100, hot >= 100.
	tempThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(85)), Color: "yellow"},
			{Value: new(float64(100)), Color: "red"},
		})

	// Junction (hotspot) temperature is expected to run hotter than edge temperature.
	junctionTempThresholds := dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps([]dashboard.Threshold{
			{Value: nil, Color: "green"},
			{Value: new(float64(95)), Color: "yellow"},
			{Value: new(float64(105)), Color: "red"},
		})

	issueThresholds := issueThresholds()

	d, err := dashboard.NewDashboardBuilder("GPU Overview").
		Uid("gpu-overview").
		Tags([]string{"gpu", "infrastructure"}).
		Timezone("browser").
		Time("now-1d", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			promDatasourceVariable(),
		).
		WithRow(dashboard.NewRowBuilder("Summary")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("GFX Activity").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Min(0).
				Thresholds(gfxThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_gfx_activity{` + gpuFilter + `}`)).
					LegendFormat("GFX Activity"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("VRAM Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				Min(0).
				Thresholds(vramThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_used_vram{`+gpuFilter+`}`) + ` / ` + byGpu(`amd_gpu_total_vram{`+gpuFilter+`}`) + ` * 100`).
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
					Expr(byGpu(`amd_gpu_edge_temperature{` + gpuFilter + `}`)).
					LegendFormat("Edge Temp"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Power (Avg)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("watt").
				Min(0).
				Thresholds(powerThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_average_package_power{` + gpuFilter + `}`)).
					LegendFormat("Power"),
				),
		).
		WithRow(dashboard.NewRowBuilder("GPU Health")).
		WithPanel(
			stat.NewPanelBuilder().
				Title("GPU Health Issues").
				Description("Number of GPUs whose exporter health value is not healthy (1).").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`count(` + byGpu(`amd_gpu_health{`+gpuFilter+`}`) + ` != 1) or vector(0)`).
					Instant().
					LegendFormat("Health Issues"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("ECC Uncorrectable Errors").
				Description("Total uncorrectable GPU ECC errors reported by the exporter.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("short").
				Min(0).
				Thresholds(issueThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(`sum(` + byGpu(`amd_gpu_ecc_uncorrect_total{`+gpuFilter+`}`) + `) or vector(0)`).
					Instant().
					LegendFormat("ECC Errors"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Temperature (Junction)").
				Description("Current GPU hotspot temperature.").
				Datasource(ds).
				Span(8).Height(4).
				Unit("celsius").
				Thresholds(junctionTempThresholds).
				ColorMode(common.BigValueColorModeBackground).
				Orientation(common.VizOrientationAuto).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_junction_temperature{` + gpuFilter + `}`)).
					Instant().
					LegendFormat("Junction Temp"),
				),
		).
		WithRow(dashboard.NewRowBuilder("Utilization & Resources")).
		// gfx=graphics/compute, umc=memory controller, vcn=video codec engine
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("GPU Activity (%)").
				Datasource(ds).
				Span(24).Height(8).
				Unit("percent").
				// Pinned to 0-100 like the other bounded percentages in this repo
				// (node-overview CPU Usage, k8s-node-overview CPU Usage). Without it
				// an idle GPU, which is the usual state here, autoscales a flat zero
				// line onto whatever range Grafana invents and the panel reads as
				// though something were happening.
				Min(0).
				Max(100).
				Tooltip(tooltipAll).
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_gfx_activity{` + gpuFilter + `}`)).
					LegendFormat("GFX"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_umc_activity{` + gpuFilter + `}`)).
					LegendFormat("Memory Controller"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_vcn_activity{` + gpuFilter + `}`)).
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
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_used_vram{`+gpuFilter+`}`) + ` * 1024 * 1024`).
					LegendFormat("Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_total_vram{`+gpuFilter+`}`) + ` * 1024 * 1024`).
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
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_used_gtt{`+gpuFilter+`}`) + ` * 1024 * 1024`).
					LegendFormat("Used"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_total_gtt{`+gpuFilter+`}`) + ` * 1024 * 1024`).
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
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_edge_temperature{` + gpuFilter + `}`)).
					LegendFormat("Edge"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_junction_temperature{` + gpuFilter + `}`)).
					LegendFormat("Junction (Hotspot)"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_memory_temperature{` + gpuFilter + `}`)).
					LegendFormat("Memory"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Power").
				Description("Average package power. The exporter publishes no instantaneous figure, so this is the only power reading available.").
				Datasource(ds).
				Span(12).Height(8).
				Unit("watt").
				Tooltip(tooltipAll).
				Legend(legend).
				// A second target read amd_gpu_power_usage and was labelled
				// "Current". That metric has never been exported: nothing matched it
				// anywhere in the retention window, so the panel drew one line while
				// claiming to compare two, and the average was silently read as an
				// instantaneous value. Only amd_gpu_average_package_power exists,
				// and the legend now says so.
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_average_package_power{` + gpuFilter + `}`)).
					LegendFormat("Average package power"),
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
				Legend(legend).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_clock{`+gpuFilter+`, clock_type="system"}`) + ` * 1000 * 1000`).
					LegendFormat("GPU Core"),
				).
				WithTarget(prometheus.NewDataqueryBuilder().
					Expr(byGpu(`amd_gpu_clock{`+gpuFilter+`, clock_type="memory"}`) + ` * 1000 * 1000`).
					LegendFormat("Memory"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}
