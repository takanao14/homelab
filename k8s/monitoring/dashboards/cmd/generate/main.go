package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

func main() {
	// Map of dashboard name to builder function.
	// The name becomes the output filename (e.g. "node-overview" in the output directory).
	// To add a new dashboard, just add an entry here.
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
