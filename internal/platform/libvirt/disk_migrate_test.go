package libvirtbackend

import (
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestRewriteDomainDiskSource(t *testing.T) {
	source := "/var/lib/libvirt/images/a&b.qcow2"
	xmlText := `<domain><devices><disk type='file'><source file='/var/lib/libvirt/images/a&amp;b.qcow2'/></disk></devices></domain>`
	rewritten, err := rewriteDomainDiskSource(xmlText, source, "/srv/fast/a&b.qcow2")
	if err != nil {
		t.Fatalf("rewriteDomainDiskSource() error = %v", err)
	}
	if !strings.Contains(rewritten, "source file='/srv/fast/a&amp;b.qcow2'") {
		t.Fatalf("rewritten XML = %s", rewritten)
	}
}

func TestRewriteDomainDiskSourceRejectsAmbiguousReferences(t *testing.T) {
	source := "/var/lib/libvirt/images/a.qcow2"
	for _, xmlText := range []string{
		`<domain><devices><disk><source file='/var/lib/libvirt/images/b.qcow2'/></disk></devices></domain>`,
		`<domain><devices><disk><source file='/var/lib/libvirt/images/a.qcow2'/></disk><disk><source file='/var/lib/libvirt/images/a.qcow2'/></disk></devices></domain>`,
	} {
		if _, err := rewriteDomainDiskSource(xmlText, source, "/srv/fast/a.qcow2"); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("rewrite error = %v for %s", err, xmlText)
		}
	}
}
