package libvirtbackend

import (
	"testing"

	"virtmgr/internal/domain"
)

func TestParseDomainXMLConfigurationAndQoS(t *testing.T) {
	details, err := parseDomainXML(`<domain type='kvm'>
  <name>configured-vm</name>
  <memory unit='MiB'>2048</memory>
  <vcpu current='3'>4</vcpu>
  <os><type arch='x86_64' machine='pc-q35-10.2'>hvm</type><boot dev='cdrom'/><boot dev='hd'/></os>
  <cpu mode='custom' match='exact'><model fallback='allow'>Skylake-Client</model><topology sockets='1' cores='4' threads='2'/></cpu>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/><source file='/var/lib/libvirt/images/demo.qcow2'/><target dev='vda' bus='virtio'/>
      <iotune><total_bytes_sec>10485760</total_bytes_sec><read_iops_sec>500</read_iops_sec><write_iops_sec>250</write_iops_sec></iotune>
    </disk>
    <interface type='network'>
      <mac address='52:54:00:01:12:99'/><source network='default'/><model type='virtio'/>
      <bandwidth><inbound average='10485760' peak='20971520' burst='1048576'/><outbound average='5242880'/></bandwidth>
    </interface>
  </devices>
</domain>`)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if len(details.BootDevices) != 2 || details.BootDevices[0] != domain.VMBootDeviceCDROM || details.BootDevices[1] != domain.VMBootDeviceDisk {
		t.Fatalf("boot devices = %+v", details.BootDevices)
	}
	if details.CPUModel != "Skylake-Client" || details.CPUSockets != 1 || details.CPUCores != 4 || details.CPUThreads != 2 {
		t.Fatalf("cpu = %q %d/%d/%d", details.CPUModel, details.CPUSockets, details.CPUCores, details.CPUThreads)
	}
	if details.VCPUs != 4 || details.MemoryBytes != 2<<30 {
		t.Fatalf("resources = %d vCPU / %d bytes", details.VCPUs, details.MemoryBytes)
	}
	if len(details.Disks) != 1 || details.Disks[0].QoS == nil || details.Disks[0].QoS.TotalBytesSec != 10485760 || details.Disks[0].QoS.ReadIOPS != 500 || details.Disks[0].QoS.WriteIOPS != 250 {
		t.Fatalf("disk QoS = %+v", details.Disks)
	}
	if len(details.Interfaces) != 1 || details.Interfaces[0].QoS == nil || details.Interfaces[0].QoS.InboundPeakBytes != 20971520 || details.Interfaces[0].QoS.OutboundAverageBytes != 5242880 {
		t.Fatalf("network QoS = %+v", details.Interfaces)
	}
}

func TestParseDomainXMLMemoryUnits(t *testing.T) {
	for _, test := range []struct {
		unit  string
		value uint64
		want  uint64
	}{
		{unit: "KiB", value: 1024, want: 1 << 20},
		{unit: "MiB", value: 2, want: 2 << 20},
		{unit: "GiB", value: 3, want: 3 << 30},
		{unit: "", value: 4, want: 4 << 10},
	} {
		if got := domainMemoryBytes(test.unit, test.value); got != test.want {
			t.Fatalf("domainMemoryBytes(%q, %d) = %d, want %d", test.unit, test.value, got, test.want)
		}
	}
}

func TestParseDomainXMLQuotaRNGAndWatchdog(t *testing.T) {
	details, err := parseDomainXML(`<domain>
  <memory unit='MiB'>2048</memory>
  <memtune><hard_limit unit='MiB'>4096</hard_limit><soft_limit unit='MiB'>3072</soft_limit><min_guarantee unit='MiB'>2048</min_guarantee></memtune>
  <vcpu>2</vcpu>
  <cputune><shares>2048</shares><period>100000</period><quota>50000</quota><vcpupin vcpu='0' cpuset='0'/></cputune>
  <devices>
    <rng model='virtio'><backend model='random'>/dev/urandom</backend></rng>
    <watchdog model='i6300esb' action='reset'/>
  </devices>
</domain>`)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if !details.RNG || details.Watchdog == nil || details.Watchdog.Model != domain.VMWatchdogModelI6300ESB || details.Watchdog.Action != domain.VMWatchdogActionReset {
		t.Fatalf("hardware = RNG %v watchdog %+v", details.RNG, details.Watchdog)
	}
	if details.ResourceQuota == nil || details.ResourceQuota.CPUShares != 2048 || details.ResourceQuota.CPUQuota != 50_000 || details.ResourceQuota.MemoryHardLimitMiB != 4096 {
		t.Fatalf("quota = %+v", details.ResourceQuota)
	}
	if details.Placement == nil || len(details.Placement.VCPUPins) != 1 {
		t.Fatalf("placement = %+v", details.Placement)
	}
}

func TestParseDomainXMLPreservesCPUAndDeviceBootSemantics(t *testing.T) {
	details, err := parseDomainXML(`<domain type='kvm'>
  <os><type arch='x86_64'>hvm</type></os>
  <cpu mode='host-passthrough' check='none'><topology sockets='2' cores='2' threads='1'/></cpu>
  <devices>
    <disk type='file' device='disk'><source file='/images/data.qcow2'/><target dev='vda'/><boot order='2'/></disk>
    <disk type='file' device='cdrom'><source file='/images/install.iso'/><target dev='sda'/><readonly/><boot order='1'/></disk>
    <interface type='network'><mac address='52:54:00:01:12:99'/><boot order='3'/></interface>
  </devices>
</domain>`)
	if err != nil {
		t.Fatalf("parseDomainXML() error = %v", err)
	}
	if details.CPUModel != "host-passthrough" || details.CPUSockets != 2 || details.CPUCores != 2 || details.CPUThreads != 1 {
		t.Fatalf("cpu = %q %d/%d/%d", details.CPUModel, details.CPUSockets, details.CPUCores, details.CPUThreads)
	}
	want := []domain.VMBootDevice{domain.VMBootDeviceCDROM, domain.VMBootDeviceDisk, domain.VMBootDeviceNetwork}
	if len(details.BootDevices) != len(want) {
		t.Fatalf("boot devices = %+v, want %+v", details.BootDevices, want)
	}
	for index := range want {
		if details.BootDevices[index] != want[index] {
			t.Fatalf("boot devices = %+v, want %+v", details.BootDevices, want)
		}
	}

	hostModel, err := parseDomainXML(`<domain><os><type>hvm</type></os><cpu mode='host-model'><topology sockets='1' cores='1' threads='1'/></cpu></domain>`)
	if err != nil || hostModel.CPUModel != "host-model" {
		t.Fatalf("host-model CPU = %q, %v", hostModel.CPUModel, err)
	}
}
