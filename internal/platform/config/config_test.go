package config

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("VIRTMGR_LISTEN", "0.0.0.0:9999")
	t.Setenv("VIRTMGR_READ_ONLY", "true")
	cfg, err := Load(writeConfig(t, ""))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Listen != "127.0.0.1:8443" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.SessionTTL.Hours() != 8 {
		t.Fatalf("SessionTTL = %v", cfg.SessionTTL)
	}
	if cfg.ReadOnly {
		t.Fatal("ReadOnly must default to false")
	}
	if cfg.TLSEnabled() {
		t.Fatal("TLS must be disabled by default")
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("TrustedProxies = %v", cfg.TrustedProxies)
	}
}

func TestLoadReadsYAMLOverrides(t *testing.T) {
	path := writeConfig(t, `
listen: "0.0.0.0:9000"
libvirt_uri: test:///default
database_path: /tmp/virtmgr.db
host_name: yaml-host
admin_username: root
admin_password: secret
session_ttl: 30m
tls_cert_file: /etc/virtmgr/tls.crt
tls_key_file: /etc/virtmgr/tls.key
cookie_secure: true
read_only: true
demo_mode: true
trusted_proxies:
  - 127.0.0.1
  - 10.0.0.0/8
  - ::1
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Listen != "0.0.0.0:9000" || cfg.SessionTTL.Minutes() != 30 || !cfg.CookieSecure || !cfg.ReadOnly || !cfg.DemoMode || !cfg.TLSEnabled() {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.HostName != "yaml-host" || cfg.AdminUsername != "root" || cfg.AdminPassword != "secret" {
		t.Fatalf("admin or host config not loaded: %+v", cfg)
	}
	for _, want := range []netip.Prefix{
		netip.MustParsePrefix("127.0.0.1/32"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	} {
		found := false
		for _, prefix := range cfg.TrustedProxies {
			if prefix == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("TrustedProxies = %v, missing %s", cfg.TrustedProxies, want)
		}
	}
}

func TestLoadRejectsMissingOrInvalidFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("Load() error = nil for a missing file")
	}
	for name, contents := range map[string]string{
		"unknown field":      "unknown_option: true\n",
		"invalid session":    "session_ttl: soon\n",
		"partial tls":        "tls_cert_file: /etc/virtmgr/tls.crt\n",
		"empty listen":       "listen: \"\"\n",
		"empty admin":        "admin_username: \"\"\n",
		"empty proxies":      "trusted_proxies: [\"\"]\n",
		"invalid proxy":      "trusted_proxies: [not-an-ip]\n",
		"multiple documents": "listen: 127.0.0.1:8443\n---\nlisten: 127.0.0.1:9443\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeConfig(t, contents))
			if err == nil {
				t.Fatalf("Load() error = nil for %q", contents)
			}
			if !strings.Contains(err.Error(), "server.yaml") && name != "multiple documents" {
				t.Fatalf("error should include config path context: %v", err)
			}
		})
	}
}

func TestLoadRejectsEmptyPath(t *testing.T) {
	if _, err := Load(" "); err == nil {
		t.Fatal("Load() error = nil for an empty path")
	}
}

func TestTLSEnabled(t *testing.T) {
	if (Config{}).TLSEnabled() {
		t.Fatal("empty config must not enable TLS")
	}
	if !(Config{TLSCertFile: "cert", TLSKeyFile: "key"}).TLSEnabled() {
		t.Fatal("complete TLS config must enable TLS")
	}
}
