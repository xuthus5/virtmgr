package config

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultPath = "/etc/virtmgr/server.yaml"

type Config struct {
	Listen          string
	TLSCertFile     string
	TLSKeyFile      string
	LibvirtURI      string
	DatabasePath    string
	HostName        string
	SessionTTL      time.Duration
	CookieSecure    bool
	AdminUsername   string
	AdminPassword   string
	ReadOnly        bool
	DemoMode        bool
	TrustedProxies  []netip.Prefix
	AllowedNetworks []netip.Prefix
}

type fileConfig struct {
	Listen          string   `yaml:"listen"`
	TLSCertFile     string   `yaml:"tls_cert_file"`
	TLSKeyFile      string   `yaml:"tls_key_file"`
	LibvirtURI      string   `yaml:"libvirt_uri"`
	DatabasePath    string   `yaml:"database_path"`
	HostName        string   `yaml:"host_name"`
	AdminUsername   string   `yaml:"admin_username"`
	AdminPassword   string   `yaml:"admin_password"`
	SessionTTL      string   `yaml:"session_ttl"`
	CookieSecure    bool     `yaml:"cookie_secure"`
	TrustedProxies  []string `yaml:"trusted_proxies"`
	AllowedNetworks []string `yaml:"management_allowed_networks"`
	ReadOnly        bool     `yaml:"read_only"`
	DemoMode        bool     `yaml:"demo_mode"`
}

func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, errors.New("server.yaml path must not be empty")
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "libvirt-host"
	}
	document := fileConfig{
		Listen:        "127.0.0.1:8443",
		LibvirtURI:    "qemu:///system",
		DatabasePath:  "/var/lib/virtmgr/virtmgr.db",
		HostName:      hostname,
		AdminUsername: "admin",
		SessionTTL:    "8h",
		ReadOnly:      false,
	}
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
		return Config{}, fmt.Errorf("parse %s: server.yaml must contain exactly one YAML document", path)
	}

	ttl, err := time.ParseDuration(strings.TrimSpace(document.SessionTTL))
	if err != nil || ttl <= 0 {
		return Config{}, fmt.Errorf("parse %s: session_ttl must be a positive duration", path)
	}
	trustedProxies, err := parsePrefixes("trusted_proxies", document.TrustedProxies)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	allowedNetworks, err := parsePrefixes("management_allowed_networks", document.AllowedNetworks)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg := Config{
		Listen:          strings.TrimSpace(document.Listen),
		TLSCertFile:     strings.TrimSpace(document.TLSCertFile),
		TLSKeyFile:      strings.TrimSpace(document.TLSKeyFile),
		LibvirtURI:      strings.TrimSpace(document.LibvirtURI),
		DatabasePath:    strings.TrimSpace(document.DatabasePath),
		HostName:        strings.TrimSpace(document.HostName),
		SessionTTL:      ttl,
		CookieSecure:    document.CookieSecure,
		AdminUsername:   strings.TrimSpace(document.AdminUsername),
		AdminPassword:   document.AdminPassword,
		ReadOnly:        document.ReadOnly,
		DemoMode:        document.DemoMode,
		TrustedProxies:  trustedProxies,
		AllowedNetworks: allowedNetworks,
	}
	if cfg.Listen == "" {
		return Config{}, fmt.Errorf("validate %s: listen must not be empty", path)
	}
	if cfg.LibvirtURI == "" {
		return Config{}, fmt.Errorf("validate %s: libvirt_uri must not be empty", path)
	}
	if cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("validate %s: database_path must not be empty", path)
	}
	if cfg.HostName == "" {
		return Config{}, fmt.Errorf("validate %s: host_name must not be empty", path)
	}
	if cfg.AdminUsername == "" {
		return Config{}, fmt.Errorf("validate %s: admin_username must not be empty", path)
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("validate %s: tls_cert_file and tls_key_file must be configured together", path)
	}
	return cfg, nil
}

func (c Config) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}

func parsePrefixes(field string, values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		entry := strings.TrimSpace(value)
		if entry == "" {
			return nil, fmt.Errorf("%s contains an empty entry", field)
		}
		prefix, err := parsePrefix(entry)
		if err != nil {
			return nil, fmt.Errorf("%s entry %q: %w", field, entry, err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func parsePrefix(value string) (netip.Prefix, error) {
	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return netip.Prefix{}, err
		}
		return prefix.Masked(), nil
	}
	address, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	address = address.Unmap()
	return netip.PrefixFrom(address, address.BitLen()), nil
}
