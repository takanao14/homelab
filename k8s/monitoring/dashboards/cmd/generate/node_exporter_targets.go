package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// nodeExporterTargetsFile contains non-hypervisor node-exporter inventory.
// Hypervisors come from proxmoxNodesFile.
const nodeExporterTargetsFile = "../values/node-exporter-external.yaml"

type nodeExporterTarget struct {
	Name        string `yaml:"name"`
	Kind        string `yaml:"kind"`
	LogShipping bool   `yaml:"logShipping"`
}

func loadNodeExporterTargets() ([]nodeExporterTarget, error) {
	raw, err := os.ReadFile(nodeExporterTargetsFile)
	if err != nil {
		return nil, fmt.Errorf("read node-exporter inventory: %w", err)
	}
	var inventory struct {
		Targets []nodeExporterTarget `yaml:"targets"`
	}
	if err := yaml.Unmarshal(raw, &inventory); err != nil {
		return nil, fmt.Errorf("parse node-exporter inventory: %w", err)
	}
	if len(inventory.Targets) == 0 {
		return nil, fmt.Errorf("no targets entries in %s", nodeExporterTargetsFile)
	}
	return inventory.Targets, nil
}

// loadLxcGuestRegex identifies LXC guests for kernel-wide metric exclusions.
// lxcfs virtualizes CPU, memory, load, and related PSI, but IO pressure and
// boot time still mirror the host. Excluding known guests avoids duplicate
// host series while leaving VMs and bare metal intact.
func loadLxcGuestRegex() (string, error) {
	targets, err := loadNodeExporterTargets()
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(targets))
	for _, target := range targets {
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

func loadLogShippingVMs() ([]string, error) {
	targets, err := loadNodeExporterTargets()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if !target.LogShipping {
			continue
		}
		if target.Name == "" {
			return nil, fmt.Errorf("log-shipping target without name in %s", nodeExporterTargetsFile)
		}
		if target.Kind != "vm" {
			return nil, fmt.Errorf("log-shipping target %q in %s has kind %q, want vm",
				target.Name, nodeExporterTargetsFile, target.Kind)
		}
		if _, exists := seen[target.Name]; exists {
			return nil, fmt.Errorf("duplicate log-shipping target %q in %s", target.Name, nodeExporterTargetsFile)
		}
		seen[target.Name] = struct{}{}
		names = append(names, target.Name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no log-shipping VM targets in %s", nodeExporterTargetsFile)
	}
	sort.Strings(names)
	return names, nil
}
