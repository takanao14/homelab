package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// nodeExporterTargetsFile is the non-hypervisor half of the node-exporter
// inventory, resolved relative to the dashboards/ working directory the Makefile
// runs from. The hypervisors themselves come from proxmoxNodesFile.
const nodeExporterTargetsFile = "../values/node-exporter-external.yaml"

// loadLxcGuestRegex builds a "resolver1|ns1|..." regex of the LXC guests, so
// panels reading kernel-wide values can exclude them.
//
// An LXC guest runs on its host's kernel, so anything node-exporter reads
// straight out of /proc is the host's number wearing the guest's instance label.
// lxcfs virtualises much of it -- CPU count, meminfo, loadavg and the CPU and
// memory PSI files are all genuinely per-container -- but not all of it.
// Measured across the fleet, node_pressure_io_{waiting,stalled}_seconds_total
// and node_boot_time_seconds are bit-for-bit the host's: five guests on node2
// all read its 9221.5s of IO wait and its exact boot timestamp, and three on
// node3 read node3's.
//
// This is the third time the same shape has turned up, after ZFS ARC and CPU
// package throttles, which is why the answer lives in the inventory now rather
// than in another hand-written list. VMs and bare metal each run their own
// kernel and are correct as they stand.
//
// The result is meant for `instance!~"..."`. Exclusion rather than inclusion is
// deliberate: the hypervisors arrive from a different file, and an exclusion
// list leaves them alone instead of needing both files stitched together.
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
		// kind is required rather than defaulted. Defaulting either way has a
		// silent failure: assume "vm" and a new LXC guest quietly republishes its
		// host's numbers, assume "lxc" and a new VM quietly vanishes from the
		// panels. Failing generation is the only option that cannot be missed.
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
