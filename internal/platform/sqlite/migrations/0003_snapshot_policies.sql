CREATE TABLE snapshot_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vm_uuid TEXT NOT NULL,
    name TEXT NOT NULL,
    interval_minutes INTEGER NOT NULL,
    retention INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run_at INTEGER,
    next_run_at INTEGER NOT NULL,
    last_snapshot TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(vm_uuid, name)
);

CREATE INDEX snapshot_policies_due_idx ON snapshot_policies(enabled, next_run_at);
