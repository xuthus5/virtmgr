CREATE TABLE port_forwards (
    id TEXT PRIMARY KEY,
    network TEXT NOT NULL,
    name TEXT NOT NULL,
    family TEXT NOT NULL CHECK (family IN ('ipv4', 'ipv6')),
    protocol TEXT NOT NULL CHECK (protocol IN ('tcp', 'udp')),
    host_address TEXT NOT NULL DEFAULT '',
    host_port INTEGER NOT NULL CHECK (host_port BETWEEN 1 AND 65535),
    guest_address TEXT NOT NULL,
    guest_port INTEGER NOT NULL CHECK (guest_port BETWEEN 1 AND 65535),
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (network, name)
);

CREATE INDEX port_forwards_network_idx ON port_forwards(network, created_at);
CREATE INDEX port_forwards_listener_idx ON port_forwards(family, protocol, host_address, host_port);
