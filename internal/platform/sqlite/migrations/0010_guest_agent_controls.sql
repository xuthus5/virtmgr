CREATE TABLE guest_agent_policies (
    vm_uuid TEXT PRIMARY KEY,
    expected INTEGER NOT NULL DEFAULT 1,
    last_state TEXT NOT NULL DEFAULT 'unknown',
    last_error TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_checked_at INTEGER,
    last_healthy_at INTEGER,
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL
);

CREATE TABLE guest_agent_health_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vm_uuid TEXT NOT NULL,
    sampled_at INTEGER NOT NULL,
    state TEXT NOT NULL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    connected INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX guest_agent_health_lookup_idx
    ON guest_agent_health_samples(vm_uuid, sampled_at DESC);

CREATE TABLE guest_agent_snapshot_records (
    vm_uuid TEXT NOT NULL,
    snapshot_name TEXT NOT NULL,
    quiesced INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    PRIMARY KEY(vm_uuid, snapshot_name)
);

ALTER TABLE vm_templates ADD COLUMN guest_agent INTEGER NOT NULL DEFAULT 1;
