package sqlite

import (
	"context"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestMonitoringSamplesRoundTripExtendedFields(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	if err := store.InsertMetricSample(ctx, domain.MetricSample{
		Scope: "vm", TargetID: "vm-a", SampledAt: now, CPUPercent: 12,
		DiskReadBytesPerSecond: 100, DiskWriteBytesPerSecond: 200,
		DiskReadIOPS: 30, DiskWriteIOPS: 40,
	}); err != nil {
		t.Fatalf("InsertMetricSample() error = %v", err)
	}
	samples, err := store.ListMetricSamples(ctx, "vm", "vm-a", now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("ListMetricSamples() error = %v", err)
	}
	if len(samples) != 1 || samples[0].DiskReadIOPS != 30 || samples[0].DiskWriteIOPS != 40 {
		t.Fatalf("metric samples = %+v", samples)
	}

	if err := store.InsertGuestFilesystemSample(ctx, domain.GuestFilesystemSample{
		VMUUID: "vm-a", VMName: "app-01", MountPoint: "/", UsedBytes: 40, TotalBytes: 100, SampledAt: now,
	}); err != nil {
		t.Fatalf("InsertGuestFilesystemSample() error = %v", err)
	}
	filesystems, err := store.ListGuestFilesystemSamples(ctx, "vm-a", now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("ListGuestFilesystemSamples() error = %v", err)
	}
	if len(filesystems) != 1 || filesystems[0].MountPoint != "/" || filesystems[0].UsedBytes != 40 {
		t.Fatalf("guest filesystem samples = %+v", filesystems)
	}

	if err := store.InsertCapacitySample(ctx, domain.CapacitySample{
		Scope: "storage_pool", TargetID: "default", UsedBytes: 500, TotalBytes: 1000, SampledAt: now,
	}); err != nil {
		t.Fatalf("InsertCapacitySample() error = %v", err)
	}
	capacities, err := store.ListCapacitySamples(ctx, "storage_pool", "", now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("ListCapacitySamples() error = %v", err)
	}
	if len(capacities) != 1 || capacities[0].TargetID != "default" || capacities[0].UsedBytes != 500 {
		t.Fatalf("capacity samples = %+v", capacities)
	}
}
