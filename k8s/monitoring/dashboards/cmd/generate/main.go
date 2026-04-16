package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

func main() {
	dashboards := map[string]func() (*dashboard.Dashboard, error){
		"node-overview": buildNodeOverview,
	}

	outputDir := "generated"
	if len(os.Args) > 1 {
		outputDir = os.Args[1]
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("failed to create output dir: %v", err)
	}

	for name, build := range dashboards {
		d, err := build()
		if err != nil {
			log.Fatalf("failed to build dashboard %s: %v", name, err)
		}

		out, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshal dashboard %s: %v", name, err)
		}

		path := filepath.Join(outputDir, name+".json")
		if err := os.WriteFile(path, out, 0o644); err != nil {
			log.Fatalf("failed to write %s: %v", path, err)
		}
		fmt.Printf("generated: %s\n", path)
	}
}

func promDatasource() common.DataSourceRef {
	dsType := "prometheus"
	dsUID := "$datasource"
	return common.DataSourceRef{
		Type: &dsType,
		Uid:  &dsUID,
	}
}

func buildNodeOverview() (*dashboard.Dashboard, error) {
	ds := promDatasource()

	d, err := dashboard.NewDashboardBuilder("Node Overview").
		Uid("node-overview").
		Tags([]string{"nodes", "infrastructure"}).
		Timezone("browser").
		Time("now-1h", "now").
		Refresh("30s").
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		WithVariable(
			dashboard.NewDatasourceVariableBuilder("datasource").
				Label("Datasource").
				Type("prometheus"),
		).
		WithVariable(
			dashboard.NewQueryVariableBuilder("node").
				Label("Node").
				Datasource(ds).
				Query(dashboard.StringOrMap{String: strPtr(`label_values(node_uname_info, nodename)`)}).
				Refresh(dashboard.VariableRefreshOnTimeRangeChanged).
				Sort(dashboard.VariableSortAlphabeticalAsc).
				Multi(false).
				IncludeAll(false),
		).
		// Row 1: Overview stats
		WithPanel(
			stat.NewPanelBuilder().
				Title("CPU Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`100 - (avg by (nodename) (rate(node_cpu_seconds_total{mode="idle", nodename="$node"}[5m])) * 100)`).
						LegendFormat("CPU Usage"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Memory Usage").
				Datasource(ds).
				Span(6).Height(4).
				Unit("percent").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`100 - (node_memory_MemAvailable_bytes{nodename="$node"} / node_memory_MemTotal_bytes{nodename="$node"} * 100)`).
						LegendFormat("Memory Usage"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Load Average (1m)").
				Datasource(ds).
				Span(6).Height(4).
				Unit("short").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`node_load1{nodename="$node"}`).
						LegendFormat("Load 1m"),
				),
		).
		WithPanel(
			stat.NewPanelBuilder().
				Title("Uptime").
				Datasource(ds).
				Span(6).Height(4).
				Unit("s").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`node_time_seconds{nodename="$node"} - node_boot_time_seconds{nodename="$node"}`).
						LegendFormat("Uptime"),
				),
		).
		// Row 2: CPU / Memory timeseries
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("CPU Usage (%)").
				Datasource(ds).
				Span(12).Height(8).
				Unit("percent").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`100 - (avg by (cpu) (rate(node_cpu_seconds_total{mode="idle", nodename="$node"}[5m])) * 100)`).
						LegendFormat("CPU {{cpu}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Memory").
				Datasource(ds).
				Span(12).Height(8).
				Unit("bytes").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`node_memory_MemTotal_bytes{nodename="$node"}`).
						LegendFormat("Total"),
				).
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`node_memory_MemAvailable_bytes{nodename="$node"}`).
						LegendFormat("Available"),
				).
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`node_memory_MemTotal_bytes{nodename="$node"} - node_memory_MemAvailable_bytes{nodename="$node"}`).
						LegendFormat("Used"),
				),
		).
		// Row 3: Network / Disk
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Network I/O").
				Datasource(ds).
				Span(12).Height(8).
				Unit("Bps").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`rate(node_network_receive_bytes_total{nodename="$node", device!="lo"}[5m])`).
						LegendFormat("Rx {{device}}"),
				).
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`rate(node_network_transmit_bytes_total{nodename="$node", device!="lo"}[5m])`).
						LegendFormat("Tx {{device}}"),
				),
		).
		WithPanel(
			timeseries.NewPanelBuilder().
				Title("Disk I/O").
				Datasource(ds).
				Span(12).Height(8).
				Unit("Bps").
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`rate(node_disk_read_bytes_total{nodename="$node"}[5m])`).
						LegendFormat("Read {{device}}"),
				).
				WithTarget(
					prometheus.NewDataqueryBuilder().
						Expr(`rate(node_disk_written_bytes_total{nodename="$node"}[5m])`).
						LegendFormat("Write {{device}}"),
				),
		).
		Build()

	if err != nil {
		return nil, err
	}
	return &d, nil
}

func strPtr(s string) *string {
	return &s
}
