package libvirtbackend

import (
	"context"
	"fmt"
	"io"
	"sync"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type Backend interface {
	Host(ctx context.Context) (domain.Host, error)
	HostMetrics(ctx context.Context) (domain.MetricsCounters, error)
	ListHostDevices(ctx context.Context) ([]domain.HostDevice, error)
	ListVMs(ctx context.Context) ([]domain.VM, error)
	GetVM(ctx context.Context, uuid string) (domain.VM, error)
	VMMetrics(ctx context.Context, uuid string) (domain.MetricsCounters, error)
	GetVMXML(ctx context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error)
	DefineVMXML(ctx context.Context, uuid, xmlText string) (domain.VMXML, error)
	UpdateVMResources(ctx context.Context, uuid string, spec domain.VMResourcesSpec) (domain.VM, error)
	ResizeVMDisk(ctx context.Context, uuid, target string, spec domain.VMDiskResizeSpec) (domain.VM, error)
	SetVMAutostart(ctx context.Context, uuid string, enabled bool) error
	AttachHostDevice(ctx context.Context, uuid, deviceName string) (domain.VM, error)
	DetachHostDevice(ctx context.Context, uuid, deviceName string) (domain.VM, error)
	CreateVM(ctx context.Context, spec domain.VMCreateSpec) (domain.VM, error)
	CloneVM(ctx context.Context, uuid string, spec domain.VMCloneSpec, progress VMCloneProgress) (domain.VM, error)
	BackupVM(ctx context.Context, uuid string, spec domain.VMBackupSpec, progress VMBackupProgress) (domain.VMBackup, error)
	VerifyVMBackup(ctx context.Context, backup domain.Backup, deep bool) (domain.BackupVerification, error)
	RestoreVMBackup(ctx context.Context, backup domain.Backup, spec domain.VMRestoreSpec, progress VMBackupProgress) (domain.VM, error)
	DeleteVMBackup(ctx context.Context, backup domain.Backup) error
	DeleteVM(ctx context.Context, uuid string) error
	PerformAction(ctx context.Context, uuid string, action domain.VMAction) error
	ListSnapshots(ctx context.Context, uuid string) ([]domain.VMSnapshot, error)
	CreateSnapshot(ctx context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error)
	RevertSnapshot(ctx context.Context, uuid, name string) error
	DeleteSnapshot(ctx context.Context, uuid, name string) error
	OpenConsole(ctx context.Context, uuid string, graphicsIndex uint) (io.ReadWriteCloser, error)
	OpenSerialConsole(ctx context.Context, uuid, device string) (io.ReadWriteCloser, error)
	SendConsoleKey(ctx context.Context, uuid string, keycodes []uint) error
	ListStoragePools(ctx context.Context) ([]domain.StoragePool, error)
	GetStoragePool(ctx context.Context, name string) (domain.StoragePool, error)
	CreateStoragePool(ctx context.Context, spec domain.StoragePoolCreateSpec) (domain.StoragePool, error)
	PerformStoragePoolAction(ctx context.Context, name string, action domain.StoragePoolAction) error
	DeleteStoragePool(ctx context.Context, name string) error
	ListStorageVolumes(ctx context.Context, poolName string) ([]domain.StorageVolume, error)
	CreateStorageVolume(ctx context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error)
	UploadStorageVolume(ctx context.Context, poolName string, spec domain.StorageVolumeCreateSpec, reader io.Reader) (domain.StorageVolume, error)
	DeleteStorageVolume(ctx context.Context, poolName, volumeName string) error
	ListNetworks(ctx context.Context) ([]domain.VirtualNetwork, error)
	ListDHCPLeases(ctx context.Context, networkName string) ([]domain.DHCPLease, error)
	CreateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error)
	UpdateNetwork(ctx context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error)
	AddNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error)
	DeleteNetworkDHCPHost(ctx context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error)
	PerformNetworkAction(ctx context.Context, name string, action domain.VirtualNetworkAction) error
	DeleteNetwork(ctx context.Context, name string) error
	Ping(ctx context.Context) error
	Close() error
}

type VMCloneProgress = func(percent int, step string)
type VMBackupProgress = func(percent int, step string)

type Client struct {
	uri             string
	hostName        string
	events          EventSink
	mu              sync.Mutex
	conn            *libvirt.Connect
	eventCallbackID int
	guestMu         sync.Mutex
	guestConn       *libvirt.Connect
}

type EventSink interface {
	Publish(event domain.SystemEvent)
}

func New(uri, hostName string, sinks ...EventSink) (*Client, error) {
	client := &Client{uri: uri, hostName: hostName}
	if len(sinks) > 0 {
		client.events = sinks[0]
		startEventLoop()
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if err := client.connectLocked(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *Client) connectLocked() error {
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return fmt.Errorf("connect libvirt %s: %w", c.uri, err)
	}
	c.conn = conn
	if c.events != nil {
		callbackID, registerErr := conn.DomainEventLifecycleRegister(nil, c.handleDomainLifecycle)
		if registerErr == nil {
			c.eventCallbackID = callbackID
		}
	}
	return nil
}

func (c *Client) ensureConnectionLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.conn != nil {
		alive, err := c.conn.IsAlive()
		if err == nil && alive {
			return nil
		}
		c.closeConnectionLocked()
		c.conn = nil
	}
	return c.connectLocked()
}

func (c *Client) Ping(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	alive, err := c.conn.IsAlive()
	if err != nil {
		return fmt.Errorf("check libvirt connection: %w", err)
	}
	if !alive {
		return domain.ErrUnavailable
	}
	return nil
}

func (c *Client) Host(ctx context.Context) (domain.Host, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.Host{}, err
	}
	info, err := c.conn.GetNodeInfo()
	if err != nil {
		return domain.Host{}, fmt.Errorf("read libvirt node info: %w", err)
	}
	version, err := c.conn.GetLibVersion()
	if err != nil {
		return domain.Host{}, fmt.Errorf("read libvirt version: %w", err)
	}
	return domain.Host{
		Name:           c.hostName,
		LibvirtVersion: formatVersion(version),
		CPUModel:       info.Model,
		CPUs:           info.Cpus,
		CPUSockets:     info.Sockets,
		CPUCores:       info.Cores,
		CPUThreads:     info.Threads,
		CPUMHz:         info.MHz,
		MemoryBytes:    info.Memory * 1024,
	}, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	var err error
	if c.conn != nil {
		err = c.closeConnectionLocked()
		c.conn = nil
	}
	c.mu.Unlock()
	c.guestMu.Lock()
	if c.guestConn != nil {
		_, guestErr := c.guestConn.Close()
		c.guestConn = nil
		if err == nil {
			err = guestErr
		}
	}
	c.guestMu.Unlock()
	return err
}

func (c *Client) closeConnectionLocked() error {
	if c.eventCallbackID > 0 {
		_ = c.conn.DomainEventDeregister(c.eventCallbackID)
		c.eventCallbackID = 0
	}
	_, err := c.conn.Close()
	return err
}

func formatVersion(version uint32) string {
	return fmt.Sprintf("%d.%d.%d", version/1_000_000, (version/1_000)%1_000, version%1_000)
}
