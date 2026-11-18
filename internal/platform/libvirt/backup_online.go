package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type onlineBackupSnapshot struct {
	snapshot      *libvirt.DomainSnapshot
	name          string
	disks         []onlineBackupDisk
	joinedTargets map[string]bool
	joined        bool
}

type onlineBackupDisk struct {
	target     string
	source     string
	overlay    string
	sourcePool string
}

func createOnlineBackupSnapshot(ctx context.Context, conn *libvirt.Connect, dom *libvirt.Domain, backupID string, spec domain.VMBackupSpec, disks []cloneDisk, sourceVolumes []*libvirt.StorageVol) (*onlineBackupSnapshot, error) {
	if len(disks) == 0 || len(disks) != len(sourceVolumes) {
		return nil, fmt.Errorf("%w: online backup requires at least one writable disk", domain.ErrInvalidInput)
	}
	name := onlineBackupSnapshotName(backupID)
	if existing, err := dom.SnapshotLookupByName(name, 0); err == nil {
		_ = existing.Free()
		return nil, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_DOMAIN_SNAPSHOT) {
		return nil, fmt.Errorf("check online backup snapshot %s: %w", name, err)
	}
	state := &onlineBackupSnapshot{name: name, joinedTargets: make(map[string]bool, len(disks))}
	cleanup := true
	defer func() {
		if cleanup {
			_ = state.deleteOverlays(ctx, conn)
		}
	}()
	entries := make([]string, 0, len(disks))
	for index, disk := range disks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := sourceVolumes[index].GetInfo()
		if err != nil {
			return nil, fmt.Errorf("read source disk %s info for online backup: %w", disk.target, err)
		}
		pool, err := sourceVolumes[index].LookupPoolByVolume()
		if err != nil {
			return nil, fmt.Errorf("lookup source disk pool for online backup: %w", err)
		}
		poolName, nameErr := pool.GetName()
		poolInfo, infoErr := pool.GetInfo()
		if nameErr != nil || infoErr != nil {
			_ = pool.Free()
			return nil, fmt.Errorf("inspect source disk pool for online backup: %w", errors.Join(nameErr, infoErr))
		}
		if poolInfo.State != libvirt.STORAGE_POOL_RUNNING {
			_ = pool.Free()
			return nil, domain.ErrUnavailable
		}
		definition, definitionErr := storagePoolDefinition(pool)
		if definitionErr != nil {
			_ = pool.Free()
			return nil, definitionErr
		}
		if definition.Type == "logical" || definition.Type == "iscsi" || definition.Type == "rbd" {
			_ = pool.Free()
			return nil, fmt.Errorf("%w: online backup requires a file-backed source storage pool", domain.ErrInvalidInput)
		}
		overlayName := onlineBackupOverlayName(backupID, index)
		overlayVolume, err := pool.StorageVolCreateXML(buildOnlineOverlayVolumeXML(overlayName, info.Capacity, disk.source, cloneDiskFormatOrRaw(disk.format)), 0)
		_ = pool.Free()
		if err != nil {
			if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
				return nil, domain.ErrConflict
			}
			return nil, fmt.Errorf("create online backup overlay for %s: %w", disk.target, err)
		}
		overlayPath, err := overlayVolume.GetPath()
		_ = overlayVolume.Free()
		if err != nil {
			return nil, fmt.Errorf("read online backup overlay path for %s: %w", disk.target, err)
		}
		state.disks = append(state.disks, onlineBackupDisk{target: disk.target, source: disk.source, overlay: overlayName, sourcePool: poolName})
		entries = append(entries, fmt.Sprintf("<disk name='%s' snapshot='external'><source file='%s'/></disk>", escapeXML(disk.target), escapeXML(overlayPath)))
	}
	flags := libvirt.DOMAIN_SNAPSHOT_CREATE_DISK_ONLY | libvirt.DOMAIN_SNAPSHOT_CREATE_ATOMIC | libvirt.DOMAIN_SNAPSHOT_CREATE_REUSE_EXT
	if spec.Quiesce {
		if err := ensureGuestAgentQuiesce(dom); err != nil {
			return nil, err
		}
		flags |= libvirt.DOMAIN_SNAPSHOT_CREATE_QUIESCE
	}
	snapshotXML := fmt.Sprintf("<domainsnapshot><name>%s</name><description>VirtMgr online backup %s</description><disks>%s</disks></domainsnapshot>",
		escapeXML(name), escapeXML(backupID), strings.Join(entries, ""))
	snapshot, err := dom.CreateSnapshotXML(snapshotXML, flags)
	if err != nil {
		return nil, fmt.Errorf("create external disk snapshot for online backup: %w", err)
	}
	state.snapshot = snapshot
	cleanup = false
	return state, nil
}

func (s *onlineBackupSnapshot) join(ctx context.Context, conn *libvirt.Connect, dom *libvirt.Domain) error {
	if s == nil || s.joined {
		return nil
	}
	for _, disk := range s.disks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.joinedTargets[disk.target] {
			continue
		}
		if err := dom.BlockCommit(disk.target, "", "", 0, libvirt.DOMAIN_BLOCK_COMMIT_ACTIVE|libvirt.DOMAIN_BLOCK_COMMIT_BANDWIDTH_BYTES); err != nil {
			return fmt.Errorf("join online backup overlay for disk %s: %w", disk.target, err)
		}
		if err := waitForBlockCommit(ctx, dom, disk.target); err != nil {
			return err
		}
		if err := dom.BlockJobAbort(disk.target, libvirt.DOMAIN_BLOCK_JOB_ABORT_PIVOT); err != nil {
			return fmt.Errorf("pivot online backup overlay for disk %s: %w", disk.target, err)
		}
		if err := verifyActiveDiskSource(dom, disk.target, disk.source); err != nil {
			return err
		}
		s.joinedTargets[disk.target] = true
	}
	if s.snapshot != nil {
		if err := s.snapshot.Delete(libvirt.DOMAIN_SNAPSHOT_DELETE_METADATA_ONLY); err != nil && !errors.Is(err, libvirt.ERR_NO_DOMAIN_SNAPSHOT) {
			return fmt.Errorf("delete online backup snapshot metadata: %w", err)
		}
		_ = s.snapshot.Free()
		s.snapshot = nil
	}
	if err := s.deleteOverlays(ctx, conn); err != nil {
		return err
	}
	s.joined = true
	return nil
}

func (s *onlineBackupSnapshot) deleteOverlays(ctx context.Context, conn *libvirt.Connect) error {
	if s == nil {
		return nil
	}
	var result error
	for _, disk := range s.disks {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		pool, err := conn.LookupStoragePoolByName(disk.sourcePool)
		if err != nil {
			if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
				continue
			}
			result = errors.Join(result, fmt.Errorf("lookup overlay pool %s: %w", disk.sourcePool, err))
			continue
		}
		volume, lookupErr := pool.LookupStorageVolByName(disk.overlay)
		if lookupErr == nil {
			if deleteErr := volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL); deleteErr != nil && !errors.Is(deleteErr, libvirt.ERR_NO_STORAGE_VOL) {
				result = errors.Join(result, fmt.Errorf("delete overlay volume %s: %w", disk.overlay, deleteErr))
			}
			_ = volume.Free()
		} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
			result = errors.Join(result, fmt.Errorf("lookup overlay volume %s: %w", disk.overlay, lookupErr))
		}
		_ = pool.Free()
	}
	return result
}

func waitForBlockCommit(ctx context.Context, dom *libvirt.Domain, target string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := dom.GetBlockJobInfo(target, libvirt.DOMAIN_BLOCK_JOB_INFO_BANDWIDTH_BYTES)
		if err != nil {
			var libvirtErr libvirt.Error
			if errors.As(err, &libvirtErr) && libvirtErr.Code == libvirt.ERR_OPERATION_INVALID {
				return nil
			}
			return fmt.Errorf("read block commit progress for disk %s: %w", target, err)
		}
		if info == nil || info.Type == libvirt.DOMAIN_BLOCK_JOB_TYPE_UNKNOWN {
			return nil
		}
		if info.End > 0 && info.Cur >= info.End {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func verifyActiveDiskSource(dom *libvirt.Domain, target, expected string) error {
	xmlText, err := dom.GetXMLDesc(0)
	if err != nil {
		return fmt.Errorf("read live domain XML after pivoting disk %s: %w", target, err)
	}
	disks, err := cloneDisksFromDomainXML(xmlText)
	if err != nil {
		return fmt.Errorf("parse live domain XML after pivoting disk %s: %w", target, err)
	}
	for _, disk := range disks {
		if disk.target != target {
			continue
		}
		if filepath.Clean(disk.source) != filepath.Clean(expected) {
			return fmt.Errorf("disk %s did not pivot back to its base image", target)
		}
		return nil
	}
	return fmt.Errorf("disk %s is missing after online backup pivot", target)
}

func buildOnlineOverlayVolumeXML(name string, capacity uint64, backingPath, backingFormat string) string {
	return fmt.Sprintf("<volume type='file'><name>%s</name><capacity unit='bytes'>%d</capacity><target><format type='qcow2'/></target><backingStore><path>%s</path><format type='%s'/></backingStore></volume>",
		escapeXML(name), capacity, escapeXML(backingPath), escapeXML(backingFormat))
}

func onlineBackupSnapshotName(backupID string) string {
	name := "virtmgr-backup-" + strings.TrimSpace(backupID)
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func onlineBackupOverlayName(backupID string, index int) string {
	return fmt.Sprintf("virtmgr-backup-%s-%02d-overlay.qcow2", strings.TrimSpace(backupID), index)
}

func cloneDiskFormatOrRaw(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "qcow2" || format == "raw" {
		return format
	}
	return "raw"
}

func domainXMLContainsTPM(xmlText string) bool {
	var parsed domainXML
	return xml.Unmarshal([]byte(xmlText), &parsed) == nil && len(parsed.Devices.TPM) > 0
}
