package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

const (
	defaultLiveCPUShares = uint64(1024)
	defaultLiveCPUPeriod = uint64(100_000)
)

func (c *Client) UpdateVMResourceQuota(ctx context.Context, uuid string, quota domain.VMResourceQuota) (domain.VM, error) {
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
		return domain.VM{}, errors.Join(domain.ErrActionNotAllowed, errors.New("domain is not persistent"))
	}
	active, err := dom.IsActive()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s active state: %w", uuid, err)
	}
	var previousLiveQuota *domain.VMResourceQuota
	if active {
		liveXML, err := dom.GetXMLDesc(0)
		if err != nil {
			return domain.VM{}, fmt.Errorf("read domain %s live XML: %w", uuid, err)
		}
		previousLiveQuota, err = domain.VMResourceQuotaFromXML(liveXML)
		if err != nil {
			return domain.VM{}, fmt.Errorf("read domain %s live quota: %w", uuid, err)
		}
	}
	configXML, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s config XML: %w", uuid, err)
	}
	updatedXML, err := domain.SetVMResourceQuotaXML(configXML, quota)
	if err != nil {
		return domain.VM{}, err
	}
	if active {
		if err := applyVMResourceQuotaLive(dom, quota); err != nil {
			mapped := mapVMResourceUpdateError(uuid, "live quota", err)
			if rollbackErr := rollbackVMResourceQuotaLive(dom, previousLiveQuota); rollbackErr != nil {
				return domain.VM{}, errors.Join(mapped, fmt.Errorf("rollback domain %s live quota: %w", uuid, rollbackErr))
			}
			return domain.VM{}, mapped
		}
	}
	defined, err := c.conn.DomainDefineXMLFlags(updatedXML, libvirt.DOMAIN_DEFINE_VALIDATE)
	if err != nil {
		mapped := mapVMResourceUpdateError(uuid, "quota config", err)
		if active {
			if rollbackErr := rollbackVMResourceQuotaLive(dom, previousLiveQuota); rollbackErr != nil {
				return domain.VM{}, errors.Join(mapped, fmt.Errorf("rollback domain %s live quota: %w", uuid, rollbackErr))
			}
		}
		return domain.VM{}, mapped
	}
	defer func() { _ = defined.Free() }()
	updated, err := domainFromLibvirt(defined)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&updated)
	setVMPersistentQuota(&updated, quota)
	return updated, nil
}

func applyVMResourceQuotaLive(dom *libvirt.Domain, quota domain.VMResourceQuota) error {
	flags := libvirt.DomainModificationImpact(libvirt.DOMAIN_AFFECT_LIVE)
	if err := dom.SetSchedulerParametersFlags(vmResourceQuotaSchedulerParameters(quota), flags); err != nil {
		return err
	}
	if err := dom.SetMemoryParameters(vmResourceQuotaMemoryParameters(quota), flags); err != nil {
		return err
	}
	return nil
}

func rollbackVMResourceQuotaLive(dom *libvirt.Domain, quota *domain.VMResourceQuota) error {
	if quota == nil {
		return applyVMResourceQuotaLive(dom, domain.VMResourceQuota{})
	}
	return applyVMResourceQuotaLive(dom, *quota)
}

func vmResourceQuotaSchedulerParameters(quota domain.VMResourceQuota) *libvirt.DomainSchedulerParameters {
	shares := quota.CPUShares
	if shares == 0 {
		shares = defaultLiveCPUShares
	}
	period := quota.CPUPeriod
	if period == 0 {
		period = defaultLiveCPUPeriod
	}
	globalQuota := int64(quota.CPUQuota)
	if globalQuota == 0 {
		globalQuota = -1
	}
	return &libvirt.DomainSchedulerParameters{
		CpuSharesSet:    true,
		CpuShares:       shares,
		GlobalPeriodSet: true,
		GlobalPeriod:    period,
		GlobalQuotaSet:  true,
		GlobalQuota:     globalQuota,
	}
}

func vmResourceQuotaMemoryParameters(quota domain.VMResourceQuota) *libvirt.DomainMemoryParameters {
	hardLimit := quota.MemoryHardLimitMiB << 10
	if hardLimit == 0 {
		hardLimit = libvirt.DOMAIN_MEMORY_PARAM_UNLIMITED
	}
	softLimit := quota.MemorySoftLimitMiB << 10
	if softLimit == 0 {
		softLimit = libvirt.DOMAIN_MEMORY_PARAM_UNLIMITED
	}
	return &libvirt.DomainMemoryParameters{
		HardLimitSet: true,
		HardLimit:    hardLimit,
		SoftLimitSet: true,
		SoftLimit:    softLimit,
	}
}

func setVMPersistentQuota(vm *domain.VM, quota domain.VMResourceQuota) {
	if quota.Empty() {
		vm.ResourceQuota = nil
		return
	}
	vm.ResourceQuota = &quota
}
