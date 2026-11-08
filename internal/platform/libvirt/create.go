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
	if existing, err := c.conn.LookupDomainByName(spec.Name); err == nil {
		_ = existing.Free()
		return domain.VM{}, domain.ErrConflict
	} else if !errors.Is(err, libvirt.ERR_NO_DOMAIN) {
		return domain.VM{}, fmt.Errorf("check existing domain %s: %w", spec.Name, err)
	}
	var seed *cloudInitSeed
	var err error
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

	dom, err := c.conn.DomainDefineXML(buildDomainXMLWithCloudInit(spec, seedPath))
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
	var builder strings.Builder
	builder.WriteString("<domain type='kvm'>")
	fmt.Fprintf(&builder, "<name>%s</name>", escapeXML(spec.Name))
	fmt.Fprintf(&builder, "<memory unit='MiB'>%d</memory>", spec.MemoryMiB)
	fmt.Fprintf(&builder, "<vcpu>%d</vcpu>", spec.VCPUs)
	if firmware == domain.VMFirmwareUEFI {
		builder.WriteString("<os firmware='efi'><type arch='x86_64' machine='q35'>hvm</type><loader secure='")
		if spec.SecureBoot {
			builder.WriteString("yes")
		} else {
			builder.WriteString("no")
		}
		builder.WriteString("'/>")
	} else {
		builder.WriteString("<os><type arch='x86_64'>hvm</type>")
	}
	builder.WriteString("</os><features><acpi/><apic/></features><devices>")
	diskBootOrder := "2"
	if spec.ISOPath == "" {
		diskBootOrder = "1"
	}
	fmt.Fprintf(&builder, "<disk type='file' device='disk'><driver name='qemu' type='%s'/><source file='%s'/><target dev='vda' bus='virtio'/><boot order='%s'/></disk>",
		escapeXML(spec.DiskFormat), escapeXML(spec.DiskPath), diskBootOrder)
	if spec.ISOPath != "" {
		fmt.Fprintf(&builder, "<disk type='file' device='cdrom'><driver name='qemu' type='raw'/><source file='%s'/><target dev='sda' bus='sata'/><readonly/><boot order='1'/></disk>", escapeXML(spec.ISOPath))
	}
	if cloudInitPath != "" {
		fmt.Fprintf(&builder, "<disk type='file' device='cdrom'><driver name='qemu' type='raw'/><source file='%s'/><target dev='sdb' bus='sata'/><readonly/><serial>cloud-init</serial></disk>", escapeXML(cloudInitPath))
	}
	fmt.Fprintf(&builder, "<interface type='network'><source network='%s'/><model type='virtio'/></interface>", escapeXML(spec.Network))
	builder.WriteString("<graphics type='vnc' listen='127.0.0.1' autoport='yes'/><video><model type='virtio'/></video>")
	builder.WriteString("</devices></domain>")
	return builder.String()
}

func escapeXML(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}
