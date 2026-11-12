package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// proxmoxNodesFile is the shared inventory relative to this working directory.
const proxmoxNodesFile = "../values/proxmox-nodes.yaml"

// loadProxmoxHostRegex keeps dashboards aligned with shared scrape inventory.
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
