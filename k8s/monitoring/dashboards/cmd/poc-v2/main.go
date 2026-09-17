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

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: poc-v2 OUTPUT_DIRECTORY")
	}
	if err := os.MkdirAll(os.Args[1], 0o755); err != nil {
		log.Fatal(err)
	}
	dashboards := []struct {
		name  string
		build func() (*dashboardv2.Dashboard, error)
	}{
		{name: "uptime-v2-poc", build: buildUptime},
		{name: "dhcp-leases-v2-poc", build: buildDhcpLeases},
		{name: "cert-manager-v2-poc", build: buildCertManager},
	}
	for _, dashboard := range dashboards {
		d, err := dashboard.build()
		if err != nil {
			log.Fatal(err)
		}
		if err := validateDashboard(d); err != nil {
			log.Fatal(err)
		}
		manifest, err := resource.NewManifestBuilder().ApiVersion("dashboard.grafana.app/v2").Kind("Dashboard").Metadata(resource.Named(dashboard.name)).Spec(d).Build()
		if err != nil {
			log.Fatal(err)
		}
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		path := filepath.Join(os.Args[1], dashboard.name+".json")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println(path)
	}
}
