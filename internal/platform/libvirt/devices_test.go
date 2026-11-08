package libvirtbackend

import (
	"testing"
)

func TestParseHostDeviceXMLPCI(t *testing.T) {
	device, err := parseHostDeviceXML(`
<device>
  <name>pci_0000_00_02_0</name>
  <parent>computer</parent>
  <capability type='pci'>
    <domain>0x0000</domain>
    <bus>0x00</bus>
    <slot>0x02</slot>
    <function>0x0</function>
    <vendor id='0x8086'>Intel Corporation</vendor>
    <product id='0x1572'>Ethernet Controller X710</product>
    <driver><name>i40e</name></driver>
  </capability>
</device>`)
	if err != nil {
		t.Fatalf("parseHostDeviceXML() error = %v", err)
	}
	if device.Name != "pci_0000_00_02_0" || device.Type != "pci" || device.Parent != "computer" {
		t.Fatalf("identity = %+v", device)
	}
	if device.Address != "0000:00:02.0" || device.Driver != "i40e" {
		t.Fatalf("pci details = %+v", device)
	}
	if device.Vendor != "Intel Corporation" || device.VendorID != "0x8086" || device.Product != "Ethernet Controller X710" || device.ProductID != "0x1572" {
		t.Fatalf("vendor/product = %+v", device)
	}
}

func TestParseHostDeviceXMLAddresses(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		typ  string
		want string
	}{
		{
			name: "usb", typ: "usb_device", want: "bus 1 · device 4",
			raw: `<device><name>usb_device_046d_c52b_1_4</name><capability type='usb_device'><bus>1</bus><device>4</device><vendor id='0x046d'>Logitech</vendor><product id='0xc52b'>Receiver</product></capability></device>`,
		},
		{
			name: "network", typ: "net", want: "enp2s0",
			raw: `<device><name>net_enp2s0</name><capability type='net'><interface>enp2s0</interface></capability></device>`,
		},
		{
			name: "storage", typ: "storage", want: "/dev/sda",
			raw: `<device><name>block_sda</name><capability type='storage'><block>/dev/sda</block></capability></device>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			device, err := parseHostDeviceXML(test.raw)
			if err != nil {
				t.Fatalf("parseHostDeviceXML() error = %v", err)
			}
			if device.Type != test.typ || device.Address != test.want {
				t.Fatalf("device = %+v, want type %q address %q", device, test.typ, test.want)
			}
		})
	}
}

func TestParseHostDeviceXMLRejectsMalformedXML(t *testing.T) {
	if _, err := parseHostDeviceXML(`<device><name>broken</device>`); err == nil {
		t.Fatal("parseHostDeviceXML() error = nil, want parse failure")
	}
}

func TestNodeHexNormalizesPCIAddressParts(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{input: "0x0000", width: 4, want: "0000"},
		{input: "0x1f", width: 2, want: "1f"},
		{input: "7", width: 2, want: "07"},
		{input: "", width: 2, want: ""},
	}
	for _, test := range tests {
		if got := nodeHex(test.input, test.width); got != test.want {
			t.Fatalf("nodeHex(%q, %d) = %q, want %q", test.input, test.width, got, test.want)
		}
	}
}
