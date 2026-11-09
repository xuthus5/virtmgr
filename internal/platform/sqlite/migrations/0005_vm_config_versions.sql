CREATE TABLE vm_config_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vm_uuid TEXT NOT NULL,
    version_no INTEGER NOT NULL,
    xml_sha256 TEXT NOT NULL,
    xml_data BLOB NOT NULL,
    size_bytes INTEGER NOT NULL,
    source TEXT NOT NULL,
    source_version INTEGER NOT NULL DEFAULT 0,
    actor TEXT NOT NULL,
    role TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    source_ip TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    protected INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    UNIQUE(vm_uuid, version_no)
);

CREATE INDEX vm_config_versions_vm_created_idx
    ON vm_config_versions(vm_uuid, version_no DESC);
CREATE INDEX vm_config_versions_created_idx
    ON vm_config_versions(created_at DESC);
