package libvirtbackend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"virtmgr/internal/domain"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	libvirt "libvirt.org/go/libvirt"
)

const cloudInitSeedSize = 1 << 20

type cloudInitSeed struct {
	volume *libvirt.StorageVol
	name   string
	path   string
}

func (s *cloudInitSeed) free() {
	if s == nil || s.volume == nil {
		return
	}
	_ = s.volume.Free()
	s.volume = nil
}

func (s *cloudInitSeed) remove() {
	if s == nil || s.volume == nil {
		return
	}
	_ = s.volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
	s.free()
}

func (c *Client) createCloudInitSeedLocked(ctx context.Context, vmName string, spec domain.CloudInitSpec) (*cloudInitSeed, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	isoData, err := buildCloudInitISO(vmName, spec.UserData)
	if err != nil {
		return nil, err
	}
	pool, err := c.conn.LookupStoragePoolByName(spec.Pool)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("lookup cloud-init storage pool %s: %w", spec.Pool, err)
	}
	defer func() { _ = pool.Free() }()
	info, err := pool.GetInfo()
	if err != nil {
		return nil, fmt.Errorf("read cloud-init storage pool %s info: %w", spec.Pool, err)
	}
	if info.State != libvirt.STORAGE_POOL_RUNNING {
		return nil, domain.ErrUnavailable
	}
	name := cloudInitSeedVolumeName(vmName)
	if existing, lookupErr := pool.LookupStorageVolByName(name); lookupErr == nil {
		_ = existing.Free()
		return nil, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
		return nil, fmt.Errorf("check cloud-init volume %s: %w", name, lookupErr)
	}
	volumeSpec := domain.StorageVolumeCreateSpec{Name: name, Format: "raw", CapacityBytes: uint64(len(isoData))}
	volume, err := pool.StorageVolCreateXML(buildStorageVolumeXML(volumeSpec), 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return nil, domain.ErrConflict
		}
		return nil, fmt.Errorf("create cloud-init volume %s in %s: %w", name, spec.Pool, err)
	}
	seed := &cloudInitSeed{volume: volume, name: name}
	committed := false
	defer func() {
		if !committed {
			seed.remove()
		}
	}()
	stream, err := c.conn.NewStream(0)
	if err != nil {
		return nil, fmt.Errorf("open cloud-init upload stream for %s: %w", name, err)
	}
	defer func() { _ = stream.Free() }()
	if err := volume.Upload(stream, 0, uint64(len(isoData)), 0); err != nil {
		_ = stream.Abort()
		return nil, fmt.Errorf("start cloud-init upload for %s: %w", name, err)
	}
	if err := copyToStorageStream(stream, bytes.NewReader(isoData), uint64(len(isoData))); err != nil {
		_ = stream.Abort()
		return nil, err
	}
	if err := stream.Finish(); err != nil {
		return nil, fmt.Errorf("finish cloud-init upload for %s: %w", name, err)
	}
	path, err := volume.GetPath()
	if err != nil {
		return nil, fmt.Errorf("read cloud-init volume %s path: %w", name, err)
	}
	seed.path = path
	committed = true
	return seed, nil
}

func cloudInitSeedVolumeName(vmName string) string {
	name := strings.TrimSpace(vmName) + "-cidata.iso"
	if len(name) <= 127 {
		return name
	}
	return name[:127]
}

func buildCloudInitISO(vmName, userData string) ([]byte, error) {
	vmName = strings.TrimSpace(vmName)
	userData = strings.TrimSpace(userData)
	if vmName == "" || userData == "" {
		return nil, domain.ErrInvalidInput
	}
	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", vmName, vmName)
	tempDir, err := os.MkdirTemp("", "virtmgr-cloud-init-*")
	if err != nil {
		return nil, fmt.Errorf("create cloud-init ISO temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	path := filepath.Join(tempDir, "seed.iso")

	image, err := diskfs.Create(path, cloudInitSeedSize, diskfs.SectorSizeDefault)
	if err != nil {
		return nil, fmt.Errorf("create cloud-init ISO image: %w", err)
	}
	defer func() { _ = image.Close() }()
	image.LogicalBlocksize = 2048
	fs, err := image.CreateFilesystem(disk.FilesystemSpec{
		Partition: 0, FSType: filesystem.TypeISO9660, VolumeLabel: "cidata",
	})
	if err != nil {
		return nil, fmt.Errorf("create cloud-init ISO filesystem: %w", err)
	}
	defer func() { _ = fs.Close() }()
	for name, content := range map[string]string{"meta-data": metaData, "user-data": userData + "\n"} {
		file, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
		if err != nil {
			return nil, fmt.Errorf("open cloud-init %s in ISO: %w", name, err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("write cloud-init %s to ISO: %w", name, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close cloud-init %s in ISO: %w", name, err)
		}
	}
	iso, ok := fs.(*iso9660.FileSystem)
	if !ok {
		return nil, fmt.Errorf("cloud-init seed filesystem is not ISO9660")
	}
	if err := iso.Finalize(iso9660.FinalizeOptions{RockRidge: true, Joliet: true, VolumeIdentifier: "cidata"}); err != nil {
		return nil, fmt.Errorf("finalize cloud-init ISO: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cloud-init ISO: %w", err)
	}
	return data, nil
}
