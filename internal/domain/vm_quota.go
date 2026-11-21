package domain

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultCPUPeriod = uint64(100_000)
	maxCPUPeriod     = uint64(1_000_000)
	maxCPUShares     = uint64(262_144)
	maxCPUQuota      = uint64(1 << 40)
	maxQuotaMemory   = uint64(1 << 20)
)

// VMResourceQuota contains cgroup-level CPU and memory guarantees. CPU quota
// and period are expressed in microseconds; memory values are expressed in MiB.
type VMResourceQuota struct {
	CPUShares          uint64 `json:"cpuShares,omitempty"`
	CPUQuota           uint64 `json:"cpuQuota,omitempty"`
	CPUPeriod          uint64 `json:"cpuPeriod,omitempty"`
	MemoryHardLimitMiB uint64 `json:"memoryHardLimitMiB,omitempty"`
	MemorySoftLimitMiB uint64 `json:"memorySoftLimitMiB,omitempty"`
	// MemoryMinGuaranteeMiB is retained for XML read compatibility. libvirt's
	// QEMU driver rejects it, so API writes must leave it at zero.
	MemoryMinGuaranteeMiB uint64 `json:"memoryMinGuaranteeMiB,omitempty"`
}

func (quota VMResourceQuota) Empty() bool {
	return quota == (VMResourceQuota{})
}

func NormalizeVMResourceQuota(quota VMResourceQuota, memoryMiB uint64) (VMResourceQuota, error) {
	if quota.CPUShares != 0 && (quota.CPUShares < 2 || quota.CPUShares > maxCPUShares) {
		return VMResourceQuota{}, fmt.Errorf("%w: CPU shares must be between 2 and %d", ErrInvalidInput, maxCPUShares)
	}
	if quota.CPUQuota > maxCPUQuota {
		return VMResourceQuota{}, fmt.Errorf("%w: CPU quota is too large", ErrInvalidInput)
	}
	if quota.CPUQuota == 0 && quota.CPUPeriod != 0 {
		return VMResourceQuota{}, fmt.Errorf("%w: CPU period requires a CPU quota", ErrInvalidInput)
	}
	if quota.CPUQuota > 0 && quota.CPUPeriod == 0 {
		quota.CPUPeriod = defaultCPUPeriod
	}
	if quota.CPUPeriod != 0 && (quota.CPUPeriod < 1_000 || quota.CPUPeriod > maxCPUPeriod) {
		return VMResourceQuota{}, fmt.Errorf("%w: CPU period must be between 1000 and %d microseconds", ErrInvalidInput, maxCPUPeriod)
	}
	if quota.MemoryMinGuaranteeMiB != 0 {
		return VMResourceQuota{}, fmt.Errorf("%w: QEMU/KVM does not support memory min guarantee", ErrInvalidInput)
	}
	for _, field := range []struct {
		name  string
		value uint64
	}{
		{name: "memory hard limit", value: quota.MemoryHardLimitMiB},
		{name: "memory soft limit", value: quota.MemorySoftLimitMiB},
	} {
		if field.value > maxQuotaMemory {
			return VMResourceQuota{}, fmt.Errorf("%w: %s is too large", ErrInvalidInput, field.name)
		}
	}
	if quota.MemoryHardLimitMiB > 0 && memoryMiB > 0 && quota.MemoryHardLimitMiB < memoryMiB {
		return VMResourceQuota{}, fmt.Errorf("%w: memory hard limit must not be below the configured VM memory", ErrInvalidInput)
	}
	if quota.MemorySoftLimitMiB > 0 && quota.MemoryHardLimitMiB > 0 && quota.MemorySoftLimitMiB > quota.MemoryHardLimitMiB {
		return VMResourceQuota{}, fmt.Errorf("%w: memory soft limit must not exceed the hard limit", ErrInvalidInput)
	}
	return quota, nil
}

func SetVMResourceQuotaXML(text string, quota VMResourceQuota) (string, error) {
	normalized, err := NormalizeVMResourceQuota(quota, 0)
	if err != nil {
		return "", err
	}
	root, err := parseVMXMLTree(text)
	if err != nil {
		return "", err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return "", fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}

	cpuTune := firstVMXMLChild(domainNode, "cputune")
	if cpuTune != nil {
		cpuTune.Children = removeVMXMLChildren(cpuTune.Children, func(node *vmXMLNode) bool {
			switch vmXMLNodeName(node) {
			case "shares", "period", "quota":
				return true
			default:
				return false
			}
		})
		if len(cpuTune.Children) == 0 {
			domainNode.Children = removeVMXMLNode(domainNode.Children, cpuTune)
			cpuTune = nil
		}
	}
	if normalized.CPUShares > 0 || normalized.CPUQuota > 0 {
		if cpuTune == nil {
			cpuTune = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "cputune"}}}
			insertAfterVMXMLChild(domainNode, cpuTune, "cpu")
		}
		if normalized.CPUShares > 0 {
			cpuTune.Children = append(cpuTune.Children, vmXMLTextNode("shares", normalized.CPUShares))
		}
		if normalized.CPUQuota > 0 {
			cpuTune.Children = append(cpuTune.Children,
				vmXMLTextNode("period", normalized.CPUPeriod),
				vmXMLTextNode("quota", normalized.CPUQuota),
			)
		}
	}

	memoryTune := firstVMXMLChild(domainNode, "memtune")
	if memoryTune != nil {
		memoryTune.Children = removeVMXMLChildren(memoryTune.Children, func(node *vmXMLNode) bool {
			switch vmXMLNodeName(node) {
			case "hard_limit", "soft_limit", "min_guarantee":
				return true
			default:
				return false
			}
		})
		if len(memoryTune.Children) == 0 {
			domainNode.Children = removeVMXMLNode(domainNode.Children, memoryTune)
			memoryTune = nil
		}
	}
	if normalized.MemoryHardLimitMiB > 0 || normalized.MemorySoftLimitMiB > 0 {
		if memoryTune == nil {
			memoryTune = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "memtune"}}}
			insertAfterVMXMLChild(domainNode, memoryTune, "memory")
		}
		for _, field := range []struct {
			name  string
			value uint64
		}{
			{name: "hard_limit", value: normalized.MemoryHardLimitMiB},
			{name: "soft_limit", value: normalized.MemorySoftLimitMiB},
		} {
			if field.value == 0 {
				continue
			}
			memoryTune.Children = append(memoryTune.Children, &vmXMLNode{
				Token:    xml.StartElement{Name: xml.Name{Local: field.name}, Attr: []xml.Attr{{Name: xml.Name{Local: "unit"}, Value: "MiB"}}},
				Children: []*vmXMLNode{{Token: xml.CharData(strconv.FormatUint(field.value, 10))}},
			})
		}
	}
	return encodeVMXMLDocument(root)
}

func vmXMLTextNode(name string, value uint64) *vmXMLNode {
	return &vmXMLNode{
		Token:    xml.StartElement{Name: xml.Name{Local: name}},
		Children: []*vmXMLNode{{Token: xml.CharData(strconv.FormatUint(value, 10))}},
	}
}

func VMResourceQuotaFromXML(text string) (*VMResourceQuota, error) {
	root, err := parseVMXMLTree(text)
	if err != nil {
		return nil, err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return nil, fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}
	quota := VMResourceQuota{}
	if node := firstVMXMLChild(domainNode, "cputune"); node != nil {
		quota.CPUShares = parseVMXMLUint(vmXMLChildText(node, "shares", ""))
		quota.CPUPeriod = parseVMXMLUint(vmXMLChildText(node, "period", ""))
		quota.CPUQuota = parseVMXMLUint(vmXMLChildText(node, "quota", ""))
	}
	if node := firstVMXMLChild(domainNode, "memtune"); node != nil {
		quota.MemoryHardLimitMiB = memoryMiBFromXML(node, "hard_limit")
		quota.MemorySoftLimitMiB = memoryMiBFromXML(node, "soft_limit")
		quota.MemoryMinGuaranteeMiB = memoryMiBFromXML(node, "min_guarantee")
	}
	if quota.Empty() {
		return nil, nil
	}
	return &quota, nil
}

func parseVMXMLUint(value string) uint64 {
	parsed, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return parsed
}

func memoryMiBFromXML(parent *vmXMLNode, name string) uint64 {
	node := firstVMXMLChild(parent, name)
	if node == nil {
		return 0
	}
	value := uint64(0)
	for _, child := range node.Children {
		if text, ok := child.Token.(xml.CharData); ok {
			value = parseVMXMLUint(string(text))
			break
		}
	}
	switch strings.ToLower(strings.TrimSpace(vmXMLAttribute(node, "unit"))) {
	case "b", "bytes":
		return (value + (1 << 20) - 1) >> 20
	case "kb", "kib":
		return (value + 1023) / 1024
	case "gb", "gib":
		return value * 1024
	default:
		return value
	}
}
