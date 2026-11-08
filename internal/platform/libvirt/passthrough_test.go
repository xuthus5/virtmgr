package libvirtbackend

import (
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestPassthroughDeviceXMLPCI(t *testing.T) {
	value, err := passthroughDeviceXML(domain.PassthroughDevice{
		Name: "pci_0000_3b_00_0", Type: "pci", Address: "0000:3b:00.0",
	})
	if err != nil {
		t.Fatalf("passthroughDeviceXML() error = %v", err)
	}
	for _, fragment := range []string{"type='pci'", "managed='yes'", "domain='0x0000'", "bus='0x3b'", "slot='0x00'", "function='0x0'"} {
		if !strings.Contains(value, fragment) {
			t.Fatalf("XML missing %q: %s", fragment, value)
		}
	}
}

func TestPassthroughDeviceXMLUSB(t *testing.T) {
	value, err := passthroughDeviceXML(domain.PassthroughDevice{
		Name: "usb_device_046d_c52b_1_2", Type: "usb", VendorID: "0x046d", ProductID: "0xC52B", Address: "bus 1 · device 2",
	})
	if err != nil {
		t.Fatalf("passthroughDeviceXML() error = %v", err)
	}
	for _, fragment := range []string{"type='usb'", "vendor id='0x046d'", "product id='0xc52b'", "address bus='1' device='2'"} {
		if !strings.Contains(value, fragment) {
			t.Fatalf("XML missing %q: %s", fragment, value)
		}
	}
}

func TestParseDomainXMLHostDevices(t *testing.T) {
	details, err := parseDomainXML(`<domain><devices>
  <hostdev mode='subsystem' type='pci' managed='yes'><source><address domain='0x0000' bus='0x3b' slot='0x00' function='0x0'/></source></hostdev>
  <hostdev mode='subsystem' type='usb' managed='yes'><source><vendor id='0x046d'/><product id='0xc52b'/><address bus='1' device='2'/></source></hostdev>
</devices></domain>`)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if len(details.Passthrough) != 2 {
		t.Fatalf("passthrough = %+v", details.Passthrough)
	}
	pci := details.Passthrough[0]
	if pci.Name != "pci_0000_3b_00_0" || pci.Type != "pci" || pci.Address != "0000:3b:00.0" {
		t.Fatalf("pci = %+v", pci)
	}
	usb := details.Passthrough[1]
	if usb.Name != "usb_device_046d_c52b_1_2" || usb.Type != "usb" || usb.VendorID != "0x046d" || usb.ProductID != "0xc52b" || usb.Address != "bus 1 · device 2" {
		t.Fatalf("usb = %+v", usb)
	}
}

func TestPassthroughFromHostDevice(t *testing.T) {
	device, err := passthroughFromHostDevice(domain.HostDevice{
		Name: "usb_device_046d_c52b_1_2", Type: "usb_device", Vendor: "Logitech, Inc.", VendorID: "0x046d",
		Product: "Unifying Receiver", ProductID: "0xc52b", Address: "bus 1 · device 2", Driver: "usbhid",
	})
	if err != nil {
		t.Fatalf("passthroughFromHostDevice() error = %v", err)
	}
	if device.Type != "usb" || device.Name != "usb_device_046d_c52b_1_2" || device.Product != "Unifying Receiver" || device.Driver != "usbhid" {
		t.Fatalf("device = %+v", device)
	}
	if _, err := passthroughFromHostDevice(domain.HostDevice{Name: "net_enp2s0", Type: "net"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("unsupported type error = %v", err)
	}
}

func TestPassthroughDeviceFromName(t *testing.T) {
	pci, err := passthroughDeviceFromName("pci_0000_00_14_0")
	if err != nil || pci.Type != "pci" || pci.Address != "0000:00:14.0" {
		t.Fatalf("pci = %+v, %v", pci, err)
	}
	usb, err := passthroughDeviceFromName("usb_device_046d_c52b_1_2")
	if err != nil || usb.Type != "usb" || usb.VendorID != "0x046d" || usb.ProductID != "0xc52b" {
		t.Fatalf("usb = %+v, %v", usb, err)
	}
	usbWithoutAddress, err := passthroughDeviceFromName("usb_device_046d_c52b")
	if err != nil || usbWithoutAddress.Address != "" || usbWithoutAddress.VendorID != "0x046d" {
		t.Fatalf("usb without address = %+v, %v", usbWithoutAddress, err)
	}
	if _, err := passthroughDeviceFromName("net_enp2s0"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("unsupported name error = %v", err)
	}
}
