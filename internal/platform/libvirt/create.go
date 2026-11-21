package libvirtbackend

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

func (c *Client) CreateVM(ctx context.Context, spec domain.VMCreateSpec) (domain.VM, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return domain.VM{}, err
	}
	platform := c.hostPlatformLocked(ctx)
	architecture, machine, err := resolveVMPlatform(platform, spec.Architecture, spec.Machine, spec.Firmware)
	if err != nil {
		return domain.VM{}, err
	}
	if spec.TPM && platform.known && !platform.capabilities.TPM {
		return domain.VM{}, fmt.Errorf("%w: vTPM 2.0 is not available on this host", domain.ErrInvalidInput)
	}
	if spec.Placement != nil {
		if err := validateHostPlacement(platform, *spec.Placement); err != nil {
			return domain.VM{}, err
		}
	}
	spec.Architecture = architecture
	spec.Machine = machine
	if existing, err := c.conn.LookupDomainByName(spec.Name); err == nil {
		_ = existing.Free()
		return domain.VM{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_DOMAIN) {
		return domain.VM{}, fmt.Errorf("check existing domain %s: %w", spec.Name, err)
	}
	var seed *cloudInitSeed
	seedCommitted := false
	if spec.CloudInit != nil {
		seed, err = c.createCloudInitSeedLocked(ctx, spec.Name, *spec.CloudInit)
		if err != nil {
			return domain.VM{}, err
		}
		defer func() {
			if seedCommitted {
				seed.free()
			} else {
				seed.remove()
			}
		}()
	}
	seedPath := ""
	if seed != nil {
		seedPath = seed.path
	}

	xmlText := buildDomainXMLWithCloudInit(spec, seedPath)
	if spec.Placement != nil {
		xmlText, err = domain.SetVMXMLPlacement(xmlText, *spec.Placement)
		if err != nil {
			return domain.VM{}, err
		}
	}
	dom, err := c.conn.DomainDefineXML(xmlText)
	if err != nil {
		return domain.VM{}, fmt.Errorf("define domain %s: %w", spec.Name, err)
	}
	domainActive := true
	started := false
	defer func() {
		if domainActive {
			if started {
				_ = dom.Destroy()
			}
			_ = dom.Undefine()
		}
		_ = dom.Free()
	}()
	if spec.Autostart {
		if err := dom.SetAutostart(true); err != nil {
			return domain.VM{}, fmt.Errorf("set domain %s autostart: %w", spec.Name, err)
		}
	}
	if spec.Start {
		if err := dom.Create(); err != nil {
			return domain.VM{}, fmt.Errorf("start domain %s: %w", spec.Name, err)
		}
		started = true
	}
	vm, err := domainFromLibvirt(dom)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	domainActive = false
	seedCommitted = true
	if spec.CloudInit != nil {
		vm.CloudInit = true
	}
	return vm, nil
}

func (c *Client) DeleteVM(ctx context.Context, uuid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureConnectionLocked(ctx); err != nil {
		return err
	}
	dom, err := c.conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return mapDomainLookupError(uuid, err)
	}
	defer func() { _ = dom.Free() }()
	state, _, err := dom.GetState()
	if err != nil {
		return fmt.Errorf("read domain %s state: %w", uuid, err)
	}
	if state != libvirt.DOMAIN_SHUTOFF && state != libvirt.DOMAIN_CRASHED {
		return domain.ErrActionNotAllowed
	}
	if err := dom.Undefine(); err != nil {
		return fmt.Errorf("undefine domain %s: %w", uuid, err)
	}
	return nil
}

func buildDomainXML(spec domain.VMCreateSpec) string {
	return buildDomainXMLWithCloudInit(spec, "")
}

func buildDomainXMLWithCloudInit(spec domain.VMCreateSpec, cloudInitPath string) string {
	firmware := spec.Firmware
	if firmware == "" {
		firmware = domain.VMFirmwareBIOS
	}
	architecture, ok := domain.NormalizeVMArchitecture(spec.Architecture)
	if !ok || architecture == "" {
		architecture = "x86_64"
	}
	machine := strings.TrimSpace(spec.Machine)
	if machine == "" {
		machine = domain.DefaultVMMachine(architecture, firmware)
	}
	machineAttribute := ""
	if machine != "" {
		machineAttribute = fmt.Sprintf(" machine='%s'", escapeXML(machine))
	}
	var builder strings.Builder
	builder.WriteString("<domain type='kvm'>")
	fmt.Fprintf(&builder, "<name>%s</name>", escapeXML(spec.Name))
	fmt.Fprintf(&builder, "<memory unit='MiB'>%d</memory>", spec.MemoryMiB)
	if spec.ResourceQuota != nil && (spec.ResourceQuota.MemoryHardLimitMiB > 0 || spec.ResourceQuota.MemorySoftLimitMiB > 0) {
		builder.WriteString("<memtune>")
		if spec.ResourceQuota.MemoryHardLimitMiB > 0 {
			fmt.Fprintf(&builder, "<hard_limit unit='MiB'>%d</hard_limit>", spec.ResourceQuota.MemoryHardLimitMiB)
		}
		if spec.ResourceQuota.MemorySoftLimitMiB > 0 {
			fmt.Fprintf(&builder, "<soft_limit unit='MiB'>%d</soft_limit>", spec.ResourceQuota.MemorySoftLimitMiB)
		}
		builder.WriteString("</memtune>")
	}
	fmt.Fprintf(&builder, "<vcpu>%d</vcpu>", spec.VCPUs)
	if spec.ResourceQuota != nil && (spec.ResourceQuota.CPUShares > 0 || spec.ResourceQuota.CPUQuota > 0) {
		builder.WriteString("<cputune>")
		if spec.ResourceQuota.CPUShares > 0 {
			fmt.Fprintf(&builder, "<shares>%d</shares>", spec.ResourceQuota.CPUShares)
		}
		if spec.ResourceQuota.CPUQuota > 0 {
			fmt.Fprintf(&builder, "<period>%d</period><quota>%d</quota>", spec.ResourceQuota.CPUPeriod, spec.ResourceQuota.CPUQuota)
		}
		builder.WriteString("</cputune>")
	}
	if firmware == domain.VMFirmwareUEFI {
		fmt.Fprintf(&builder, "<os firmware='efi'><type arch='%s'%s>hvm</type><loader secure='", escapeXML(architecture), machineAttribute)
		if spec.SecureBoot {
			builder.WriteString("yes")
		} else {
			builder.WriteString("no")
		}
		builder.WriteString("'/>")
	} else {
		fmt.Fprintf(&builder, "<os><type arch='%s'%s>hvm</type>", escapeXML(architecture), machineAttribute)
	}
	builder.WriteString("</os><features><acpi/><apic/></features><devices>")
	diskBus := spec.DiskBus
	if diskBus == "" {
		diskBus = domain.VMDiskBusVirtio
	}
	nicModel := spec.NICModel
	if nicModel == "" {
		nicModel = domain.VMNICModelVirtio
	}
	diskTarget, nextTarget := "vda", "sdb"
	if diskBus != domain.VMDiskBusVirtio {
		diskTarget = "sda"
	}
	diskBootOrder := "1"
	cdromBootOrder := "2"
	if spec.ISOPath != "" && spec.BootOrder != domain.VMBootOrderDiskFirst {
		diskBootOrder, cdromBootOrder = "2", "1"
	}
	if diskBus == domain.VMDiskBusSCSI {
		builder.WriteString("<controller type='scsi' index='0' model='virtio-scsi'/>")
	}
	fmt.Fprintf(&builder, "<disk type='file' device='disk'><driver name='qemu' type='%s'/><source file='%s'/><target dev='%s' bus='%s'/><boot order='%s'/></disk>",
		escapeXML(spec.DiskFormat), escapeXML(spec.DiskPath), diskTarget, diskBus, diskBootOrder)
	if spec.ISOPath != "" {
		fmt.Fprintf(&builder, "<disk type='file' device='cdrom'><driver name='qemu' type='raw'/><source file='%s'/><target dev='%s' bus='sata'/><readonly/><boot order='%s'/></disk>", escapeXML(spec.ISOPath), nextTarget, cdromBootOrder)
		nextTarget = nextSATAAndSCSITarget(nextTarget)
	}
	if cloudInitPath != "" {
		fmt.Fprintf(&builder, "<disk type='file' device='cdrom'><driver name='qemu' type='raw'/><source file='%s'/><target dev='%s' bus='sata'/><readonly/><serial>cloud-init</serial></disk>", escapeXML(cloudInitPath), nextTarget)
	}
	fmt.Fprintf(&builder, "<interface type='network'><source network='%s'/><model type='%s'/></interface>", escapeXML(spec.Network), nicModel)
	builder.WriteString("<graphics type='vnc' listen='127.0.0.1' autoport='yes'/><video><model type='virtio'/></video>")
	if spec.GuestAgentEnabled() {
		builder.WriteString("<controller type='virtio-serial' index='0'/><channel type='unix'><target type='virtio' name='org.qemu.guest_agent.0'/></channel>")
	}
	if spec.TPM {
		fmt.Fprintf(&builder, "<tpm model='%s'><backend type='emulator' version='2.0'/></tpm>", vmTPMModel(architecture))
	}
	if spec.RNG {
		builder.WriteString("<rng model='virtio'><backend model='random'>/dev/urandom</backend></rng>")
	}
	if spec.Watchdog != nil {
		fmt.Fprintf(&builder, "<watchdog model='%s' action='%s'/>", escapeXML(string(spec.Watchdog.Model)), escapeXML(string(spec.Watchdog.Action)))
	}
	builder.WriteString("</devices></domain>")
	return builder.String()
}

func vmTPMModel(architecture string) string {
	architecture, _ = domain.NormalizeVMArchitecture(architecture)
	if architecture == "x86_64" || architecture == "i686" {
		return "tpm-crb"
	}
	return "tpm-tis"
}

func nextSATAAndSCSITarget(current string) string {
	if current == "" || current[len(current)-1] == 'z' {
		return current
	}
	return current[:len(current)-1] + string(current[len(current)-1]+1)
}

func escapeXML(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}
