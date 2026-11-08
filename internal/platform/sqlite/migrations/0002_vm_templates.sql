CREATE TABLE vm_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    vcpus INTEGER NOT NULL CHECK (vcpus > 0),
    memory_mib INTEGER NOT NULL CHECK (memory_mib > 0),
    disk_format TEXT NOT NULL CHECK (disk_format IN ('qcow2', 'raw')),
    network TEXT NOT NULL DEFAULT 'default',
    iso_path TEXT NOT NULL DEFAULT '',
    firmware TEXT NOT NULL CHECK (firmware IN ('bios', 'uefi')),
    secure_boot INTEGER NOT NULL DEFAULT 0,
    cloud_init INTEGER NOT NULL DEFAULT 0,
    cloud_init_user_data TEXT NOT NULL DEFAULT '',
    autostart INTEGER NOT NULL DEFAULT 0,
    start INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX vm_templates_updated_at_idx ON vm_templates(updated_at DESC);
