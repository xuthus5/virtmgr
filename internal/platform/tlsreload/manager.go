package tlsreload

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"virtmgr/internal/domain"
)

type certificateFileState struct {
	size    int64
	modTime time.Time
	mode    os.FileMode
}

type certificateState struct {
	certificate *tls.Certificate
	leaf        *x509.Certificate
	certFile    certificateFileState
	keyFile     certificateFileState
	loadedAt    time.Time
	reloadError string
}

type Manager struct {
	certPath string
	keyPath  string
	now      func() time.Time
	mu       sync.Mutex
	state    atomic.Pointer[certificateState]
}

func New(certPath, keyPath string) (*Manager, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return nil, errors.New("TLS certificate and key paths are required")
	}
	manager := &Manager{certPath: certPath, keyPath: keyPath, now: time.Now}
	state, err := manager.load()
	if err != nil {
		return nil, err
	}
	manager.state.Store(state)
	return manager, nil
}

func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: m.Certificate,
	}
}

func (m *Manager) Certificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.reloadIfChanged()
	state := m.state.Load()
	if state == nil || state.certificate == nil {
		return nil, errors.New("TLS certificate is unavailable")
	}
	return state.certificate, nil
}

func (m *Manager) Status(requestHost string) domain.TLSInfo {
	m.reloadIfChanged()
	state := m.state.Load()
	if state == nil || state.leaf == nil {
		return domain.TLSInfo{Enabled: false}
	}
	now := m.now().UTC()
	host := normalizeHost(requestHost)
	matched := host == "" || state.leaf.VerifyHostname(host) == nil
	warnings := make([]string, 0, 3)
	if !matched {
		warnings = append(warnings, fmt.Sprintf("当前访问主机 %s 不在证书 SAN 中；请在证书中加入该地址，或改用稳定 DNS 名称访问。", host))
	}
	if state.reloadError != "" {
		warnings = append(warnings, "证书热重载失败，服务仍在继续使用上一份有效证书："+state.reloadError)
	}
	if state.keyFile.mode.Perm()&0o077 != 0 {
		warnings = append(warnings, fmt.Sprintf("TLS 私钥文件权限为 %04o，组或其他用户可能读取该文件；建议设置为 0600。", state.keyFile.mode.Perm()))
	}
	days := int(state.leaf.NotAfter.Sub(now).Hours() / 24)
	if !state.leaf.NotAfter.After(now) {
		warnings = append(warnings, "管理面 TLS 证书已过期。")
	} else if days <= 30 {
		warnings = append(warnings, fmt.Sprintf("管理面 TLS 证书将在 %d 天内到期。", days))
	}
	notBefore := state.leaf.NotBefore.UTC()
	notAfter := state.leaf.NotAfter.UTC()
	loadedAt := state.loadedAt.UTC()
	return domain.TLSInfo{
		Enabled: true, Subject: state.leaf.Subject.String(), Issuer: state.leaf.Issuer.String(),
		NotBefore: &notBefore, NotAfter: &notAfter, DNSNames: append([]string(nil), state.leaf.DNSNames...),
		IPAddresses: certificateIPs(state.leaf.IPAddresses), RequestHost: host, RequestHostMatched: matched,
		ExpiresInDays: days, KeyFileMode: fmt.Sprintf("%04o", state.keyFile.mode.Perm()),
		LastReloadedAt: &loadedAt, ReloadError: state.reloadError, Warnings: warnings,
	}
}

func (m *Manager) reloadIfChanged() {
	current := m.state.Load()
	if current == nil {
		return
	}
	certFile, certErr := statFile(m.certPath)
	keyFile, keyErr := statFile(m.keyPath)
	if certErr == nil && keyErr == nil && certFile == current.certFile && keyFile == current.keyFile && current.reloadError == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current = m.state.Load()
	if current == nil {
		return
	}
	certFile, certErr = statFile(m.certPath)
	keyFile, keyErr = statFile(m.keyPath)
	if certErr == nil && keyErr == nil && certFile == current.certFile && keyFile == current.keyFile && current.reloadError == "" {
		return
	}
	next, err := m.load()
	if err != nil {
		failed := *current
		failed.reloadError = err.Error()
		m.state.Store(&failed)
		return
	}
	m.state.Store(next)
}

func (m *Manager) load() (*certificateState, error) {
	certificate, err := tls.LoadX509KeyPair(m.certPath, m.keyPath)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}
	if len(certificate.Certificate) == 0 {
		return nil, errors.New("TLS certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse TLS leaf certificate: %w", err)
	}
	certFile, err := statFile(m.certPath)
	if err != nil {
		return nil, err
	}
	keyFile, err := statFile(m.keyPath)
	if err != nil {
		return nil, err
	}
	certificate.Leaf = leaf
	return &certificateState{
		certificate: &certificate, leaf: leaf, certFile: certFile, keyFile: keyFile, loadedAt: m.now().UTC(),
	}, nil
}

func statFile(path string) (certificateFileState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return certificateFileState{}, fmt.Errorf("stat TLS file %s: %w", path, err)
	}
	return certificateFileState{size: info.Size(), modTime: info.ModTime(), mode: info.Mode()}, nil
}

func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(value, "[]")
}

func certificateIPs(values []net.IP) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.String())
	}
	return result
}
