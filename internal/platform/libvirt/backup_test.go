package libvirtbackend

import (
	"context"
	"errors"
	"strings"
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
	if result.FormatVersion != domain.VMBackupFormatVersionV2 || result.VMName != "demo-legacy-01" || len(result.Disks) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.ConfigVolume != "demo-backup-01-domain.xml" || result.ManifestVolume != "demo-backup-01-manifest.json" {
		t.Fatalf("result volumes = %+v", result)
	}
	if result.Disks[0].BackupVolume != "demo-backup-01-disk-00.qcow2" || result.TotalBytes == 0 {
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

func TestDemoOnlineBackupPreservesRunningVMAndMarksConsistency(t *testing.T) {
	backend := NewDemo()
	const uuid = "11111111-1111-4111-8111-111111111111"
	result, err := backend.BackupVM(context.Background(), uuid, domain.VMBackupSpec{
		Name: "demo-online-backup", TargetPool: "default", Online: true, Quiesce: true,
	}, nil)
	if err != nil {
		t.Fatalf("online BackupVM() error = %v", err)
	}
	if !result.Online || !result.Quiesced || !result.Consistent || result.TPMStateSaved {
		t.Fatalf("online backup metadata = %+v", result)
	}
	if _, err := backend.BackupVM(context.Background(), "44444444-4444-4444-8444-444444444444", domain.VMBackupSpec{
		Name: "invalid-quiesce", TargetPool: "default", Quiesce: true,
	}, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("offline quiesced backup error = %v", err)
	}
}

func TestOnlineBackupHelpers(t *testing.T) {
	xmlText := buildOnlineOverlayVolumeXML("overlay.qcow2", 10<<30, "/var/lib/libvirt/images/base.qcow2", "qcow2")
	for _, fragment := range []string{"<name>overlay.qcow2</name>", "<format type='qcow2'/>", "<path>/var/lib/libvirt/images/base.qcow2</path>", "unit='bytes'>10737418240</capacity>"} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("overlay XML missing %q: %s", fragment, xmlText)
		}
	}
	if got := onlineBackupSnapshotName(strings.Repeat("a", 100)); len(got) != 64 {
		t.Fatalf("snapshot name length = %d", len(got))
	}
	if got := onlineBackupOverlayName("backup-1", 3); got != "virtmgr-backup-backup-1-03-overlay.qcow2" {
		t.Fatalf("overlay name = %q", got)
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
