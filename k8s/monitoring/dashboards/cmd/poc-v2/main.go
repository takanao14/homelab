package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/grafana/grafana-foundation-sdk/go/resource"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: poc-v2 OUTPUT_DIRECTORY")
	}
	d, err := buildUptime()
	if err != nil {
		log.Fatal(err)
	}
	if err := validateDashboard(d); err != nil {
		log.Fatal(err)
	}
	manifest, err := resource.NewManifestBuilder().ApiVersion("dashboard.grafana.app/v2").Kind("Dashboard").Metadata(resource.Named("uptime-v2-poc")).Spec(d).Build()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(os.Args[1], 0o755); err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(os.Args[1], "uptime-v2-poc.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(path)
}
