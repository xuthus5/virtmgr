package libvirtbackend

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) UpdateVMPlacement(ctx context.Context, uuid string, spec domain.VMPlacementSpec) (domain.VM, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VM{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	persistent, err := dom.IsPersistent()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s persistence: %w", uuid, err)
	}
	if !persistent {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	state, _, err := dom.GetState()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s state: %w", uuid, err)
	}
	if state != libvirt.DOMAIN_SHUTOFF && state != libvirt.DOMAIN_CRASHED {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	if err := validateHostPlacement(c.hostPlatformLocked(ctx), spec); err != nil {
		return domain.VM{}, err
	}
	xmlText, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read persistent domain %s xml: %w", uuid, err)
	}
	updatedXML, err := domain.SetVMXMLPlacement(xmlText, spec)
	if err != nil {
		return domain.VM{}, err
	}
	defined, err := c.conn.DomainDefineXMLFlags(updatedXML, libvirt.DOMAIN_DEFINE_VALIDATE)
	if err != nil {
		return domain.VM{}, fmt.Errorf("define domain %s placement: %w", uuid, err)
	}
	defer func() { _ = defined.Free() }()
	vm, err := domainFromLibvirt(defined)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	return vm, nil
}

func validateHostPlacement(platform hostPlatform, spec domain.VMPlacementSpec) error {
	if !platform.known {
		return nil
	}
	online := make([]uint, 0)
	for _, node := range platform.capabilities.NUMANodes {
		online = append(online, node.CPUs...)
	}
	slices.Sort(online)
	online = slices.Compact(online)
	checkCPUSet := func(label, cpuset string) error {
		cpus, ok := domain.ParseCPUSet(cpuset)
		if !ok {
			return fmt.Errorf("%w: %s cpuset is invalid", domain.ErrInvalidInput, label)
		}
		if len(online) == 0 {
			return nil
		}
		for _, cpu := range cpus {
			if !slices.Contains(online, cpu) {
				return fmt.Errorf("%w: %s references offline CPU %d", domain.ErrInvalidInput, label, cpu)
			}
		}
		return nil
	}
	for _, cell := range spec.NUMACells {
		if err := checkCPUSet("NUMA cell", cell.Cpuset); err != nil {
			return err
		}
	}
	for _, pin := range spec.VCPUPins {
		if err := checkCPUSet("vCPU pin", pin.Cpuset); err != nil {
			return err
		}
	}
	if spec.EmulatorPin != "" {
		if err := checkCPUSet("emulator pin", spec.EmulatorPin); err != nil {
			return err
		}
	}
	if spec.HugePageSize == "" {
		return nil
	}
	pageSize, err := hugePageBytes(spec.HugePageSize)
	if err != nil {
		return err
	}
	for _, page := range platform.capabilities.HugePages {
		if page.SizeBytes == pageSize {
			if page.Total == 0 {
				return fmt.Errorf("%w: host has no reserved %s huge pages", domain.ErrInvalidInput, spec.HugePageSize)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: host does not support %s huge pages", domain.ErrInvalidInput, spec.HugePageSize)
}

func hugePageBytes(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	for suffix, shift := range map[string]uint{"KiB": 10, "MiB": 20, "GiB": 30} {
		sizeText, ok := strings.CutSuffix(value, suffix)
		if !ok {
			continue
		}
		size, err := strconv.ParseUint(sizeText, 10, 64)
		if err != nil || size == 0 {
			return 0, domain.ErrInvalidInput
		}
		if size > ^uint64(0)>>shift {
			return 0, domain.ErrInvalidInput
		}
		return size << shift, nil
	}
	return 0, domain.ErrInvalidInput
}
