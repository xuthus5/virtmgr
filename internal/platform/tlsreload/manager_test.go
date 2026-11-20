package tlsreload

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeCertificate(t *testing.T, directory, commonName string, dnsNames []string, ips []net.IP) (string, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(directory, "tls.crt")
	keyPath := filepath.Join(directory, "tls.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestManagerReloadsCertificateAndReportsSANMismatch(t *testing.T) {
	directory := t.TempDir()
	certPath, keyPath := writeCertificate(t, directory, "v6.example", []string{"v6.example"}, []net.IP{net.ParseIP("::1")})
	manager, err := New(certPath, keyPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	status := manager.Status("[::1]:8443")
	if !status.Enabled || !status.RequestHostMatched || status.ExpiresInDays < 300 {
		t.Fatalf("status = %+v", status)
	}
	if status.RequestHost != "::1" || len(status.IPAddresses) != 1 || status.IPAddresses[0] != "::1" {
		t.Fatalf("IPv6 status = %+v", status)
	}
	if status.KeyFileMode != "0600" {
		t.Fatalf("key file mode = %q, want 0600", status.KeyFileMode)
	}
	status = manager.Status("old-prefix.example")
	if status.RequestHostMatched || len(status.Warnings) == 0 || !strings.Contains(status.Warnings[0], "不在证书 SAN 中") {
		t.Fatalf("mismatch status = %+v", status)
	}

	nextCert, nextKey := writeCertificate(t, directory, "new.example", []string{"new.example"}, nil)
	if err := os.Rename(nextCert, certPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(nextKey, keyPath); err != nil {
		t.Fatal(err)
	}
	changed := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(certPath, changed, changed); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(keyPath, changed, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Certificate(nil); err != nil {
		t.Fatalf("Certificate() after reload = %v", err)
	}
	status = manager.Status("new.example")
	if !status.RequestHostMatched || !strings.Contains(status.Subject, "new.example") {
		t.Fatalf("reloaded status = %+v", status)
	}
}

func TestManagerWarnsWhenPrivateKeyIsReadableByOtherUsers(t *testing.T) {
	directory := t.TempDir()
	certPath, keyPath := writeCertificate(t, directory, "v6.example", []string{"v6.example"}, nil)
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	manager, err := New(certPath, keyPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	status := manager.Status("v6.example")
	if status.KeyFileMode != "0644" {
		t.Fatalf("key file mode = %q, want 0644", status.KeyFileMode)
	}
	for _, warning := range status.Warnings {
		if strings.Contains(warning, "私钥文件权限") {
			return
		}
	}
	t.Fatalf("warnings = %v, want private key permission warning", status.Warnings)
}

func TestManagerKeepsPreviousCertificateWhenReloadFails(t *testing.T) {
	directory := t.TempDir()
	certPath, keyPath := writeCertificate(t, directory, "stable.example", []string{"stable.example"}, nil)
	manager, err := New(certPath, keyPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := os.WriteFile(certPath, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(certPath, changed, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Certificate(nil); err != nil {
		t.Fatalf("Certificate() must keep the previous certificate: %v", err)
	}
	status := manager.Status("stable.example")
	if status.ReloadError == "" || !status.RequestHostMatched {
		t.Fatalf("status = %+v", status)
	}
	hasReloadWarning := false
	for _, warning := range status.Warnings {
		if strings.Contains(warning, "热重载失败") {
			hasReloadWarning = true
		}
	}
	if !hasReloadWarning {
		t.Fatalf("warnings = %v", status.Warnings)
	}
}
