package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseVMXMLIdentity(t *testing.T) {
	identity, err := ParseVMXMLIdentity(`<?xml version="1.0"?>
<domain type="kvm">
  <name> api-01 </name>
  <uuid>uuid-a</uuid>
  <devices/>
</domain>`)
	if err != nil {
		t.Fatalf("ParseVMXMLIdentity() error = %v", err)
	}
	if identity.Name != "api-01" || identity.UUID != "uuid-a" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestParseVMXMLIdentityRejectsInvalidOrUnsafeXML(t *testing.T) {
	tests := map[string]string{
		"empty":            " ",
		"wrong root":       `<network><name>api-01</name><uuid>uuid-a</uuid></network>`,
		"missing uuid":     `<domain><name>api-01</name></domain>`,
		"broken xml":       `<domain><name>api-01</name>`,
		"multiple roots":   `<domain><name>api-01</name><uuid>uuid-a</uuid></domain><domain/>`,
		"qemu commandline": `<domain xmlns:qemu="http://libvirt.org/schemas/domain/qemu/1.0"><name>api-01</name><uuid>uuid-a</uuid><qemu:commandline><qemu:arg value="-s"/></qemu:commandline></domain>`,
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseVMXMLIdentity(text); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParseVMXMLIdentityAllowsMetadataNamespaces(t *testing.T) {
	identity, err := ParseVMXMLIdentity(`<domain xmlns:virtmgr="https://example.test/virtmgr"><name>api-01</name><uuid>uuid-a</uuid><metadata><virtmgr:owner>ops</virtmgr:owner></metadata></domain>`)
	if err != nil {
		t.Fatalf("ParseVMXMLIdentity() error = %v", err)
	}
	if identity.Name != "api-01" || strings.TrimSpace(identity.UUID) != "uuid-a" {
		t.Fatalf("identity = %+v", identity)
	}
}
