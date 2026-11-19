package libvirtbackend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func recoveryTestConnection(t *testing.T) *libvirt.Connect {
	t.Helper()
	conn, err := libvirt.NewConnect("test:///default")
	if err != nil {
		t.Skipf("libvirt test driver unavailable: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Close() })
	return conn
}

func createRecoveryTestVolume(t *testing.T, conn *libvirt.Connect, name string) (string, string) {
	t.Helper()
	pool, err := conn.LookupStoragePoolByName("default-pool")
	if err != nil {
		t.Skipf("libvirt test storage pool unavailable: %v", err)
	}
	defer func() { _ = pool.Free() }()
	volume, err := pool.StorageVolCreateXML(buildCloneVolumeXML(name, 1<<20, "qcow2"), 0)
	if err != nil {
		t.Fatalf("create test volume: %v", err)
	}
	defer func() { _ = volume.Free() }()
	path, err := volume.GetPath()
	if err != nil {
		t.Fatalf("get test volume path: %v", err)
	}
	return "default-pool", path
}

func TestRecoverCloneOperationCompletesWhenTargetDomainExists(t *testing.T) {
	conn := recoveryTestConnection(t)
	result, err := recoverCloneOperation(context.Background(), conn, domain.DataPlaneOperation{
		Kind: domain.DataPlaneOperationClone, VMName: "test",
	})
	if err != nil {
		t.Fatalf("recoverCloneOperation() error = %v", err)
	}
	if result.Status != domain.DataPlaneRecoveryCompleted {
		t.Fatalf("recoverCloneOperation() = %+v", result)
	}
}

func TestRecoverUnreferencedTargetVolumeIsDeleted(t *testing.T) {
	conn := recoveryTestConnection(t)
	poolName, path := createRecoveryTestVolume(t, conn, "recovery-unused.qcow2")
	result, err := recoverReferencedTarget(context.Background(), conn, domain.DataPlaneOperation{
		TargetPool: poolName, TargetName: "recovery-unused.qcow2",
	}, "", false)
	if err != nil {
		t.Fatalf("recoverReferencedTarget() error = %v", err)
	}
	if result.Status != domain.DataPlaneRecoveryRolledBack || !strings.Contains(result.Message, "已回收") {
		t.Fatalf("recoverReferencedTarget() = %+v", result)
	}
	volume, lookupErr := conn.LookupStorageVolByPath(path)
	if lookupErr == nil {
		_ = volume.Free()
		t.Fatalf("unreferenced target volume %s still exists", path)
	}
	if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
		t.Fatalf("lookup deleted volume error = %v", lookupErr)
	}
}

func TestRecoverReferencedTargetVolumeNeedsAttention(t *testing.T) {
	conn := recoveryTestConnection(t)
	poolName, path := createRecoveryTestVolume(t, conn, "recovery-referenced.qcow2")
	domainXML := `<domain type='test'>
  <name>recovery-ref-domain</name>
  <uuid>11111111-1111-4111-8111-111111111111</uuid>
  <memory unit='KiB'>1048576</memory>
  <vcpu>1</vcpu>
  <os><type arch='x86_64'>hvm</type></os>
  <devices><disk type='file' device='disk'><source file='` + path + `'/><target dev='vda' bus='virtio'/></disk></devices>
</domain>`
	dom, err := conn.DomainDefineXML(domainXML)
	if err != nil {
		t.Fatalf("define referencing domain: %v", err)
	}
	defer func() {
		_ = dom.Undefine()
		_ = dom.Free()
	}()

	result, err := recoverReferencedTarget(context.Background(), conn, domain.DataPlaneOperation{
		TargetPool: poolName, TargetName: "recovery-referenced.qcow2",
	}, "", false)
	if err != nil {
		t.Fatalf("recoverReferencedTarget() error = %v", err)
	}
	if result.Status != domain.DataPlaneRecoveryNeedsAttention || !strings.Contains(result.Message, "recovery-ref-domain") {
		t.Fatalf("recoverReferencedTarget() = %+v", result)
	}
	volume, lookupErr := conn.LookupStorageVolByPath(path)
	if lookupErr != nil {
		t.Fatalf("referenced target volume was removed: %v", lookupErr)
	}
	_ = volume.Free()
}

func TestBackupRestoreRecoveryTargetsFollowManifestOrder(t *testing.T) {
	targets, err := backupRestoreRecoveryTargets(domain.DataPlaneOperation{
		VMName: "restored-vm", TargetPool: "fast",
	}, backupManifest{Disks: []domain.VMBackupDisk{
		{BackupVolume: "backup-disk-0.qcow2", Format: "qcow2"},
		{BackupVolume: "backup-disk-1.raw", Format: "raw"},
	}})
	if err != nil {
		t.Fatalf("backupRestoreRecoveryTargets() error = %v", err)
	}
	if len(targets) != 2 || targets[0] != (recoveryTargetVolume{Pool: "fast", Name: "restored-vm-1.qcow2"}) ||
		targets[1] != (recoveryTargetVolume{Pool: "fast", Name: "restored-vm-2.raw"}) {
		t.Fatalf("backupRestoreRecoveryTargets() = %+v", targets)
	}
}
