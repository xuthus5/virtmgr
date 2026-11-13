package runtimeconfig

import (
	"context"
	"testing"
)

func TestAuditRetentionSeedsAndPersists(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{values: map[string]string{}}
	retention, err := NewAuditRetention(ctx, store, 30)
	if err != nil || retention.Days() != 30 || store.values[AuditRetentionSettingKey] != "30" {
		t.Fatalf("NewAuditRetention() = %+v, %v store=%v", retention, err, store.values)
	}
	if err := retention.SetDays(ctx, 90); err != nil || retention.Days() != 90 || store.values[AuditRetentionSettingKey] != "90" {
		t.Fatalf("SetDays() = %v days=%d store=%v", err, retention.Days(), store.values)
	}
	reloaded, err := NewAuditRetention(ctx, store, 30)
	if err != nil || reloaded.Days() != 90 {
		t.Fatalf("reload = %+v, %v", reloaded, err)
	}
	if err := retention.SetDays(ctx, 0); err == nil {
		t.Fatal("invalid retention accepted")
	}
}
