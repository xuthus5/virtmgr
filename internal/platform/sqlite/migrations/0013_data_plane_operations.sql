CREATE TABLE data_plane_operations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    vm_uuid TEXT NOT NULL DEFAULT '',
    vm_name TEXT NOT NULL DEFAULT '',
    task_id TEXT NOT NULL DEFAULT '',
    backup_id TEXT NOT NULL DEFAULT '',
    phase TEXT NOT NULL DEFAULT '',
    online INTEGER NOT NULL DEFAULT 0,
    source_pool TEXT NOT NULL DEFAULT '',
    source_volume TEXT NOT NULL DEFAULT '',
    source_path TEXT NOT NULL DEFAULT '',
    target_pool TEXT NOT NULL DEFAULT '',
    target_name TEXT NOT NULL DEFAULT '',
    target_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX data_plane_operations_pending_idx
    ON data_plane_operations(status, created_at);

CREATE INDEX data_plane_operations_vm_idx
    ON data_plane_operations(vm_uuid, created_at DESC);
