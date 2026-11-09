CREATE TABLE backup_repositories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    type TEXT NOT NULL CHECK (type IN ('libvirt-pool')),
    pool_name TEXT NOT NULL UNIQUE,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX backup_repositories_default_idx ON backup_repositories(is_default) WHERE is_default = 1;

CREATE TABLE backups (
    id TEXT PRIMARY KEY,
    vm_uuid TEXT NOT NULL,
    vm_name TEXT NOT NULL,
    repository_id TEXT NOT NULL REFERENCES backup_repositories(id),
    target_pool TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    format_version TEXT NOT NULL,
    manifest_volume TEXT NOT NULL,
    config_volume TEXT NOT NULL,
    config_bytes INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    disk_count INTEGER NOT NULL DEFAULT 0,
    pinned INTEGER NOT NULL DEFAULT 0,
    verify_status TEXT NOT NULL DEFAULT 'unknown',
    verified_at INTEGER,
    verify_message TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE INDEX backups_created_at_idx ON backups(created_at DESC);
CREATE INDEX backups_vm_idx ON backups(vm_uuid, created_at DESC);
CREATE INDEX backups_status_idx ON backups(status, created_at DESC);

CREATE TABLE backup_disks (
    backup_id TEXT NOT NULL REFERENCES backups(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    source_volume TEXT NOT NULL,
    artifact_volume TEXT NOT NULL,
    format TEXT NOT NULL,
    capacity_bytes INTEGER NOT NULL,
    sha256 TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (backup_id, sequence)
);

CREATE TABLE backup_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vm_uuid TEXT NOT NULL,
    repository_id TEXT NOT NULL REFERENCES backup_repositories(id),
    target_pool TEXT NOT NULL,
    name TEXT NOT NULL,
    interval_hours INTEGER NOT NULL,
    retention_count INTEGER NOT NULL,
    retention_days INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run_at INTEGER,
    next_run_at INTEGER NOT NULL,
    last_backup_id TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(vm_uuid, name)
);

CREATE INDEX backup_schedules_due_idx ON backup_schedules(enabled, next_run_at);
