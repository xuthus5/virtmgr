package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestVMMetadataStoreUpsertAndList(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)

	if _, err := store.GetVMMetadata(ctx, "vm-a"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing metadata error = %v", err)
	}
	created, err := store.UpsertVMMetadata(ctx, domain.VMMetadata{
		VMUUID: "vm-a", Tags: []string{"prod", "数据库"}, Note: "主业务虚拟机", Owner: "ops", Folder: "生产/数据库", ResourceGroup: "核心业务",
		UpdatedBy: "admin", UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertVMMetadata() error = %v", err)
	}
	if created.VMUUID != "vm-a" || len(created.Tags) != 2 || created.Tags[1] != "数据库" || created.Folder != "生产/数据库" || created.ResourceGroup != "核心业务" || !created.UpdatedAt.Equal(now) {
		t.Fatalf("created metadata = %+v", created)
	}

	updated, err := store.UpsertVMMetadata(ctx, domain.VMMetadata{
		VMUUID: "vm-a", Tags: []string{"test"}, Note: "更新", Owner: "dev", Folder: "测试", ResourceGroup: "研发",
		UpdatedBy: "operator", UpdatedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("second UpsertVMMetadata() error = %v", err)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "test" || updated.Owner != "dev" || updated.Folder != "测试" || updated.ResourceGroup != "研发" || updated.UpdatedBy != "operator" {
		t.Fatalf("updated metadata = %+v", updated)
	}

	items, err := store.ListVMMetadata(ctx)
	if err != nil || len(items) != 1 || items["vm-a"].Note != "更新" {
		t.Fatalf("ListVMMetadata() = %+v, %v", items, err)
	}
	if err := store.DeleteVMMetadata(ctx, "vm-a"); err != nil {
		t.Fatalf("DeleteVMMetadata() error = %v", err)
	}
	if _, err := store.GetVMMetadata(ctx, "vm-a"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted metadata error = %v", err)
	}
}
