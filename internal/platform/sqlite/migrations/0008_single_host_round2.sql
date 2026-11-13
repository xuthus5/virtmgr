ALTER TABLE tasks ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE snapshot_policies ADD COLUMN schedule_mode TEXT NOT NULL DEFAULT 'interval';
ALTER TABLE snapshot_policies ADD COLUMN daily_time TEXT NOT NULL DEFAULT '02:00';

ALTER TABLE backup_schedules ADD COLUMN schedule_mode TEXT NOT NULL DEFAULT 'interval';
ALTER TABLE backup_schedules ADD COLUMN daily_time TEXT NOT NULL DEFAULT '02:00';

CREATE TABLE vm_metadata (
    vm_uuid TEXT PRIMARY KEY,
    tags_json TEXT NOT NULL DEFAULT '[]',
    note TEXT NOT NULL DEFAULT '',
    owner TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

CREATE TABLE alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL CHECK (operator IN ('gt', 'gte', 'lt', 'lte')),
    threshold REAL NOT NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    webhook_url TEXT NOT NULL DEFAULT '',
    last_state TEXT NOT NULL DEFAULT 'ok',
    last_changed_at INTEGER,
    last_notified_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE metric_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scope TEXT NOT NULL,
    target_id TEXT NOT NULL,
    sampled_at INTEGER NOT NULL,
    cpu_percent REAL NOT NULL DEFAULT 0,
    memory_used_bytes INTEGER NOT NULL DEFAULT 0,
    memory_total_bytes INTEGER NOT NULL DEFAULT 0,
    disk_read_bytes_per_second INTEGER NOT NULL DEFAULT 0,
    disk_write_bytes_per_second INTEGER NOT NULL DEFAULT 0,
    network_rx_bytes_per_second INTEGER NOT NULL DEFAULT 0,
    network_tx_bytes_per_second INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX metric_samples_lookup_idx ON metric_samples(scope, target_id, sampled_at DESC);
