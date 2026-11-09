package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) ListVMs(ctx context.Context) ([]domain.VM, error) {
	c.mu.Lock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	domains, err := c.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("list libvirt domains: %w", err)
	}
	result := make([]domain.VM, 0, len(domains))
	for i := range domains {
		dom := &domains[i]
		vm, err := domainFromLibvirt(dom)
		_ = dom.Free()
		if err != nil {
			c.mu.Unlock()
			return nil, err
		}
		c.populateDiskStorageInfoLocked(&vm)
		result = append(result, vm)
	}
	c.mu.Unlock()
	c.populateGuestInfoBatch(ctx, result)
	return result, nil
}

func (c *Client) GetVM(ctx context.Context, uuid string) (domain.VM, error) {
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
	vm, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	populateGuestInfo(dom, &vm)
	return vm, nil
}

func (c *Client) GetVMXML(ctx context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VMXML{}, err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VMXML{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	name, err := dom.GetName()
	if err != nil {
		return domain.VMXML{}, fmt.Errorf("read domain %s name: %w", uuid, err)
	}
	var flags libvirt.DomainXMLFlags
	if format == domain.VMXMLFormatConfig {
		flags = libvirt.DOMAIN_XML_INACTIVE
	}
	xmlText, err := dom.GetXMLDesc(flags)
	if err != nil {
		return domain.VMXML{}, fmt.Errorf("read domain %s %s xml: %w", uuid, format, err)
	}
	return domain.VMXML{UUID: uuid, Name: name, Format: format, XML: xmlText}, nil
}

func (c *Client) DefineVMXML(ctx context.Context, uuid, xmlText string) (domain.VMXML, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VMXML{}, err
	}
	existing, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VMXML{}, mapDomainLookupError(uuid, err)
	}
	name, nameErr := existing.GetName()
	persistent, persistentErr := existing.IsPersistent()
	_ = existing.Free()
	if nameErr != nil {
		return domain.VMXML{}, fmt.Errorf("read domain %s name: %w", uuid, nameErr)
	}
	if persistentErr != nil {
		return domain.VMXML{}, fmt.Errorf("read domain %s persistence: %w", uuid, persistentErr)
	}
	if !persistent {
		return domain.VMXML{}, domain.ErrActionNotAllowed
	}
	identity, err := domain.ParseVMXMLIdentity(xmlText)
	if err != nil {
		return domain.VMXML{}, err
	}
	if identity.UUID != uuid || identity.Name != name {
		return domain.VMXML{}, fmt.Errorf("%w: domain name and UUID must match the existing VM", domain.ErrInvalidInput)
	}

	defined, err := c.conn.DomainDefineXMLFlags(xmlText, libvirt.DOMAIN_DEFINE_VALIDATE)
	if err != nil {
		switch {
		case errors.Is(err, libvirt.ERR_NO_DOMAIN):
			return domain.VMXML{}, domain.ErrNotFound
		case errors.Is(err, libvirt.ERR_INVALID_ARG), errors.Is(err, libvirt.ERR_XML_ERROR), errors.Is(err, libvirt.ERR_INVALID_DOMAIN):
			return domain.VMXML{}, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		case errors.Is(err, libvirt.ERR_OPERATION_INVALID):
			return domain.VMXML{}, errors.Join(domain.ErrActionNotAllowed, err)
		default:
			return domain.VMXML{}, fmt.Errorf("define libvirt domain %s XML: %w", uuid, err)
		}
	}
	defer func() { _ = defined.Free() }()
	definedUUID, err := defined.GetUUIDString()
	if err != nil {
		return domain.VMXML{}, fmt.Errorf("read updated domain UUID: %w", err)
	}
	definedName, err := defined.GetName()
	if err != nil {
		return domain.VMXML{}, fmt.Errorf("read updated domain name: %w", err)
	}
	if !strings.EqualFold(definedUUID, uuid) || definedName != name {
		return domain.VMXML{}, fmt.Errorf("%w: libvirt returned a different domain identity", domain.ErrConflict)
	}
	updatedXML, err := defined.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.VMXML{}, fmt.Errorf("read updated domain %s config XML: %w", uuid, err)
	}
	return domain.VMXML{UUID: uuid, Name: name, Format: domain.VMXMLFormatConfig, XML: updatedXML}, nil
}

func (c *Client) UpdateVMResources(ctx context.Context, uuid string, spec domain.VMResourcesSpec) (domain.VM, error) {
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
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("domain is not persistent"))
	}
	active, err := dom.IsActive()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s active state: %w", uuid, err)
	}
	info, err := dom.GetInfo()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s info: %w", uuid, err)
	}

	maxVCPUs := info.NrVirtCpu
	vcpuFlags := libvirt.DomainVcpuFlags(libvirt.DOMAIN_VCPU_CONFIG | libvirt.DOMAIN_VCPU_MAXIMUM)
	if active {
		vcpuFlags = libvirt.DOMAIN_VCPU_LIVE | libvirt.DOMAIN_VCPU_MAXIMUM
	}
	if maximum, err := dom.GetVcpusFlags(vcpuFlags); err == nil && maximum > 0 {
		maxVCPUs = uint(maximum)
	}
	targetMemoryBytes := spec.MemoryMiB * 1024 * 1024
	if active && (spec.VCPUs > maxVCPUs || targetMemoryBytes > info.MaxMem*1024) {
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("requested resources exceed the active domain maximum"))
	}

	if active {
		if spec.VCPUs != info.NrVirtCpu {
			if err := dom.SetVcpusFlags(spec.VCPUs, libvirt.DOMAIN_VCPU_LIVE|libvirt.DOMAIN_VCPU_CONFIG); err != nil {
				return domain.VM{}, mapVMResourceUpdateError(uuid, "vCPU", err)
			}
		}
		if targetMemoryBytes != info.Memory*1024 {
			if err := dom.SetMemoryFlags(spec.MemoryMiB*1024, libvirt.DOMAIN_MEM_LIVE|libvirt.DOMAIN_MEM_CONFIG); err != nil {
				return domain.VM{}, mapVMResourceUpdateError(uuid, "memory", err)
			}
		}
	} else {
		if spec.VCPUs > maxVCPUs {
			if err := dom.SetVcpusFlags(spec.VCPUs, libvirt.DOMAIN_VCPU_CONFIG|libvirt.DOMAIN_VCPU_MAXIMUM); err != nil {
				return domain.VM{}, mapVMResourceUpdateError(uuid, "maximum vCPU", err)
			}
		}
		if spec.VCPUs != info.NrVirtCpu {
			if err := dom.SetVcpusFlags(spec.VCPUs, libvirt.DOMAIN_VCPU_CONFIG); err != nil {
				return domain.VM{}, mapVMResourceUpdateError(uuid, "vCPU", err)
			}
		}
		targetMemoryKiB := spec.MemoryMiB * 1024
		if spec.MemoryMiB*1024*1024 > info.MaxMem*1024 {
			if err := dom.SetMemoryFlags(targetMemoryKiB, libvirt.DOMAIN_MEM_CONFIG|libvirt.DOMAIN_MEM_MAXIMUM); err != nil {
				return domain.VM{}, mapVMResourceUpdateError(uuid, "maximum memory", err)
			}
		}
		if spec.MemoryMiB*1024 != info.Memory {
			if err := dom.SetMemoryFlags(targetMemoryKiB, libvirt.DOMAIN_MEM_CONFIG); err != nil {
				return domain.VM{}, mapVMResourceUpdateError(uuid, "memory", err)
			}
		}
	}
	updated, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&updated)
	return updated, nil
}

func mapVMResourceUpdateError(uuid, resource string, err error) error {
	switch {
	case errors.Is(err, libvirt.ERR_NO_DOMAIN):
		return domain.ErrNotFound
	case errors.Is(err, libvirt.ERR_OPERATION_UNSUPPORTED),
		errors.Is(err, libvirt.ERR_OPERATION_INVALID),
		errors.Is(err, libvirt.ERR_INVALID_ARG):
		return errors.Join(domain.ErrActionNotAllowed, fmt.Errorf("update %s: %w", resource, err))
	default:
		return fmt.Errorf("update %s for domain %s: %w", resource, uuid, err)
	}
}

func (c *Client) PerformAction(ctx context.Context, uuid string, action domain.VMAction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()

	switch action {
	case domain.VMActionStart:
		err = dom.Create()
	case domain.VMActionShutdown:
		err = dom.Shutdown()
	case domain.VMActionReboot:
		err = dom.Reboot(0)
	case domain.VMActionReset:
		err = dom.Reset(0)
	case domain.VMActionPause:
		err = dom.Suspend()
	case domain.VMActionResume:
		err = dom.Resume()
	case domain.VMActionDestroy:
		err = dom.Destroy()
	default:
		return domain.ErrInvalidInput
	}
	if err != nil {
		switch {
		case errors.Is(err, libvirt.ERR_NO_DOMAIN):
			return domain.ErrNotFound
		case errors.Is(err, libvirt.ERR_OPERATION_INVALID):
			return errors.Join(domain.ErrActionNotAllowed, err)
		default:
			return fmt.Errorf("perform libvirt action %s on domain %s: %w", action, uuid, err)
		}
	}
	return nil
}

func (c *Client) SetVMAutostart(ctx context.Context, uuid string, enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	if err := dom.SetAutostart(enabled); err != nil {
		switch {
		case errors.Is(err, libvirt.ERR_NO_DOMAIN):
			return domain.ErrNotFound
		case errors.Is(err, libvirt.ERR_OPERATION_INVALID):
			return errors.Join(domain.ErrActionNotAllowed, err)
		default:
			return fmt.Errorf("set libvirt domain %s autostart: %w", uuid, err)
		}
	}
	return nil
}

func mapDomainLookupError(uuid string, err error) error {
	if errors.Is(err, libvirt.ERR_NO_DOMAIN) {
		return domain.ErrNotFound
	}
	return fmt.Errorf("lookup libvirt domain %s: %w", uuid, err)
}

func domainFromLibvirt(dom *libvirt.Domain) (domain.VM, error) {
	name, err := dom.GetName()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain name: %w", err)
	}
	uuid, err := dom.GetUUIDString()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s uuid: %w", name, err)
	}
	state, reason, err := dom.GetState()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s state: %w", name, err)
	}
	info, err := dom.GetInfo()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s info: %w", name, err)
	}
	persistent, _ := dom.IsPersistent()
	autostart, _ := dom.GetAutostart()
	xmlText, err := dom.GetXMLDesc(0)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s xml: %w", name, err)
	}
	details, err := parseDomainXML(xmlText)
	if err != nil {
		return domain.VM{}, fmt.Errorf("parse domain %s xml: %w", name, err)
	}
	for index := range details.Disks {
		disk := &details.Disks[index]
		if disk.Device != "disk" || disk.ReadOnly || disk.Target == "" {
			continue
		}
		diskInfo, infoErr := dom.GetBlockInfo(disk.Target, 0)
		if infoErr != nil {
			continue
		}
		disk.CapacityBytes = diskInfo.Capacity
		disk.AllocationBytes = diskInfo.Allocation
	}
	maxVCPUs := info.NrVirtCpu
	vcpuFlags := libvirt.DomainVcpuFlags(libvirt.DOMAIN_VCPU_CONFIG | libvirt.DOMAIN_VCPU_MAXIMUM)
	if state != libvirt.DOMAIN_SHUTOFF && state != libvirt.DOMAIN_CRASHED {
		vcpuFlags = libvirt.DOMAIN_VCPU_LIVE | libvirt.DOMAIN_VCPU_MAXIMUM
	}
	if maximum, err := dom.GetVcpusFlags(vcpuFlags); err == nil && maximum > 0 {
		maxVCPUs = uint(maximum)
	}
	return domain.VM{
		UUID:           uuid,
		Name:           name,
		State:          mapDomainState(state),
		StateReason:    reason,
		Persistent:     persistent,
		Autostart:      autostart,
		VCPUs:          info.NrVirtCpu,
		MaxVCPUs:       maxVCPUs,
		MemoryBytes:    info.Memory * 1024,
		MaxMemoryBytes: info.MaxMem * 1024,
		CPUTimeNanos:   info.CpuTime,
		OSType:         details.OSType,
		Arch:           details.Arch,
		Machine:        details.Machine,
		Firmware:       details.Firmware,
		SecureBoot:     details.SecureBoot,
		CloudInit:      details.CloudInit,
		Disks:          details.Disks,
		Interfaces:     details.Interfaces,
		Graphics:       details.Graphics,
		Serials:        details.Serials,
		Passthrough:    details.Passthrough,
		IPAddresses:    []string{},
		AllowedActions: allowedActions(state),
	}, nil
}

func mapDomainState(state libvirt.DomainState) domain.VMState {
	switch state {
	case libvirt.DOMAIN_RUNNING:
		return domain.VMStateRunning
	case libvirt.DOMAIN_BLOCKED:
		return domain.VMStateBlocked
	case libvirt.DOMAIN_PAUSED:
		return domain.VMStatePaused
	case libvirt.DOMAIN_SHUTDOWN:
		return domain.VMStateShutdown
	case libvirt.DOMAIN_CRASHED:
		return domain.VMStateCrashed
	case libvirt.DOMAIN_PMSUSPENDED:
		return domain.VMStatePMSuspended
	case libvirt.DOMAIN_SHUTOFF:
		return domain.VMStateShutoff
	default:
		return domain.VMStateUnknown
	}
}

func allowedActions(state libvirt.DomainState) []domain.VMAction {
	switch state {
	case libvirt.DOMAIN_RUNNING, libvirt.DOMAIN_BLOCKED:
		return []domain.VMAction{
			domain.VMActionShutdown, domain.VMActionReboot, domain.VMActionReset,
			domain.VMActionPause, domain.VMActionDestroy,
		}
	case libvirt.DOMAIN_PAUSED:
		return []domain.VMAction{domain.VMActionResume, domain.VMActionDestroy}
	case libvirt.DOMAIN_SHUTOFF, libvirt.DOMAIN_CRASHED:
		return []domain.VMAction{domain.VMActionStart}
	default:
		return []domain.VMAction{}
	}
}
