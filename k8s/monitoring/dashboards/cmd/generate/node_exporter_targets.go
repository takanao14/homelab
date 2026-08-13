package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// nodeExporterTargetsFile contains non-hypervisor node-exporter inventory.
// Hypervisors come from proxmoxNodesFile.
const nodeExporterTargetsFile = "../values/node-exporter-external.yaml"

// loadLxcGuestRegex identifies LXC guests for kernel-wide metric exclusions.
// lxcfs virtualizes CPU, memory, load, and related PSI, but IO pressure and
// boot time still mirror the host. Excluding known guests avoids duplicate
// host series while leaving VMs and bare metal intact.
func loadLxcGuestRegex() (string, error) {
	raw, err := os.ReadFile(nodeExporterTargetsFile)
	if err != nil {
		return "", fmt.Errorf("read node-exporter inventory: %w", err)
	}
	var inventory struct {
		Targets []struct {
			Name string `yaml:"name"`
			Kind string `yaml:"kind"`
		} `yaml:"targets"`
	}
	if err := yaml.Unmarshal(raw, &inventory); err != nil {
		return "", fmt.Errorf("parse node-exporter inventory: %w", err)
	}
	if len(inventory.Targets) == 0 {
		return "", fmt.Errorf("no targets entries in %s", nodeExporterTargetsFile)
	}
	names := make([]string, 0, len(inventory.Targets))
	for _, target := range inventory.Targets {
		// Require kind so new VMs remain visible and new LXC guests cannot silently
		// republish host metrics.
		switch target.Kind {
		case "lxc":
			if target.Name == "" {
				return "", fmt.Errorf("targets entry without name in %s", nodeExporterTargetsFile)
			}
			names = append(names, target.Name)
		case "vm", "metal":
		default:
			return "", fmt.Errorf("targets entry %q in %s has kind %q, want lxc|vm|metal",
				target.Name, nodeExporterTargetsFile, target.Kind)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no lxc targets in %s", nodeExporterTargetsFile)
	}
	return strings.Join(names, "|"), nil
}
