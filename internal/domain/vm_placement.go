package domain

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var hugePageSizePattern = regexp.MustCompile(`^(\d+)(KiB|MiB|GiB)$`)

const maxCPUSetMembers = 1 << 16

func NormalizeVMPlacement(spec VMPlacementSpec, vcpus, memoryMiB uint64) (VMPlacementSpec, error) {
	spec.EmulatorPin = strings.TrimSpace(spec.EmulatorPin)
	spec.HugePageSize = strings.TrimSpace(spec.HugePageSize)
	if spec.HugePageSize == "none" || spec.HugePageSize == "0" {
		spec.HugePageSize = ""
	}
	if spec.HugePageSize != "" {
		match := hugePageSizePattern.FindStringSubmatch(spec.HugePageSize)
		if match == nil {
			return VMPlacementSpec{}, fmt.Errorf("%w: huge page size must use KiB, MiB or GiB", ErrInvalidInput)
		}
		size, _ := strconv.ParseUint(match[1], 10, 64)
		if size == 0 {
			return VMPlacementSpec{}, fmt.Errorf("%w: huge page size must be positive", ErrInvalidInput)
		}
		spec.HugePageSize = strconv.FormatUint(size, 10) + match[2]
		pageBytes, ok := parseHugePageBytes(spec.HugePageSize)
		if !ok {
			return VMPlacementSpec{}, fmt.Errorf("%w: huge page size is too large", ErrInvalidInput)
		}
		if memoryMiB > ^uint64(0)>>20 {
			return VMPlacementSpec{}, fmt.Errorf("%w: VM memory is too large", ErrInvalidInput)
		}
		if memoryMiB > 0 && pageBytes > 0 && (memoryMiB<<20)%pageBytes != 0 {
			return VMPlacementSpec{}, fmt.Errorf("%w: huge page size must divide VM memory evenly", ErrInvalidInput)
		}
	}
	if spec.EmulatorPin != "" {
		if _, ok := ParseCPUSet(spec.EmulatorPin); !ok {
			return VMPlacementSpec{}, fmt.Errorf("%w: emulator pin cpuset is invalid", ErrInvalidInput)
		}
	}
	seenVCPU := map[uint]struct{}{}
	for index := range spec.VCPUPins {
		pin := &spec.VCPUPins[index]
		pin.Cpuset = strings.TrimSpace(pin.Cpuset)
		if vcpus == 0 || uint64(pin.VCPU) >= vcpus {
			return VMPlacementSpec{}, fmt.Errorf("%w: vCPU pin index is out of range", ErrInvalidInput)
		}
		if _, ok := ParseCPUSet(pin.Cpuset); !ok {
			return VMPlacementSpec{}, fmt.Errorf("%w: vCPU pin cpuset is invalid", ErrInvalidInput)
		}
		if _, exists := seenVCPU[pin.VCPU]; exists {
			return VMPlacementSpec{}, fmt.Errorf("%w: vCPU pin is duplicated", ErrInvalidInput)
		}
		seenVCPU[pin.VCPU] = struct{}{}
	}
	slices.SortFunc(spec.VCPUPins, func(left, right VMCpuPin) int { return int(left.VCPU) - int(right.VCPU) })

	seenCell := map[uint]struct{}{}
	memoryTotal := uint64(0)
	assignedCPUs := map[uint]struct{}{}
	assignedCPUCount := 0
	for index := range spec.NUMACells {
		cell := &spec.NUMACells[index]
		cell.Cpuset = strings.TrimSpace(cell.Cpuset)
		if cell.MemoryMiB == 0 || cell.Cpuset == "" {
			return VMPlacementSpec{}, fmt.Errorf("%w: NUMA cell cpuset and memory are required", ErrInvalidInput)
		}
		cpus, ok := ParseCPUSet(cell.Cpuset)
		if !ok {
			return VMPlacementSpec{}, fmt.Errorf("%w: NUMA cell cpuset is invalid", ErrInvalidInput)
		}
		if _, exists := seenCell[cell.ID]; exists {
			return VMPlacementSpec{}, fmt.Errorf("%w: NUMA cell ID is duplicated", ErrInvalidInput)
		}
		seenCell[cell.ID] = struct{}{}
		for _, cpu := range cpus {
			if _, exists := assignedCPUs[cpu]; exists {
				return VMPlacementSpec{}, fmt.Errorf("%w: NUMA cell CPU sets overlap", ErrInvalidInput)
			}
			assignedCPUs[cpu] = struct{}{}
		}
		assignedCPUCount += len(cpus)
		if memoryTotal > ^uint64(0)-cell.MemoryMiB {
			return VMPlacementSpec{}, fmt.Errorf("%w: NUMA cell memory is too large", ErrInvalidInput)
		}
		memoryTotal += cell.MemoryMiB
	}
	if len(spec.NUMACells) > 0 && vcpus > 0 && uint64(assignedCPUCount) != vcpus {
		return VMPlacementSpec{}, fmt.Errorf("%w: NUMA cell CPU count must equal VM vCPU count", ErrInvalidInput)
	}
	if len(spec.NUMACells) > 0 && memoryTotal != memoryMiB {
		return VMPlacementSpec{}, fmt.Errorf("%w: NUMA cell memory must equal VM memory", ErrInvalidInput)
	}
	slices.SortFunc(spec.NUMACells, func(left, right VMNUMACell) int { return int(left.ID) - int(right.ID) })
	return spec, nil
}

func parseHugePageBytes(value string) (uint64, bool) {
	match := hugePageSizePattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return 0, false
	}
	size, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || size == 0 {
		return 0, false
	}
	shift := uint(0)
	switch match[2] {
	case "KiB":
		shift = 10
	case "MiB":
		shift = 20
	case "GiB":
		shift = 30
	default:
		return 0, false
	}
	if size > ^uint64(0)>>shift {
		return 0, false
	}
	return size << shift, true
}

// ParseCPUSet expands a libvirt cpuset such as "0-3,8" into sorted CPU IDs.
func ParseCPUSet(value string) ([]uint, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	seen := map[uint]struct{}{}
	processed := 0
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
		if end-start+1 > maxCPUSetMembers || processed+int(end-start+1) > maxCPUSetMembers {
			return nil, false
		}
		for cpu := start; cpu <= end; cpu++ {
			seen[uint(cpu)] = struct{}{}
			processed++
		}
	}
	result := make([]uint, 0, len(seen))
	for cpu := range seen {
		result = append(result, cpu)
	}
	slices.Sort(result)
	return result, true
}

func SetVMXMLPlacement(text string, spec VMPlacementSpec) (string, error) {
	root, err := parseVMXMLTree(text)
	if err != nil {
		return "", err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return "", fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}
	cpuNode := firstVMXMLChild(domainNode, "cpu")
	needsCPU := len(spec.NUMACells) > 0 || len(spec.VCPUPins) > 0 || spec.EmulatorPin != ""
	if cpuNode == nil && needsCPU {
		cpuNode = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "cpu"}, Attr: []xml.Attr{{Name: xml.Name{Local: "mode"}, Value: "host-passthrough"}}}}
		insertAt := len(domainNode.Children)
		for index, child := range domainNode.Children {
			if vmXMLNodeName(child) == "os" {
				insertAt = index + 1
				break
			}
		}
		domainNode.Children = append(domainNode.Children, nil)
		copy(domainNode.Children[insertAt+1:], domainNode.Children[insertAt:])
		domainNode.Children[insertAt] = cpuNode
	}
	if cpuNode != nil {
		cpuNode.Children = removeVMXMLChildren(cpuNode.Children, func(node *vmXMLNode) bool { return vmXMLNodeName(node) == "numa" })
		if len(spec.NUMACells) > 0 {
			numa := &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "numa"}}}
			for _, cell := range spec.NUMACells {
				numa.Children = append(numa.Children, &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "cell"}, Attr: []xml.Attr{
					{Name: xml.Name{Local: "id"}, Value: strconv.FormatUint(uint64(cell.ID), 10)},
					{Name: xml.Name{Local: "cpus"}, Value: cell.Cpuset},
					{Name: xml.Name{Local: "memory"}, Value: strconv.FormatUint(cell.MemoryMiB, 10)},
					{Name: xml.Name{Local: "unit"}, Value: "MiB"},
				}}})
			}
			cpuNode.Children = append(cpuNode.Children, numa)
		}
	}
	cpuTune := firstVMXMLChild(domainNode, "cputune")
	if cpuTune != nil {
		cpuTune.Children = removeVMXMLChildren(cpuTune.Children, func(node *vmXMLNode) bool {
			name := vmXMLNodeName(node)
			return name == "vcpupin" || name == "emulatorpin"
		})
		if len(cpuTune.Children) == 0 {
			domainNode.Children = removeVMXMLNode(domainNode.Children, cpuTune)
			cpuTune = nil
		}
	}
	if len(spec.VCPUPins) > 0 || spec.EmulatorPin != "" {
		cleanup := false
		if cpuTune == nil {
			cpuTune = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "cputune"}}}
			cleanup = true
		}
		for _, pin := range spec.VCPUPins {
			cpuTune.Children = append(cpuTune.Children, &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "vcpupin"}, Attr: []xml.Attr{
				{Name: xml.Name{Local: "vcpu"}, Value: strconv.FormatUint(uint64(pin.VCPU), 10)},
				{Name: xml.Name{Local: "cpuset"}, Value: pin.Cpuset},
			}}})
		}
		if spec.EmulatorPin != "" {
			cpuTune.Children = append(cpuTune.Children, &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "emulatorpin"}, Attr: []xml.Attr{{Name: xml.Name{Local: "cpuset"}, Value: spec.EmulatorPin}}}})
		}
		if cleanup {
			insertAfterVMXMLChild(domainNode, cpuTune, "cpu")
		}
	}
	memoryBacking := firstVMXMLChild(domainNode, "memoryBacking")
	if memoryBacking != nil {
		memoryBacking.Children = removeVMXMLChildren(memoryBacking.Children, func(node *vmXMLNode) bool { return vmXMLNodeName(node) == "hugepages" })
		if len(memoryBacking.Children) == 0 {
			domainNode.Children = removeVMXMLNode(domainNode.Children, memoryBacking)
			memoryBacking = nil
		}
	}
	if spec.HugePageSize != "" {
		match := hugePageSizePattern.FindStringSubmatch(spec.HugePageSize)
		if match == nil {
			return "", fmt.Errorf("%w: invalid huge page size", ErrInvalidInput)
		}
		if memoryBacking == nil {
			memoryBacking = &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "memoryBacking"}}}
			insertBeforeVMXMLChild(domainNode, memoryBacking, "vcpu")
		}
		hugePages := &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "hugepages"}}}
		hugePages.Children = append(hugePages.Children, &vmXMLNode{Token: xml.StartElement{Name: xml.Name{Local: "page"}, Attr: []xml.Attr{
			{Name: xml.Name{Local: "size"}, Value: match[1]}, {Name: xml.Name{Local: "unit"}, Value: match[2]},
		}}})
		memoryBacking.Children = append([]*vmXMLNode{hugePages}, memoryBacking.Children...)
	}
	vcpuNode := firstVMXMLChild(domainNode, "vcpu")
	if vcpuNode != nil {
		if len(spec.VCPUPins) > 0 || spec.EmulatorPin != "" || len(spec.NUMACells) > 0 {
			setVMXMLAttribute(vcpuNode, "placement", "static")
		} else {
			removeVMXMLAttribute(vcpuNode, "placement")
		}
	}
	return encodeVMXMLDocument(root)
}

func insertAfterVMXMLChild(parent, child *vmXMLNode, after string) {
	index := len(parent.Children)
	for childIndex, existing := range parent.Children {
		if vmXMLNodeName(existing) == after {
			index = childIndex + 1
			break
		}
	}
	parent.Children = append(parent.Children, nil)
	copy(parent.Children[index+1:], parent.Children[index:])
	parent.Children[index] = child
}

func insertBeforeVMXMLChild(parent, child *vmXMLNode, before string) {
	index := len(parent.Children)
	for childIndex, existing := range parent.Children {
		if vmXMLNodeName(existing) == before {
			index = childIndex
			break
		}
	}
	parent.Children = append(parent.Children, nil)
	copy(parent.Children[index+1:], parent.Children[index:])
	parent.Children[index] = child
}
