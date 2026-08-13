package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

func main() {
	// Dashboard names become output filenames.
	dashboards := map[string]func() (*dashboard.Dashboard, error){
		"node-overview":          buildNodeOverview,
		"k8s-node-overview":      buildK8sNodeOverview,
		"proxmox-otlp-overview":  buildProxmoxOtlpOverview,
		"gpu-overview":           buildGpuOverview,
		"disk-health":            buildDiskHealth,
		"dns-overview":           buildDnsOverview,
		"network-overview":       buildNetworkOverview,
		"uptime":                 buildUptime,
		"kubernetes-overview":    buildKubernetesOverview,
		"k8s-control-plane":      buildK8sControlPlaneOverview,
		"monitoring-overview":    buildMonitoringOverview,
		"dns-logs":               buildDnsLogs,
		"syslog":                 buildSyslog,
		"proxmox-logs":           buildProxmoxLogs,
		"service-logs":           buildServiceLogs,
		"cert-manager-overview":  buildCertManagerOverview,
		"cilium-overview":        buildCiliumOverview,
		"envoy-gateway-overview": buildEnvoyGatewayOverview,
		"argocd-overview":        buildArgocdOverview,
		"openbao-overview":       buildOpenbaoOverview,
	}

	outputDir := "generated"
	if len(os.Args) > 1 {
		outputDir = os.Args[1]
	}

	generated := make(map[string][]byte, len(dashboards))
	for name, build := range dashboards {
		d, err := build()
		if err != nil {
			log.Fatalf("failed to build dashboard %s: %v", name, err)
		}

		out, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshal dashboard %s: %v", name, err)
		}
		if err := validateDashboardJSON(name, out); err != nil {
			log.Fatalf("invalid dashboard: %v", err)
		}
		generated[name] = out
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("failed to create output dir: %v", err)
	}

	names := make([]string, 0, len(generated))
	for name := range generated {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(outputDir, name+".json")
		if err := os.WriteFile(path, generated[name], 0o644); err != nil {
			log.Fatalf("failed to write %s: %v", path, err)
		}
		fmt.Printf("generated: %s\n", path)
	}
}
