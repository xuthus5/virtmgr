CREATE TABLE api_tokens (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    scopes TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER,
    last_used_at INTEGER,
    revoked_at INTEGER
);

CREATE INDEX api_tokens_user_created_idx ON api_tokens(user_id, created_at DESC);
CREATE INDEX api_tokens_token_hash_idx ON api_tokens(token_hash);

CREATE TABLE vm_images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    description TEXT NOT NULL DEFAULT '',
    pool TEXT NOT NULL,
    volume TEXT NOT NULL,
    format TEXT NOT NULL CHECK (format IN ('qcow2', 'raw')),
    os_family TEXT NOT NULL DEFAULT '',
    os_name TEXT NOT NULL DEFAULT '',
    architecture TEXT NOT NULL DEFAULT '',
    capacity_bytes INTEGER NOT NULL DEFAULT 0,
    allocation_bytes INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(pool, volume)
);

CREATE INDEX vm_images_name_idx ON vm_images(name COLLATE NOCASE);

ALTER TABLE vm_templates ADD COLUMN image_id INTEGER REFERENCES vm_images(id) ON DELETE RESTRICT;
