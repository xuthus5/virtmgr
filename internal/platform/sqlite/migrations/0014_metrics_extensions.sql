ALTER TABLE metric_samples ADD COLUMN disk_read_operations_per_second INTEGER NOT NULL DEFAULT 0;
ALTER TABLE metric_samples ADD COLUMN disk_write_operations_per_second INTEGER NOT NULL DEFAULT 0;

CREATE TABLE guest_filesystem_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vm_uuid TEXT NOT NULL,
    vm_name TEXT NOT NULL,
    mountpoint TEXT NOT NULL,
    used_bytes INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    sampled_at INTEGER NOT NULL
);

CREATE INDEX guest_filesystem_samples_lookup_idx
    ON guest_filesystem_samples(vm_uuid, mountpoint, sampled_at DESC);

CREATE INDEX guest_filesystem_samples_retention_idx
    ON guest_filesystem_samples(sampled_at);

CREATE TABLE capacity_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scope TEXT NOT NULL,
    target_id TEXT NOT NULL,
    used_bytes INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    sampled_at INTEGER NOT NULL
);

CREATE INDEX capacity_samples_lookup_idx
    ON capacity_samples(scope, target_id, sampled_at DESC);

CREATE INDEX capacity_samples_retention_idx
    ON capacity_samples(sampled_at);
