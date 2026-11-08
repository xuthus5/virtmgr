package libvirtbackend

import (
	"context"
	"errors"
	"testing"

	"virtmgr/internal/domain"
)

func TestBackupVolumeNameAndExtension(t *testing.T) {
	if got := backupVolumeName("vm-backup", "-disk-01.qcow2"); got != "vm-backup-disk-01.qcow2" {
		t.Fatalf("backupVolumeName() = %q", got)
	}
	if got := backupExtension("/var/lib/libvirt/images/disk.QCOW2", "qcow2"); got != ".qcow2" {
		t.Fatalf("backupExtension() = %q", got)
	}
	if got := backupExtension("disk", "raw"); got != ".raw" {
		t.Fatalf("backupExtension() fallback = %q", got)
	}
	if got := backupVolumeName(string(make([]byte, 127)), "-manifest.json"); len(got) != 127 {
		t.Fatalf("backupVolumeName() length = %d", len(got))
	}
}

func TestDemoBackupVMCreatesConfigManifestAndDiskVolumes(t *testing.T) {
	backend := NewDemo()
	before, err := backend.ListStorageVolumes(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	progress := make([]int, 0, 5)
	result, err := backend.BackupVM(context.Background(), "44444444-4444-4444-8444-444444444444", domain.VMBackupSpec{
		Name: "demo-backup-01", TargetPool: "default",
	}, func(percent int, _ string) { progress = append(progress, percent) })
	if err != nil {
		t.Fatalf("BackupVM() error = %v", err)
	}
	if result.FormatVersion != domain.VMBackupFormatVersion || result.VMName != "demo-legacy-01" || len(result.Disks) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.ConfigVolume != "demo-backup-01-domain.xml" || result.ManifestVolume != "demo-backup-01-manifest.json" {
		t.Fatalf("result volumes = %+v", result)
	}
	if result.Disks[0].BackupVolume != "demo-backup-01-disk-01.qcow2" || result.TotalBytes == 0 {
		t.Fatalf("result disks = %+v total=%d", result.Disks, result.TotalBytes)
	}
	if len(progress) == 0 || progress[len(progress)-1] != 100 {
		t.Fatalf("progress = %v", progress)
	}
	after, err := backend.ListStorageVolumes(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+3 {
		t.Fatalf("volumes before=%d after=%d", len(before), len(after))
	}
	for _, name := range []string{result.ConfigVolume, result.ManifestVolume, result.Disks[0].BackupVolume} {
		if !containsDemoVolume(after, name) {
			t.Fatalf("backup volume %s not found in %+v", name, after)
		}
	}
}

func TestDemoBackupVMRejectsUnsafeSourceAndConflicts(t *testing.T) {
	backend := NewDemo()
	if _, err := backend.BackupVM(context.Background(), "11111111-1111-4111-8111-111111111111", domain.VMBackupSpec{
		Name: "running-backup", TargetPool: "default",
	}, nil); !errors.Is(err, domain.ErrActionNotAllowed) {
		t.Fatalf("running source error = %v", err)
	}
	if _, err := backend.BackupVM(context.Background(), "44444444-4444-4444-8444-444444444444", domain.VMBackupSpec{
		Name: "inactive-backup", TargetPool: "archive",
	}, nil); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("inactive pool error = %v", err)
	}
	if _, err := backend.BackupVM(context.Background(), "44444444-4444-4444-8444-444444444444", domain.VMBackupSpec{
		Name: "demo-backup-01", TargetPool: "default",
	}, nil); err != nil {
		t.Fatalf("first backup error = %v", err)
	}
	if _, err := backend.BackupVM(context.Background(), "44444444-4444-4444-8444-444444444444", domain.VMBackupSpec{
		Name: "demo-backup-01", TargetPool: "default",
	}, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestDemoBackupVMRollsBackPartialVolumes(t *testing.T) {
	backend := NewDemo()
	const uuid = "99999999-9999-4999-8999-999999999999"
	backend.mu.Lock()
	backend.vms[uuid] = domain.VM{
		UUID: uuid, Name: "rollback-demo", State: domain.VMStateShutoff, Persistent: true,
		Disks: []domain.Disk{
			{Device: "disk", Source: "/var/lib/libvirt/images/rollback-01.qcow2", Format: "qcow2"},
			{Device: "disk", Source: "/var/lib/libvirt/images/rollback-02.qcow2", Format: "qcow2"},
		},
		AllowedActions: domain.AllowedVMActions(domain.VMStateShutoff),
	}
	backend.volumes["default"] = append(backend.volumes["default"],
		domain.StorageVolume{Name: "rollback-01.qcow2", Path: "/var/lib/libvirt/images/rollback-01.qcow2", Type: "file", Format: "qcow2", CapacityBytes: 1024},
		domain.StorageVolume{Name: "rollback-02.qcow2", Path: "/var/lib/libvirt/images/rollback-02.qcow2", Type: "file", Format: "qcow2", CapacityBytes: 1024},
	)
	before := len(backend.volumes["default"])
	backend.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	_, err := backend.BackupVM(ctx, uuid, domain.VMBackupSpec{Name: "rollback-backup", TargetPool: "default"}, func(percent int, _ string) {
		if percent >= 10 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BackupVM() error = %v", err)
	}
	backend.mu.RLock()
	after := len(backend.volumes["default"])
	backend.mu.RUnlock()
	if after != before {
		t.Fatalf("partial backup volumes were not removed: before=%d after=%d", before, after)
	}
}

func containsDemoVolume(volumes []domain.StorageVolume, name string) bool {
	for _, volume := range volumes {
		if volume.Name == name {
			return true
		}
	}
	return false
}
