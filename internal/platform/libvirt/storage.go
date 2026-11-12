package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type storagePoolXML struct {
	Type   string `xml:"type,attr"`
	Target struct {
		Path string `xml:"path"`
	} `xml:"target"`
}

type storagePoolDefinitionXML struct {
	XMLName xml.Name `xml:"pool"`
	Type    string   `xml:"type,attr"`
	Name    string   `xml:"name"`
	Target  struct {
		Path string `xml:"path"`
	} `xml:"target"`
}

type storageVolumeXML struct {
	Type   string `xml:"type,attr"`
	Target struct {
		Format struct {
			Type string `xml:"type,attr"`
		} `xml:"format"`
	} `xml:"target"`
}

func (c *Client) ListStoragePools(ctx context.Context) ([]domain.StoragePool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	pools, err := c.conn.ListAllStoragePools(libvirt.CONNECT_LIST_STORAGE_POOLS_ACTIVE | libvirt.CONNECT_LIST_STORAGE_POOLS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list libvirt storage pools: %w", err)
	}
	result := make([]domain.StoragePool, 0, len(pools))
	for i := range pools {
		pool := &pools[i]
		item, err := storagePoolFromLibvirt(pool)
		_ = pool.Free()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (c *Client) GetStoragePool(ctx context.Context, name string) (domain.StoragePool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.StoragePool{}, err
	}
	pool, err := c.conn.LookupStoragePoolByName(name)
	if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
		return domain.StoragePool{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.StoragePool{}, fmt.Errorf("lookup storage pool %s: %w", name, err)
	}
	defer func() { _ = pool.Free() }()
	return storagePoolFromLibvirt(pool)
}

func (c *Client) CreateStoragePool(ctx context.Context, spec domain.StoragePoolCreateSpec) (domain.StoragePool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.StoragePool{}, err
	}
	if existing, err := c.conn.LookupStoragePoolByName(spec.Name); err == nil {
		_ = existing.Free()
		return domain.StoragePool{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
		return domain.StoragePool{}, fmt.Errorf("lookup storage pool before create: %w", err)
	}
	pool, err := c.conn.StoragePoolDefineXML(buildStoragePoolXML(spec), 0)
	if err != nil {
		return domain.StoragePool{}, fmt.Errorf("define storage pool %s: %w", spec.Name, err)
	}
	defer func() { _ = pool.Free() }()
	started := false
	rollback := func() {
		if started {
			_ = pool.Destroy()
		}
		_ = pool.Undefine()
	}
	if spec.Autostart {
		if err := pool.SetAutostart(true); err != nil {
			rollback()
			return domain.StoragePool{}, fmt.Errorf("set storage pool %s autostart: %w", spec.Name, err)
		}
	}
	if spec.Start {
		if err := pool.Create(0); err != nil {
			rollback()
			return domain.StoragePool{}, fmt.Errorf("start storage pool %s: %w", spec.Name, err)
		}
		started = true
	}
	result, err := storagePoolFromLibvirt(pool)
	if err != nil {
		rollback()
		return domain.StoragePool{}, err
	}
	return result, nil
}

func (c *Client) PerformStoragePoolAction(ctx context.Context, name string, action domain.StoragePoolAction) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	pool, err := c.conn.LookupStoragePoolByName(name)
	if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup storage pool %s: %w", name, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return fmt.Errorf("read storage pool %s info: %w", name, err)
	}
	switch action {
	case domain.StoragePoolActionStart:
		if info.State != libvirt.STORAGE_POOL_INACTIVE {
			return domain.ErrConflict
		}
		if err := pool.Create(0); err != nil {
			return fmt.Errorf("start storage pool %s: %w", name, err)
		}
	case domain.StoragePoolActionStop:
		if info.State != libvirt.STORAGE_POOL_RUNNING {
			return domain.ErrConflict
		}
		if err := pool.Destroy(); err != nil {
			return fmt.Errorf("stop storage pool %s: %w", name, err)
		}
	case domain.StoragePoolActionRefresh:
		if info.State != libvirt.STORAGE_POOL_RUNNING {
			return domain.ErrConflict
		}
		if err := pool.Refresh(0); err != nil {
			return fmt.Errorf("refresh storage pool %s: %w", name, err)
		}
	default:
		return domain.ErrInvalidInput
	}
	return nil
}

func (c *Client) DeleteStoragePool(ctx context.Context, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	pool, err := c.conn.LookupStoragePoolByName(name)
	if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lookup storage pool %s: %w", name, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return fmt.Errorf("read storage pool %s info: %w", name, err)
	}
	if info.State != libvirt.STORAGE_POOL_INACTIVE {
		return domain.ErrConflict
	}
	if err := pool.Undefine(); err != nil {
		return fmt.Errorf("delete storage pool %s: %w", name, err)
	}
	return nil
}

func (c *Client) ListStorageVolumes(ctx context.Context, poolName string) ([]domain.StorageVolume, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return nil, err
	}
	pool, err := c.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("lookup storage pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return nil, fmt.Errorf("read storage pool %s info: %w", poolName, err)
	}
	if info.State == libvirt.STORAGE_POOL_INACTIVE {
		return nil, domain.ErrUnavailable
	}
	volumes, err := pool.ListAllStorageVolumes(0)
	if err != nil {
		return nil, fmt.Errorf("list storage volumes in %s: %w", poolName, err)
	}
	result := make([]domain.StorageVolume, 0, len(volumes))
	for i := range volumes {
		volume := &volumes[i]
		item, err := storageVolumeFromLibvirt(volume)
		_ = volume.Free()
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (c *Client) CreateStorageVolume(ctx context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.StorageVolume{}, err
	}
	pool, err := c.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup storage pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read storage pool %s info: %w", poolName, err)
	}
	if info.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	if existing, err := pool.LookupStorageVolByName(spec.Name); err == nil {
		_ = existing.Free()
		return domain.StorageVolume{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
		return domain.StorageVolume{}, fmt.Errorf("check storage volume %s: %w", spec.Name, err)
	}
	volume, err := pool.StorageVolCreateXML(buildStorageVolumeXML(spec), 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return domain.StorageVolume{}, domain.ErrConflict
		}
		return domain.StorageVolume{}, fmt.Errorf("create storage volume %s in %s: %w", spec.Name, poolName, err)
	}
	defer func() { _ = volume.Free() }()
	return storageVolumeFromLibvirt(volume)
}

func (c *Client) UploadStorageVolume(ctx context.Context, poolName string, spec domain.StorageVolumeCreateSpec, reader io.Reader) (domain.StorageVolume, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.StorageVolume{}, err
	}
	pool, err := c.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.StorageVolume{}, domain.ErrNotFound
		}
		return domain.StorageVolume{}, fmt.Errorf("lookup storage pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read storage pool %s info: %w", poolName, err)
	}
	if info.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.StorageVolume{}, domain.ErrUnavailable
	}
	if existing, err := pool.LookupStorageVolByName(spec.Name); err == nil {
		_ = existing.Free()
		return domain.StorageVolume{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
		return domain.StorageVolume{}, fmt.Errorf("check storage volume %s: %w", spec.Name, err)
	}
	volume, err := pool.StorageVolCreateXML(buildStorageVolumeXML(spec), 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return domain.StorageVolume{}, domain.ErrConflict
		}
		return domain.StorageVolume{}, fmt.Errorf("create storage volume %s in %s: %w", spec.Name, poolName, err)
	}
	defer func() { _ = volume.Free() }()
	committed := false
	defer func() {
		if !committed {
			_ = volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		}
	}()
	stream, err := c.conn.NewStream(0)
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("open libvirt upload stream for %s: %w", spec.Name, err)
	}
	defer func() { _ = stream.Free() }()
	if err := volume.Upload(stream, 0, spec.CapacityBytes, 0); err != nil {
		_ = stream.Abort()
		return domain.StorageVolume{}, fmt.Errorf("start upload for storage volume %s: %w", spec.Name, err)
	}
	if err := copyToStorageStream(stream, reader, spec.CapacityBytes); err != nil {
		_ = stream.Abort()
		return domain.StorageVolume{}, err
	}
	if err := stream.Finish(); err != nil {
		return domain.StorageVolume{}, fmt.Errorf("finish upload for storage volume %s: %w", spec.Name, err)
	}
	committed = true
	if err := pool.Refresh(0); err == nil {
		if uploaded, lookupErr := pool.LookupStorageVolByName(spec.Name); lookupErr == nil {
			defer func() { _ = uploaded.Free() }()
			if result, infoErr := storageVolumeFromLibvirt(uploaded); infoErr == nil {
				return result, nil
			}
		}
	}
	return storageVolumeFromLibvirt(volume)
}

type storageStreamSender interface {
	Send([]byte) (int, error)
}

type storageStreamWriter struct {
	sender storageStreamSender
}

func (w storageStreamWriter) Write(body []byte) (int, error) {
	n, err := w.sender.Send(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	return n, err
}

func copyToStorageStream(sender storageStreamSender, reader io.Reader, size uint64) error {
	if reader == nil || size == 0 {
		return domain.ErrInvalidInput
	}
	written, err := io.CopyN(storageStreamWriter{sender: sender}, reader, int64(size))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: upload ended before declared size", domain.ErrInvalidInput)
		}
		return fmt.Errorf("stream storage upload: %w", err)
	}
	if uint64(written) != size {
		return fmt.Errorf("%w: uploaded size did not match declared size", domain.ErrInvalidInput)
	}
	return nil
}

func (c *Client) DeleteStorageVolume(ctx context.Context, poolName, volumeName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	pool, err := c.conn.LookupStoragePoolByName(poolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("lookup storage pool %s: %w", poolName, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return fmt.Errorf("read storage pool %s info: %w", poolName, err)
	}
	if info.State != libvirt.STORAGE_POOL_RUNNING {
		return domain.ErrUnavailable
	}
	volume, err := pool.LookupStorageVolByName(volumeName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("lookup storage volume %s in %s: %w", volumeName, poolName, err)
	}
	defer func() { _ = volume.Free() }()
	volumePath, err := volume.GetPath()
	if err != nil {
		return fmt.Errorf("read storage volume %s path: %w", volumeName, err)
	}
	attachedTo, err := c.domainNamesUsingVolumeLocked(volumePath, volumeName)
	if err != nil {
		return err
	}
	if len(attachedTo) > 0 {
		return fmt.Errorf("%w: attached to %s", domain.ErrVolumeInUse, strings.Join(attachedTo, ", "))
	}
	if err := volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL); err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("delete storage volume %s in %s: %w", volumeName, poolName, err)
	}
	return nil
}

func (c *Client) domainNamesUsingVolumeLocked(volumePath, volumeName string) ([]string, error) {
	domains, err := c.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return nil, fmt.Errorf("list libvirt domains before volume delete: %w", err)
	}
	attachedTo := make([]string, 0)
	for i := range domains {
		dom := &domains[i]
		name, nameErr := dom.GetName()
		xmlText, xmlErr := dom.GetXMLDesc(0)
		_ = dom.Free()
		if nameErr != nil || xmlErr != nil {
			return nil, fmt.Errorf("inspect domains before volume delete: %w", errors.Join(nameErr, xmlErr))
		}
		details, err := parseDomainXML(xmlText)
		if err != nil {
			return nil, fmt.Errorf("inspect domain %s before volume delete: %w", name, err)
		}
		for _, disk := range details.Disks {
			if diskUsesVolume(disk.Source, volumePath, volumeName) {
				attachedTo = append(attachedTo, name)
				break
			}
		}
	}
	return attachedTo, nil
}

func diskUsesVolume(source, volumePath, volumeName string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	if source == volumeName {
		return true
	}
	if volumePath == "" {
		return false
	}
	return filepath.Clean(source) == filepath.Clean(volumePath)
}

func buildStorageVolumeXML(spec domain.StorageVolumeCreateSpec) string {
	var builder strings.Builder
	builder.WriteString("<volume type='file'>")
	fmt.Fprintf(&builder, "<name>%s</name>", escapeXML(spec.Name))
	fmt.Fprintf(&builder, "<capacity unit='bytes'>%d</capacity>", spec.CapacityBytes)
	builder.WriteString("<allocation unit='bytes'>0</allocation>")
	fmt.Fprintf(&builder, "<target><format type='%s'/></target>", escapeXML(spec.Format))
	builder.WriteString("</volume>")
	return builder.String()
}

func buildStoragePoolXML(spec domain.StoragePoolCreateSpec) string {
	var builder strings.Builder
	builder.WriteString("<pool type='dir'>")
	fmt.Fprintf(&builder, "<name>%s</name>", escapeXML(spec.Name))
	fmt.Fprintf(&builder, "<target><path>%s</path></target>", escapeXML(spec.TargetPath))
	builder.WriteString("</pool>")
	return builder.String()
}

func storagePoolFromLibvirt(pool *libvirt.StoragePool) (domain.StoragePool, error) {
	name, err := pool.GetName()
	if err != nil {
		return domain.StoragePool{}, fmt.Errorf("read storage pool name: %w", err)
	}
	uuid, _ := pool.GetUUIDString()
	info, err := pool.GetInfo()
	if err != nil {
		return domain.StoragePool{}, fmt.Errorf("read storage pool %s info: %w", name, err)
	}
	autostart, _ := pool.GetAutostart()
	persistent, _ := pool.IsPersistent()
	xmlText, err := pool.GetXMLDesc(0)
	if err != nil {
		return domain.StoragePool{}, fmt.Errorf("read storage pool %s xml: %w", name, err)
	}
	var parsed storagePoolXML
	if err := xml.Unmarshal([]byte(xmlText), &parsed); err != nil {
		return domain.StoragePool{}, fmt.Errorf("decode storage pool %s xml: %w", name, err)
	}
	volumeCount := 0
	if info.State != libvirt.STORAGE_POOL_INACTIVE {
		if volumes, err := pool.ListAllStorageVolumes(0); err == nil {
			volumeCount = len(volumes)
			for i := range volumes {
				_ = volumes[i].Free()
			}
		}
	}
	return domain.StoragePool{
		Name: name, UUID: uuid, Type: parsed.Type, State: mapStoragePoolState(info.State),
		Autostart: autostart, Persistent: persistent, TargetPath: parsed.Target.Path,
		CapacityBytes: info.Capacity, AllocationBytes: info.Allocation, AvailableBytes: info.Available,
		VolumeCount: volumeCount,
	}, nil
}

func storageVolumeFromLibvirt(volume *libvirt.StorageVol) (domain.StorageVolume, error) {
	name, err := volume.GetName()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read storage volume name: %w", err)
	}
	path, _ := volume.GetPath()
	info, err := volume.GetInfo()
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read storage volume %s info: %w", name, err)
	}
	xmlText, err := volume.GetXMLDesc(0)
	if err != nil {
		return domain.StorageVolume{}, fmt.Errorf("read storage volume %s xml: %w", name, err)
	}
	var parsed storageVolumeXML
	if err := xml.Unmarshal([]byte(xmlText), &parsed); err != nil {
		return domain.StorageVolume{}, fmt.Errorf("decode storage volume %s xml: %w", name, err)
	}
	return domain.StorageVolume{
		Name: name, Path: path, Type: mapStorageVolumeType(info.Type), Format: parsed.Target.Format.Type,
		CapacityBytes: info.Capacity, AllocationBytes: info.Allocation,
	}, nil
}

func mapStoragePoolState(state libvirt.StoragePoolState) domain.StoragePoolState {
	switch state {
	case libvirt.STORAGE_POOL_RUNNING:
		return domain.StoragePoolStateActive
	case libvirt.STORAGE_POOL_INACTIVE, libvirt.STORAGE_POOL_BUILDING:
		return domain.StoragePoolStateInactive
	case libvirt.STORAGE_POOL_DEGRADED:
		return domain.StoragePoolStateDegraded
	case libvirt.STORAGE_POOL_INACCESSIBLE:
		return domain.StoragePoolStateInaccessible
	default:
		return domain.StoragePoolStateUnknown
	}
}

func mapStorageVolumeType(volumeType libvirt.StorageVolType) string {
	switch volumeType {
	case libvirt.STORAGE_VOL_FILE:
		return "file"
	case libvirt.STORAGE_VOL_BLOCK:
		return "block"
	case libvirt.STORAGE_VOL_DIR:
		return "dir"
	case libvirt.STORAGE_VOL_NETWORK, libvirt.STORAGE_VOL_NETDIR:
		return "network"
	case libvirt.STORAGE_VOL_PLOOP:
		return "ploop"
	default:
		return "unknown"
	}
}
