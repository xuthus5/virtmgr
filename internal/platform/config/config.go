package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen        string
	TLSCertFile   string
	TLSKeyFile    string
	LibvirtURI    string
	DatabasePath  string
	HostName      string
	SessionTTL    time.Duration
	CookieSecure  bool
	AdminUsername string
	AdminPassword string
	ReadOnly      bool
	DemoMode      bool
}

func Load() (Config, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "libvirt-host"
	}
	cfg := Config{
		Listen:        env("VIRTMGR_LISTEN", "127.0.0.1:8443"),
		TLSCertFile:   env("VIRTMGR_TLS_CERT_FILE", ""),
		TLSKeyFile:    env("VIRTMGR_TLS_KEY_FILE", ""),
		LibvirtURI:    env("VIRTMGR_LIBVIRT_URI", "qemu:///system"),
		DatabasePath:  env("VIRTMGR_DATABASE_PATH", "/var/lib/virtmgr/virtmgr.db"),
		HostName:      env("VIRTMGR_HOST_NAME", hostname),
		AdminUsername: env("VIRTMGR_ADMIN_USERNAME", "admin"),
		AdminPassword: os.Getenv("VIRTMGR_ADMIN_PASSWORD"),
		CookieSecure:  envBool("VIRTMGR_COOKIE_SECURE", false),
		ReadOnly:      envBool("VIRTMGR_READ_ONLY", true),
		DemoMode:      envBool("VIRTMGR_DEMO_MODE", false),
	}
	ttl, err := time.ParseDuration(env("VIRTMGR_SESSION_TTL", "8h"))
	if err != nil || ttl <= 0 {
		return Config{}, fmt.Errorf("VIRTMGR_SESSION_TTL must be a positive duration")
	}
	cfg.SessionTTL = ttl
	if strings.TrimSpace(cfg.Listen) == "" {
		return Config{}, fmt.Errorf("VIRTMGR_LISTEN must not be empty")
	}
	if strings.TrimSpace(cfg.LibvirtURI) == "" {
		return Config{}, fmt.Errorf("VIRTMGR_LIBVIRT_URI must not be empty")
	}
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		return Config{}, fmt.Errorf("VIRTMGR_DATABASE_PATH must not be empty")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("VIRTMGR_TLS_CERT_FILE and VIRTMGR_TLS_KEY_FILE must be configured together")
	}
	return cfg, nil
}

func (c Config) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
