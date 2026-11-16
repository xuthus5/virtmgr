package libvirtbackend

import (
	"strings"
	"testing"
)

func TestBuildLinkedCloneVolumeXML(t *testing.T) {
	xmlText := buildLinkedCloneVolumeXML("child.qcow2", 20<<30, "/var/lib/libvirt/images/base.qcow2")
	for _, fragment := range []string{
		"<name>child.qcow2</name>", "<capacity unit='bytes'>21474836480</capacity>",
		"<allocation unit='bytes'>0</allocation>", "<backingStore><path>/var/lib/libvirt/images/base.qcow2</path><format type='qcow2'/></backingStore>",
	} {
		if !strings.Contains(xmlText, fragment) {
			t.Fatalf("linked clone XML missing %q: %s", fragment, xmlText)
		}
	}
	if !storageVolumeXMLReferencesBacking(xmlText, "/var/lib/libvirt/images/base.qcow2") {
		t.Fatalf("linked clone XML backing store was not detected: %s", xmlText)
	}
	if storageVolumeXMLReferencesBacking(xmlText, "/var/lib/libvirt/images/other.qcow2") {
		t.Fatal("unexpected backing store match")
	}
}
