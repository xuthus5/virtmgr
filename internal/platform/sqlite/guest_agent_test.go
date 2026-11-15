package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestGuestAgentPolicyAndHealthStore(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)
	policy, err := store.UpsertGuestAgentPolicy(ctx, domain.GuestAgentPolicy{
		VMUUID: "vm-a", Expected: true, LastState: domain.GuestAgentStateConnected,
		LastHealthyAt: &now, UpdatedBy: "admin", UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("UpsertGuestAgentPolicy() error = %v", err)
	}
	if !policy.Expected || policy.LastHealthyAt == nil || policy.UpdatedBy != "admin" {
		t.Fatalf("policy = %+v", policy)
	}
	if err := store.InsertGuestAgentHealthSample(ctx, domain.GuestAgentHealthSample{
		VMUUID: "vm-a", SampledAt: now, State: domain.GuestAgentStateConnected, Connected: true, LatencyMS: 12,
	}); err != nil {
		t.Fatalf("InsertGuestAgentHealthSample() error = %v", err)
	}
	samples, err := store.ListGuestAgentHealthSamples(ctx, "vm-a", now.Add(-time.Hour), 10)
	if err != nil || len(samples) != 1 || !samples[0].Connected || samples[0].LatencyMS != 12 {
		t.Fatalf("samples = %+v, %v", samples, err)
	}
	if err := store.DeleteGuestAgentHealthSamplesBefore(ctx, now.Add(time.Second)); err != nil {
		t.Fatalf("DeleteGuestAgentHealthSamplesBefore() error = %v", err)
	}
	samples, err = store.ListGuestAgentHealthSamples(ctx, "vm-a", now.Add(-time.Hour), 10)
	if err != nil || len(samples) != 0 {
		t.Fatalf("samples after prune = %+v, %v", samples, err)
	}
	policy.Expected = false
	policy.ConsecutiveFailures = 2
	policy.UpdatedAt = now.Add(time.Minute)
	updated, err := store.UpsertGuestAgentPolicy(ctx, policy)
	if err != nil || updated.Expected || updated.ConsecutiveFailures != 2 {
		t.Fatalf("updated policy = %+v, %v", updated, err)
	}
	if err := store.DeleteGuestAgentPolicy(ctx, "vm-a"); err != nil {
		t.Fatalf("DeleteGuestAgentPolicy() error = %v", err)
	}
	if _, err := store.GetGuestAgentPolicy(ctx, "vm-a"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted policy error = %v", err)
	}
}

func TestGuestAgentSnapshotRecordAndVMCleanup(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	if err := store.UpsertGuestAgentSnapshotRecord(ctx, domain.GuestAgentSnapshotRecord{
		VMUUID: "vm-a", SnapshotName: "before-upgrade", Quiesced: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListGuestAgentSnapshotRecords(ctx, "vm-a")
	if err != nil || len(records) != 1 || !records[0].Quiesced {
		t.Fatalf("records = %+v, %v", records, err)
	}
	if _, err := store.UpsertGuestAgentPolicy(ctx, domain.GuestAgentPolicy{VMUUID: "vm-a", Expected: true, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertGuestAgentHealthSample(ctx, domain.GuestAgentHealthSample{VMUUID: "vm-a", SampledAt: now, Connected: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteGuestAgentSnapshotRecord(ctx, "vm-a", "before-upgrade"); err != nil {
		t.Fatal(err)
	}
	records, _ = store.ListGuestAgentSnapshotRecords(ctx, "vm-a")
	if len(records) != 0 {
		t.Fatalf("snapshot records were not deleted: %+v", records)
	}
	if err := store.UpsertGuestAgentSnapshotRecord(ctx, domain.GuestAgentSnapshotRecord{VMUUID: "vm-a", SnapshotName: "s2", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteGuestAgentVMData(ctx, "vm-a"); err != nil {
		t.Fatal(err)
	}
	if records, _ = store.ListGuestAgentSnapshotRecords(ctx, "vm-a"); len(records) != 0 {
		t.Fatalf("VM cleanup left snapshots: %+v", records)
	}
	if _, err := store.GetGuestAgentPolicy(ctx, "vm-a"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("VM cleanup left policy: %v", err)
	}
}
