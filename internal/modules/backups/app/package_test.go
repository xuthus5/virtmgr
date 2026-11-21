package app

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"virtmgr/internal/domain"
	libvirtbackend "virtmgr/internal/platform/libvirt"
	"virtmgr/internal/platform/sqlite"
)

func TestBackupPackageExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "virtmgr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewStore(db)
	backend := libvirtbackend.NewDemo()
	if _, err := store.CreateBackupRepository(ctx, domain.BackupRepository{
		ID: "brepo_test", Name: "default archive", Type: "libvirt-pool", PoolName: "default", IsDefault: true,
	}); err != nil {
		t.Fatal(err)
	}
	vmUUID := "44444444-4444-4444-8444-444444444444"
	result, err := backend.BackupVM(ctx, vmUUID, domain.VMBackupSpec{Name: "nightly", TargetPool: "default"}, nil)
	if err != nil {
		t.Fatalf("BackupVM() error = %v", err)
	}
	now := time.Now().UTC()
	original, err := store.CreateBackup(ctx, domain.Backup{
		ID: "vmb_original", VMUUID: vmUUID, VMName: result.VMName, RepositoryID: "brepo_test",
		RepositoryName: "default archive", TargetPool: "default", Name: "nightly", Status: domain.BackupStatusCompleted,
		FormatVersion: result.FormatVersion, ManifestVolume: result.ManifestVolume, ConfigVolume: result.ConfigVolume,
		ConfigBytes: result.ConfigBytes, TotalBytes: result.TotalBytes, DiskCount: len(result.Disks), Disks: backupDisks(result.Disks),
		VerifyStatus: domain.BackupVerifyUnknown, CreatedBy: "admin", CreatedAt: now, CompletedAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, backend, store, nil, false)
	var packageBytes bytes.Buffer
	if err := service.Export(ctx, ExportCommand{ID: original.ID, Writer: &packageBytes, Request: backupMutationContext()}); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if packageBytes.Len() == 0 {
		t.Fatal("exported package is empty")
	}
	if _, err := zip.NewReader(bytes.NewReader(packageBytes.Bytes()), int64(packageBytes.Len())); err != nil {
		t.Fatalf("exported package is not a valid zip: %v", err)
	}
	imported, err := service.Import(ctx, ImportCommand{
		Spec: domain.BackupImportSpec{Name: "imported-nightly"}, Reader: bytes.NewReader(packageBytes.Bytes()), Request: backupMutationContext(),
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if imported.ID == original.ID || imported.Name != "imported-nightly" || imported.Status != domain.BackupStatusCompleted {
		t.Fatalf("imported backup = %+v", imported)
	}
	for _, disk := range imported.Disks {
		for _, originalDisk := range original.Disks {
			if disk.Artifact == originalDisk.Artifact {
				t.Fatalf("import reused artifact name %q", disk.Artifact)
			}
		}
	}
	manifest, err := store.GetBackup(ctx, imported.ID)
	if err != nil || manifest.DiskCount != len(original.Disks) || manifest.ConfigVolume == original.ConfigVolume {
		t.Fatalf("reloaded imported backup = %+v, %v", manifest, err)
	}
}

func TestBackupArchiveRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name   string
		build  func(*zip.Writer)
		target error
	}{
		{
			name: "path traversal",
			build: func(archive *zip.Writer) {
				writeTestZipEntry(t, archive, "../escape", "x", 0)
			},
			target: domain.ErrInvalidInput,
		},
		{
			name: "absolute path",
			build: func(archive *zip.Writer) {
				writeTestZipEntry(t, archive, "/etc/passwd", "x", 0)
			},
			target: domain.ErrInvalidInput,
		},
		{
			name: "duplicate path",
			build: func(archive *zip.Writer) {
				writeTestZipEntry(t, archive, "domain.xml", "one", 0)
				writeTestZipEntry(t, archive, "domain.xml", "two", 0)
			},
			target: domain.ErrInvalidInput,
		},
		{
			name: "symlink",
			build: func(archive *zip.Writer) {
				writeTestZipEntry(t, archive, "domain.xml", "/etc/passwd", os.ModeSymlink)
			},
			target: domain.ErrInvalidInput,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := indexBackupArchive(makeTestZipFiles(t, test.build))
			if !errors.Is(err, test.target) {
				t.Fatalf("error = %v, want %v", err, test.target)
			}
		})
	}
}

func TestSafeBackupArchivePath(t *testing.T) {
	for _, value := range []string{"package.json", "domain.xml", "disks/0000.qcow2"} {
		if !safeBackupArchivePath(value) {
			t.Fatalf("safe path rejected: %q", value)
		}
	}
	for _, value := range []string{"", ".", "./package.json", "a/../b", "../x", "/x", `a\b`, "a//b"} {
		if safeBackupArchivePath(value) {
			t.Fatalf("unsafe path accepted: %q", value)
		}
	}
}

func TestBackupPackageNameAndDiskNamesAreBounded(t *testing.T) {
	name := backupVolumeNameForImport("vmb_123", "-manifest.json")
	if len(name) > 127 || backupPackageDiskPath(3, "QCOW2", "disk.img") != "disks/0003.img" {
		t.Fatalf("name = %q, path = %q", name, backupPackageDiskPath(3, "QCOW2", "disk.img"))
	}
	if got := backupPackageDiskPath(1, "raw", "artifact"); got != "disks/0001.raw" {
		t.Fatalf("fallback disk path = %q", got)
	}
}

func backupMutationContext() MutationContext {
	return MutationContext{Session: domain.Session{User: domain.User{ID: 1, Username: "admin", Role: domain.RoleAdmin}}}
}

func makeTestZipFiles(t *testing.T, build func(*zip.Writer)) []*zip.File {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	build(archive)
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return reader.File
}

func writeTestZipEntry(t *testing.T, archive *zip.Writer, name, body string, mode os.FileMode) {
	t.Helper()
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(mode)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, body); err != nil {
		t.Fatal(err)
	}
}
