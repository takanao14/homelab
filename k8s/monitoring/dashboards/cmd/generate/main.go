package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/resource"
)

type dashboardDefinition struct {
	name  string
	uid   string
	build func() (*dashboardv2.Dashboard, error)
}

var dashboards = []dashboardDefinition{
	{name: "argocd-overview", uid: "argocd-overview", build: buildArgocdOverview},
	{name: "cert-manager-overview", uid: "cert-manager-overview", build: buildCertManagerOverview},
	{name: "cilium-overview", uid: "cilium-overview", build: buildCiliumOverview},
	{name: "dhcp-leases", uid: "dhcp-leases", build: buildDhcpLeases},
	{name: "disk-health", uid: "disk-health", build: buildDiskHealth},
	{name: "dns-logs", uid: "dns-logs", build: buildDnsLogs},
	{name: "dns-overview", uid: "dns-overview", build: buildDnsOverview},
	{name: "envoy-gateway-overview", uid: "envoy-gateway-overview", build: buildEnvoyGatewayOverview},
	{name: "gpu-overview", uid: "gpu-overview", build: buildGpuOverview},
	{name: "k8s-control-plane", uid: "k8s-control-plane-overview", build: buildK8sControlPlaneOverview},
	{name: "k8s-node-overview", uid: "k8s-node-overview", build: buildK8sNodeOverview},
	{name: "kubernetes-overview", uid: "kubernetes-overview", build: buildKubernetesOverview},
	{name: "monitoring-overview", uid: "monitoring-overview", build: buildMonitoringOverview},
	{name: "network-overview", uid: "network-overview", build: buildNetworkOverview},
	{name: "node-overview", uid: "node-overview", build: buildNodeOverview},
	{name: "openbao-overview", uid: "openbao-overview", build: buildOpenbaoOverview},
	{name: "proxmox-logs", uid: "proxmox-logs", build: buildProxmoxLogs},
	{name: "proxmox-otlp-overview", uid: "proxmox-otlp-overview", build: buildProxmoxOtlpOverview},
	{name: "service-logs", uid: "service-logs", build: buildServiceLogs},
	{name: "slack-bot-overview", uid: "slack-bot-overview", build: buildSlackBotOverview},
	{name: "syslog", uid: "syslog", build: buildSyslog},
	{name: "uptime", uid: "uptime", build: buildUptime},
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: generate OUTPUT_DIRECTORY")
	}
	outputDir := os.Args[1]
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatal(err)
	}

	for _, definition := range dashboards {
		d, err := definition.build()
		if err != nil {
			log.Fatalf("%s: %v", definition.name, err)
		}
		if err := validateDashboard(d); err != nil {
			log.Fatalf("%s: %v", definition.name, err)
		}

		manifest, err := resource.NewManifestBuilder().ApiVersion("dashboard.grafana.app/v2").Kind("Dashboard").Metadata(resource.Named(definition.uid)).Spec(d).Build()
		if err != nil {
			log.Fatal(err)
		}
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(outputDir, definition.name+".json")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(path)
	}
}
