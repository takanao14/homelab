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

// translation is a dashboardv2 rewrite of a cmd/generate builder. Until it is
// cut over, its Classic version stays authoritative and the translation is
// written only to the local directory, under a separate UID, for comparison.
type translation struct {
	name    string // output file name, shared with the Classic dashboard
	uid     string // Classic UID, kept at cutover
	build   func() (*dashboardv2.Dashboard, error)
	cutover bool
}

var translations = []translation{
	{name: "monitoring-overview", uid: "monitoring-overview", build: buildMonitoringOverview, cutover: true},
	{name: "node-overview", uid: "node-overview", build: buildNodeOverview, cutover: true},
	{name: "disk-health", uid: "disk-health", build: buildDiskHealth, cutover: true},
	{name: "k8s-node-overview", uid: "k8s-node-overview", build: buildK8sNodeOverview, cutover: true},
	{name: "argocd-overview", uid: "argocd-overview", build: buildArgocdOverview, cutover: true},
	{name: "cilium-overview", uid: "cilium-overview", build: buildCiliumOverview, cutover: true},
	{name: "cert-manager-overview", uid: "cert-manager-overview", build: buildCertManagerOverview, cutover: true},
	{name: "network-overview", uid: "network-overview", build: buildNetworkOverview, cutover: true},
	{name: "proxmox-otlp-overview", uid: "proxmox-otlp-overview", build: buildProxmoxOtlpOverview, cutover: true},
	{name: "envoy-gateway-overview", uid: "envoy-gateway-overview", build: buildEnvoyGatewayOverview, cutover: true},
	{name: "slack-bot-overview", uid: "slack-bot-overview", build: buildSlackBotOverview, cutover: true},
	{name: "openbao-overview", uid: "openbao-overview", build: buildOpenbaoOverview, cutover: true},
	{name: "gpu-overview", uid: "gpu-overview", build: buildGpuOverview, cutover: true},
	{name: "uptime", uid: "uptime", build: buildUptime, cutover: true},
}

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: poc-v2 CHART_DIRECTORY LOCAL_DIRECTORY")
	}
	chartDir, localDir := os.Args[1], os.Args[2]
	for _, dir := range []string{chartDir, localDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
	}
	// Drop translations that have since been cut over or removed.
	stale, err := filepath.Glob(filepath.Join(localDir, "*.json"))
	if err != nil {
		log.Fatal(err)
	}
	for _, path := range stale {
		if err := os.Remove(path); err != nil {
			log.Fatal(err)
		}
	}

	for _, t := range translations {
		d, err := t.build()
		if err != nil {
			log.Fatalf("%s: %v", t.name, err)
		}
		if err := validateDashboard(d); err != nil {
			log.Fatalf("%s: %v", t.name, err)
		}
		dir, uid := localDir, t.uid+"-v2"
		if t.cutover {
			dir, uid = chartDir, t.uid
		}
		manifest, err := resource.NewManifestBuilder().ApiVersion("dashboard.grafana.app/v2").Kind("Dashboard").Metadata(resource.Named(uid)).Spec(d).Build()
		if err != nil {
			log.Fatal(err)
		}
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(dir, t.name+".json")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(path)
	}
}
