package sqlite

import (
	"context"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestBackupOnlineConsistencyMetadataRoundTrip(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	if _, err := store.CreateBackupRepository(ctx, domain.BackupRepository{
		ID: "repo-1", Name: "archive", Type: "libvirt-pool", PoolName: "default",
	}); err != nil {
		t.Fatalf("CreateBackupRepository() error = %v", err)
	}
	created, err := store.CreateBackup(ctx, domain.Backup{
		ID: "backup-1", VMUUID: "uuid-1", VMName: "web-01", RepositoryID: "repo-1",
		TargetPool: "default", Name: "nightly", Status: domain.BackupStatusRunning,
		FormatVersion: domain.VMBackupFormatVersionV2, ManifestVolume: "backup-1-manifest.json",
		ConfigVolume: "backup-1-domain.xml", VerifyStatus: domain.BackupVerifyUnknown,
		CreatedBy: "operator", CreatedAt: time.Now().UTC(), Online: true, Consistent: true,
		ContainsTPM: true,
	})
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}
	if !created.Online || !created.Consistent || created.Quiesced || !created.ContainsTPM || created.TPMStateSaved {
		t.Fatalf("created backup metadata = %+v", created)
	}

	created.Quiesced = true
	created.TPMStateSaved = true
	updated, err := store.UpdateBackup(ctx, created)
	if err != nil {
		t.Fatalf("UpdateBackup() error = %v", err)
	}
	if !updated.Online || !updated.Consistent || !updated.Quiesced || !updated.ContainsTPM || !updated.TPMStateSaved {
		t.Fatalf("updated backup metadata = %+v", updated)
	}
}
