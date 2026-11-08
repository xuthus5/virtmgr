package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestSnapshotPolicyStoreCRUD(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	created, err := store.CreateSnapshotPolicy(ctx, domain.SnapshotPolicy{
		VMUUID:          "vm-a",
		Name:            "nightly",
		IntervalMinutes: 60,
		Retention:       3,
		Enabled:         true,
		NextRunAt:       now.Add(time.Hour),
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("CreateSnapshotPolicy() error = %v", err)
	}
	if created.ID < 1 || created.Name != "nightly" || created.LastRunAt != nil || !created.NextRunAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("created policy = %+v", created)
	}

	if _, err := store.CreateSnapshotPolicy(ctx, domain.SnapshotPolicy{
		VMUUID: "vm-a", Name: "nightly", IntervalMinutes: 60, Retention: 3,
		Enabled: true, NextRunAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate policy error = %v", err)
	}
	if _, err := store.CreateSnapshotPolicy(ctx, domain.SnapshotPolicy{
		VMUUID: "vm-b", Name: "nightly", IntervalMinutes: 60, Retention: 3,
		Enabled: true, NextRunAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("same name on another VM error = %v", err)
	}

	items, err := store.ListSnapshotPolicies(ctx, "vm-a")
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("ListSnapshotPolicies() = %+v, %v", items, err)
	}

	lastRun := now.Add(30 * time.Minute)
	created.Name = "weekly"
	created.IntervalMinutes = 7 * 24 * 60
	created.Retention = 5
	created.Enabled = false
	created.LastRunAt = &lastRun
	created.NextRunAt = now.Add(7 * 24 * time.Hour)
	created.LastSnapshot = "nightly-20260501T100000"
	created.LastError = "cleanup failed"
	created.UpdatedAt = now.Add(time.Minute)
	updated, err := store.UpdateSnapshotPolicy(ctx, created)
	if err != nil {
		t.Fatalf("UpdateSnapshotPolicy() error = %v", err)
	}
	if updated.Name != "weekly" || updated.Retention != 5 || updated.Enabled || updated.LastRunAt == nil || !updated.LastRunAt.Equal(lastRun) {
		t.Fatalf("updated policy = %+v", updated)
	}
	if updated.LastSnapshot != "nightly-20260501T100000" || updated.LastError != "cleanup failed" {
		t.Fatalf("updated run state = %+v", updated)
	}

	missing := created
	missing.ID = 999
	if _, err := store.UpdateSnapshotPolicy(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing update error = %v", err)
	}
	if err := store.DeleteSnapshotPolicy(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSnapshotPolicy() error = %v", err)
	}
	if _, err := store.GetSnapshotPolicy(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted policy error = %v", err)
	}
	if err := store.DeleteSnapshotPolicy(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
}

func TestSnapshotPolicyStoreDueQuery(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	create := func(name string, enabled bool, next time.Time) domain.SnapshotPolicy {
		t.Helper()
		policy, err := store.CreateSnapshotPolicy(ctx, domain.SnapshotPolicy{
			VMUUID: "vm-a", Name: name, IntervalMinutes: 60, Retention: 2,
			Enabled: enabled, NextRunAt: next, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return policy
	}

	due := create("due", true, now.Add(-time.Minute))
	future := create("future", true, now.Add(time.Minute))
	disabled := create("disabled", false, now.Add(-time.Hour))

	items, err := store.ListDueSnapshotPolicies(ctx, now, 100)
	if err != nil {
		t.Fatalf("ListDueSnapshotPolicies() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != due.ID {
		t.Fatalf("due policies = %+v", items)
	}

	items, err = store.ListDueSnapshotPolicies(ctx, now, 1)
	if err != nil || len(items) != 1 || items[0].ID != due.ID {
		t.Fatalf("limited due policies = %+v, %v", items, err)
	}
	items, err = store.ListDueSnapshotPolicies(ctx, now.Add(2*time.Minute), 0)
	if err != nil || len(items) != 2 || items[0].ID != due.ID || items[1].ID != future.ID {
		t.Fatalf("updated due policies = %+v, %v", items, err)
	}
	if disabled.Enabled {
		t.Fatalf("disabled policy unexpectedly enabled: %+v", disabled)
	}
}
