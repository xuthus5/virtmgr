package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("VIRTMGR_LISTEN", "")
	t.Setenv("VIRTMGR_LIBVIRT_URI", "")
	t.Setenv("VIRTMGR_DATABASE_PATH", "")
	t.Setenv("VIRTMGR_SESSION_TTL", "")
	t.Setenv("VIRTMGR_ADMIN_PASSWORD", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Listen != "127.0.0.1:8443" {
		t.Fatalf("Listen = %q", cfg.Listen)
	}
	if cfg.SessionTTL.Hours() != 8 {
		t.Fatalf("SessionTTL = %v", cfg.SessionTTL)
	}
	if !cfg.ReadOnly {
		t.Fatal("ReadOnly must default to true")
	}
}

func TestLoadRejectsInvalidSessionTTL(t *testing.T) {
	t.Setenv("VIRTMGR_SESSION_TTL", "soon")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadReadsOverrides(t *testing.T) {
	t.Setenv("VIRTMGR_LISTEN", "0.0.0.0:9000")
	t.Setenv("VIRTMGR_LIBVIRT_URI", "test:///default")
	t.Setenv("VIRTMGR_DATABASE_PATH", "/tmp/virtmgr.db")
	t.Setenv("VIRTMGR_SESSION_TTL", "30m")
	t.Setenv("VIRTMGR_COOKIE_SECURE", "true")
	t.Setenv("VIRTMGR_READ_ONLY", "false")
	t.Setenv("VIRTMGR_DEMO_MODE", "true")
	t.Setenv("VIRTMGR_ADMIN_USERNAME", "root")
	t.Setenv("VIRTMGR_ADMIN_PASSWORD", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Listen != "0.0.0.0:9000" || cfg.SessionTTL.Minutes() != 30 || !cfg.CookieSecure || cfg.ReadOnly || !cfg.DemoMode {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.AdminUsername != "root" || cfg.AdminPassword != "secret" {
		t.Fatalf("admin config not loaded: %+v", cfg)
	}
}
