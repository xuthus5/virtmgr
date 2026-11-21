package libvirtbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
)

const hostTopologyRoot = "/sys"

func readHostTopology() ([]domain.HostNUMANode, []domain.HostHugePage) {
	return readHostTopologyFrom(hostTopologyRoot)
}

func readHostTopologyFrom(root string) ([]domain.HostNUMANode, []domain.HostHugePage) {
	nodes := readNUMANodes(filepath.Join(root, "devices", "system"))
	hugePages := readHugePages(filepath.Join(root, "kernel", "mm", "hugepages"))
	return nodes, hugePages
}

func readNUMANodes(root string) []domain.HostNUMANode {
	rawOnline, err := os.ReadFile(filepath.Join(root, "node", "online"))
	if err != nil {
		return nil
	}
	nodeIDs, ok := parseCPUList(strings.TrimSpace(string(rawOnline)))
	if !ok {
		return nil
	}
	result := make([]domain.HostNUMANode, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		nodeRoot := filepath.Join(root, "node", fmt.Sprintf("node%d", nodeID))
		cpus := []uint{}
		if raw, readErr := os.ReadFile(filepath.Join(nodeRoot, "cpulist")); readErr == nil {
			cpus, _ = parseCPUList(strings.TrimSpace(string(raw)))
		}
		if len(cpus) == 0 {
			continue
		}
		total, free := readNodeMemory(filepath.Join(nodeRoot, "meminfo"), int(nodeID))
		result = append(result, domain.HostNUMANode{ID: int(nodeID), CPUs: cpus, MemoryBytes: total, FreeMemoryBytes: free})
	}
	slices.SortFunc(result, func(left, right domain.HostNUMANode) int { return left.ID - right.ID })
	return result
}

func readNodeMemory(path string, nodeID int) (uint64, uint64) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	prefix := fmt.Sprintf("Node %d ", nodeID)
	var total, free uint64
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, prefix))
		if len(fields) < 3 {
			continue
		}
		value, parseErr := strconv.ParseUint(fields[len(fields)-2], 10, 64)
		if parseErr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemFree:":
			free = value * 1024
		}
	}
	return total, free
}

func readHugePages(path string) []domain.HostHugePage {
	entries, err := filepath.Glob(filepath.Join(path, "hugepages-*kB"))
	if err != nil {
		return nil
	}
	result := make([]domain.HostHugePage, 0, len(entries))
	for _, entry := range entries {
		base := filepath.Base(entry)
		sizeKiB, parseErr := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(base, "hugepages-"), "kB"), 10, 64)
		if parseErr != nil || sizeKiB == 0 {
			continue
		}
		item := domain.HostHugePage{
			SizeBytes: sizeKiB * 1024,
			Total:     readSysfsInt(filepath.Join(entry, "nr_hugepages")),
			Free:      readSysfsInt(filepath.Join(entry, "free_hugepages")),
			Reserved:  readSysfsInt(filepath.Join(entry, "resv_hugepages")),
			Surplus:   readSysfsInt(filepath.Join(entry, "surplus_hugepages")),
		}
		result = append(result, item)
	}
	slices.SortFunc(result, func(left, right domain.HostHugePage) int {
		return int(left.SizeBytes) - int(right.SizeBytes)
	})
	return result
}

func readSysfsInt(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return value
}

// parseCPUList expands Linux CPU lists such as "0-3,8,10-11" into sorted IDs.
func parseCPUList(value string) ([]uint, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	seen := map[uint]struct{}{}
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		startText, endText, ranged := strings.Cut(part, "-")
		start, err := strconv.ParseUint(startText, 10, 32)
		if err != nil {
			return nil, false
		}
		end := start
		if ranged {
			end, err = strconv.ParseUint(endText, 10, 32)
			if err != nil || end < start {
				return nil, false
			}
		}
		for cpu := start; cpu <= end; cpu++ {
			seen[uint(cpu)] = struct{}{}
		}
	}
	result := make([]uint, 0, len(seen))
	for cpu := range seen {
		result = append(result, cpu)
	}
	slices.Sort(result)
	return result, true
}
