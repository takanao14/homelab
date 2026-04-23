package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

func main() {
	// Map of dashboard name to builder function.
	// The name becomes the output filename (e.g. "node-overview" → generated/node-overview.json).
	// To add a new dashboard, just add an entry here.
	dashboards := map[string]func() (*dashboard.Dashboard, error){
		"node-overview":       buildNodeOverview,
		"proxmox-overview":    buildProxmoxOverview,
		"gpu-overview":        buildGpuOverview,
		"dns-overview":        buildDnsOverview,
		"network-overview":    buildNetworkOverview,
		"uptime":              buildUptime,
		"kubernetes-overview": buildKubernetesOverview,
		"monitoring-overview": buildMonitoringOverview,
		"dns-logs":            buildDnsLogs,
		"syslog":              buildSyslog,
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

// promDatasource returns a datasource ref using "$datasource" as the UID so that
// all panels switch together when the user changes the datasource dropdown variable.
func promDatasource() common.DataSourceRef {
	dsType := "prometheus"
	dsUID := "$datasource"
	return common.DataSourceRef{
		Type: &dsType,
		Uid:  &dsUID,
	}
}

func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}
