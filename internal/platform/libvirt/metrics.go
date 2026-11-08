package libvirtbackend

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) HostMetrics(ctx context.Context) (domain.MetricsCounters, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.MetricsCounters{}, err
	}
	info, err := c.conn.GetNodeInfo()
	if err != nil {
		return domain.MetricsCounters{}, fmt.Errorf("read libvirt node info for metrics: %w", err)
	}
	cpu, err := c.conn.GetCPUStats(int(libvirt.NODE_CPU_STATS_ALL_CPUS), 0)
	if err != nil {
		return domain.MetricsCounters{}, fmt.Errorf("read libvirt host cpu metrics: %w", err)
	}
	memory, err := c.conn.GetMemoryStats(libvirt.NODE_MEMORY_STATS_ALL_CELLS, 0)
	if err != nil {
		return domain.MetricsCounters{}, fmt.Errorf("read libvirt host memory metrics: %w", err)
	}
	memoryUsed := hostMemoryUsed(memory)
	rxBytes, txBytes := readHostNetworkCounters()
	return domain.MetricsCounters{
		CPUTimeNanos:     hostCPUTimeNanos(cpu),
		CPUCapacity:      uint64(info.Cpus),
		MemoryUsedBytes:  memoryUsed,
		MemoryTotalBytes: memory.Total,
		NetworkRxBytes:   rxBytes,
		NetworkTxBytes:   txBytes,
	}, nil
}

func (c *Client) VMMetrics(ctx context.Context, uuid string) (domain.MetricsCounters, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.MetricsCounters{}, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.MetricsCounters{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	info, err := dom.GetInfo()
	if err != nil {
		return domain.MetricsCounters{}, fmt.Errorf("read domain %s metrics: %w", uuid, err)
	}
	memoryTotal := info.Memory * 1024
	memoryUsed := memoryTotal
	if stats, statsErr := dom.MemoryStats(uint32(libvirt.DOMAIN_MEMORY_STAT_NR), 0); statsErr == nil {
		if rss := domainMemoryValue(stats, libvirt.DOMAIN_MEMORY_STAT_RSS); rss > 0 {
			memoryUsed = min(rss, memoryTotal)
		}
	}
	rxBytes, txBytes := domainNetworkCounters(dom)
	return domain.MetricsCounters{
		CPUTimeNanos:     info.CpuTime,
		CPUCapacity:      uint64(info.NrVirtCpu),
		MemoryUsedBytes:  memoryUsed,
		MemoryTotalBytes: memoryTotal,
		NetworkRxBytes:   rxBytes,
		NetworkTxBytes:   txBytes,
	}, nil
}

func hostCPUTimeNanos(stats *libvirt.NodeCPUStats) uint64 {
	if stats == nil {
		return 0
	}
	return stats.User + stats.Kernel + stats.Iowait
}

func hostMemoryUsed(stats *libvirt.NodeMemoryStats) uint64 {
	if stats == nil {
		return 0
	}
	if stats.AvailableSet && stats.Total >= stats.Available {
		return stats.Total - stats.Available
	}
	if stats.FreeSet && stats.Total >= stats.Free {
		return stats.Total - stats.Free
	}
	return 0
}

func domainMemoryValue(stats []libvirt.DomainMemoryStat, tag libvirt.DomainMemoryStatTags) uint64 {
	for _, stat := range stats {
		if stat.Tag == int32(tag) {
			return stat.Val
		}
	}
	return 0
}

type domainInterfaceTargetsXML struct {
	Devices struct {
		Interfaces []struct {
			Target struct {
				Dev string `xml:"dev,attr"`
			} `xml:"target"`
		} `xml:"interface"`
	} `xml:"devices"`
}

func parseDomainInterfaceTargets(text string) []string {
	var parsed domainInterfaceTargetsXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		return nil
	}
	targets := make([]string, 0, len(parsed.Devices.Interfaces))
	for _, iface := range parsed.Devices.Interfaces {
		if target := strings.TrimSpace(iface.Target.Dev); target != "" {
			targets = append(targets, target)
		}
	}
	return targets
}

func domainNetworkCounters(dom *libvirt.Domain) (uint64, uint64) {
	xmlText, err := dom.GetXMLDesc(0)
	if err != nil {
		return 0, 0
	}
	var rxBytes uint64
	var txBytes uint64
	for _, target := range parseDomainInterfaceTargets(xmlText) {
		stats, err := dom.InterfaceStats(target)
		if err != nil {
			continue
		}
		if stats.RxBytesSet && stats.RxBytes > 0 {
			rxBytes += uint64(stats.RxBytes)
		}
		if stats.TxBytesSet && stats.TxBytes > 0 {
			txBytes += uint64(stats.TxBytes)
		}
	}
	return rxBytes, txBytes
}

func readHostNetworkCounters() (uint64, uint64) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	return parseHostNetworkCounters(file)
}

func parseHostNetworkCounters(reader io.Reader) (uint64, uint64) {
	scanner := bufio.NewScanner(reader)
	var rxBytes uint64
	var txBytes uint64
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if !countHostInterface(name) {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(line[colon+1:]))
		if len(fields) < 9 {
			continue
		}
		rx, rxErr := strconv.ParseUint(fields[0], 10, 64)
		tx, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr == nil {
			rxBytes += rx
		}
		if txErr == nil {
			txBytes += tx
		}
	}
	return rxBytes, txBytes
}

func countHostInterface(name string) bool {
	if name == "" || name == "lo" {
		return false
	}
	for _, prefix := range []string{"vnet", "virbr", "docker", "br-", "tap", "tun", "veth"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}
