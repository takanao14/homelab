package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// proxmoxNodesFile is the shared Proxmox hypervisor inventory, resolved
// relative to the dashboards/ working directory the Makefile runs from.
const proxmoxNodesFile = "../values/proxmox-nodes.yaml"

// loadProxmoxHostRegex builds a "pve|node1|..." host regex from the shared
// inventory so dashboards stay in sync with the scrape and alerting
// configuration derived from the same file.
func loadProxmoxHostRegex() (string, error) {
	raw, err := os.ReadFile(proxmoxNodesFile)
	if err != nil {
		return "", fmt.Errorf("read proxmox inventory: %w", err)
	}
	var inventory struct {
		ProxmoxNodes []struct {
			Name string `yaml:"name"`
		} `yaml:"proxmoxNodes"`
	}
	if err := yaml.Unmarshal(raw, &inventory); err != nil {
		return "", fmt.Errorf("parse proxmox inventory: %w", err)
	}
	if len(inventory.ProxmoxNodes) == 0 {
		return "", fmt.Errorf("no proxmoxNodes entries in %s", proxmoxNodesFile)
	}
	names := make([]string, 0, len(inventory.ProxmoxNodes))
	for _, node := range inventory.ProxmoxNodes {
		if node.Name == "" {
			return "", fmt.Errorf("proxmoxNodes entry without name in %s", proxmoxNodesFile)
		}
		names = append(names, node.Name)
	}
	return strings.Join(names, "|"), nil
}
