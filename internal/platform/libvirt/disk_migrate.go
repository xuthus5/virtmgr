package libvirtbackend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) MigrateVMDisk(ctx context.Context, uuid, target string, spec domain.VMDiskMigrateSpec) (domain.VM, error) {
	uuid = strings.TrimSpace(uuid)
	target = strings.TrimSpace(target)
	spec.TargetPool = strings.TrimSpace(spec.TargetPool)
	spec.TargetName = strings.TrimSpace(spec.TargetName)
	spec.TargetFormat = strings.ToLower(strings.TrimSpace(spec.TargetFormat))
	if uuid == "" || target == "" || spec.TargetPool == "" || spec.TargetName == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if spec.TargetFormat != "" && spec.TargetFormat != "qcow2" && spec.TargetFormat != "raw" {
		return domain.VM{}, fmt.Errorf("%w: target format must be qcow2 or raw", domain.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return domain.VM{}, err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return domain.VM{}, fmt.Errorf("connect libvirt %s for disk migration: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()
	dom, err := conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VM{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	state, _, err := dom.GetState()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s state: %w", uuid, err)
	}
	if state != libvirt.DOMAIN_SHUTOFF && state != libvirt.DOMAIN_CRASHED {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	persistent, err := dom.IsPersistent()
	if err != nil {
		return domain.VM{}, fmt.Errorf("check domain %s persistence: %w", uuid, err)
	}
	if !persistent {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	xmlText, err := dom.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read domain %s config xml: %w", uuid, err)
	}
	disks, err := cloneDisksFromDomainXML(xmlText)
	if err != nil {
		return domain.VM{}, err
	}
	var disk cloneDisk
	for _, candidate := range disks {
		if candidate.target == target {
			disk = candidate
			break
		}
	}
	if disk.target == "" {
		return domain.VM{}, domain.ErrNotFound
	}
	sourceVolume, err := conn.LookupStorageVolByPath(disk.source)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return domain.VM{}, fmt.Errorf("%w: disk is not managed by a libvirt storage pool", domain.ErrInvalidInput)
		}
		return domain.VM{}, fmt.Errorf("lookup source disk volume %s: %w", disk.source, err)
	}
	defer func() { _ = sourceVolume.Free() }()
	sourceInfo, err := sourceVolume.GetInfo()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read source disk volume info: %w", err)
	}
	sourceName, err := sourceVolume.GetName()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read source disk volume name: %w", err)
	}
	sourceFormat, err := cloneDiskFormat(disk.format, sourceName)
	if err != nil {
		return domain.VM{}, err
	}
	if spec.TargetFormat == "" {
		spec.TargetFormat = sourceFormat
	}
	if !strings.HasSuffix(strings.ToLower(spec.TargetName), "."+spec.TargetFormat) {
		return domain.VM{}, fmt.Errorf("%w: target name must end with .%s", domain.ErrInvalidInput, spec.TargetFormat)
	}
	sourcePool, err := sourceVolume.LookupPoolByVolume()
	if err != nil {
		return domain.VM{}, fmt.Errorf("lookup source storage pool: %w", err)
	}
	sourcePoolName, nameErr := sourcePool.GetName()
	_ = sourcePool.Free()
	if nameErr != nil {
		return domain.VM{}, fmt.Errorf("read source storage pool name: %w", nameErr)
	}
	if sourcePoolName == spec.TargetPool && sourceName == spec.TargetName {
		return domain.VM{}, domain.ErrInvalidInput
	}
	targetPool, err := conn.LookupStoragePoolByName(spec.TargetPool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.VM{}, domain.ErrNotFound
		}
		return domain.VM{}, fmt.Errorf("lookup target storage pool %s: %w", spec.TargetPool, err)
	}
	defer func() { _ = targetPool.Free() }()
	targetInfo, err := targetPool.GetInfo()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read target storage pool %s info: %w", spec.TargetPool, err)
	}
	if targetInfo.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.VM{}, domain.ErrUnavailable
	}
	targetDefinition, err := storagePoolDefinition(targetPool)
	if err != nil {
		return domain.VM{}, err
	}
	if targetDefinition.Type == "logical" && spec.TargetFormat != "raw" {
		return domain.VM{}, fmt.Errorf("%w: logical LVM pools only support raw volumes", domain.ErrInvalidInput)
	}
	if existing, lookupErr := targetPool.LookupStorageVolByName(spec.TargetName); lookupErr == nil {
		_ = existing.Free()
		return domain.VM{}, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
		return domain.VM{}, fmt.Errorf("check target storage volume %s: %w", spec.TargetName, lookupErr)
	}
	targetVolume, err := targetPool.StorageVolCreateXMLFrom(buildStorageVolumeXML(domain.StorageVolumeCreateSpec{
		Name:          spec.TargetName,
		Format:        spec.TargetFormat,
		CapacityBytes: sourceInfo.Capacity,
	}, targetDefinition), sourceVolume, 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return domain.VM{}, domain.ErrConflict
		}
		return domain.VM{}, fmt.Errorf("copy disk %s to %s/%s: %w", target, spec.TargetPool, spec.TargetName, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = targetVolume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		}
		_ = targetVolume.Free()
	}()
	targetPath, err := targetVolume.GetPath()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read migrated disk path: %w", err)
	}
	targetPath = filepath.Clean(targetPath)
	rewritten, err := rewriteDomainDiskSource(xmlText, disk.source, targetPath)
	if err != nil {
		return domain.VM{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.VM{}, err
	}
	defined, err := conn.DomainDefineXML(rewritten)
	if err != nil {
		return domain.VM{}, fmt.Errorf("update domain %s disk reference: %w", uuid, err)
	}
	vm, err := domainFromLibvirt(defined)
	_ = defined.Free()
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	committed = true
	return vm, nil
}

func rewriteDomainDiskSource(xmlText, source, target string) (string, error) {
	if source == "" || target == "" || source == target {
		return "", domain.ErrInvalidInput
	}
	escapedSource := escapeXML(source)
	escapedTarget := escapeXML(target)
	patterns := []string{
		"source file='" + escapedSource + "'",
		`source file="` + escapedSource + `"`,
	}
	for _, pattern := range patterns {
		if strings.Count(xmlText, pattern) != 1 {
			continue
		}
		replacement := strings.Replace(pattern, escapedSource, escapedTarget, 1)
		return strings.Replace(xmlText, pattern, replacement, 1), nil
	}
	return "", fmt.Errorf("%w: domain disk source could not be rewritten unambiguously", domain.ErrInvalidInput)
}
