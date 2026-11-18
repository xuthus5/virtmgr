package libvirtbackend

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
)

type domainHostDevSource struct {
	Address struct {
		Domain   string `xml:"domain,attr"`
		Bus      string `xml:"bus,attr"`
		Slot     string `xml:"slot,attr"`
		Function string `xml:"function,attr"`
		Device   string `xml:"device,attr"`
	} `xml:"address"`
	Vendor struct {
		ID string `xml:"id,attr"`
	} `xml:"vendor"`
	Product struct {
		ID string `xml:"id,attr"`
	} `xml:"product"`
}

type domainSerialXML struct {
	Type   string `xml:"type,attr"`
	Target struct {
		Type string `xml:"type,attr"`
		Port int    `xml:"port,attr"`
	} `xml:"target"`
	Alias struct {
		Name string `xml:"name,attr"`
	} `xml:"alias"`
}

type domainDiskXML struct {
	Type   string `xml:"type,attr"`
	Device string `xml:"device,attr"`
	Driver struct {
		Type string `xml:"type,attr"`
	} `xml:"driver"`
	Source struct {
		File   string `xml:"file,attr"`
		Dev    string `xml:"dev,attr"`
		Volume string `xml:"volume,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
		Bus string `xml:"bus,attr"`
	} `xml:"target"`
	Serial   string    `xml:"serial"`
	ReadOnly *struct{} `xml:"readonly"`
	Boot     struct {
		Order int `xml:"order,attr"`
	} `xml:"boot"`
	IoTune struct {
		TotalBytesSec uint64 `xml:"total_bytes_sec"`
		ReadBytesSec  uint64 `xml:"read_bytes_sec"`
		WriteBytesSec uint64 `xml:"write_bytes_sec"`
		TotalIopsSec  uint64 `xml:"total_iops_sec"`
		ReadIopsSec   uint64 `xml:"read_iops_sec"`
		WriteIopsSec  uint64 `xml:"write_iops_sec"`
	} `xml:"iotune"`
}

type domainInterfaceXML struct {
	Type string `xml:"type,attr"`
	MAC  struct {
		Address string `xml:"address,attr"`
	} `xml:"mac"`
	Source struct {
		Network string `xml:"network,attr"`
		Bridge  string `xml:"bridge,attr"`
		Dev     string `xml:"dev,attr"`
	} `xml:"source"`
	Model struct {
		Type string `xml:"type,attr"`
	} `xml:"model"`
	Address struct {
		Type string `xml:"type,attr"`
	} `xml:"address"`
	Bandwidth struct {
		Inbound  domainBandwidthXML `xml:"inbound"`
		Outbound domainBandwidthXML `xml:"outbound"`
	} `xml:"bandwidth"`
	Boot struct {
		Order int `xml:"order,attr"`
	} `xml:"boot"`
}

type domainXML struct {
	Memory struct {
		Unit  string `xml:"unit,attr"`
		Value uint64 `xml:",chardata"`
	} `xml:"memory"`
	VCPU struct {
		Current uint `xml:"current,attr"`
		Value   uint `xml:",chardata"`
	} `xml:"vcpu"`
	OS struct {
		Type struct {
			Arch    string `xml:"arch,attr"`
			Machine string `xml:"machine,attr"`
			Value   string `xml:",chardata"`
		} `xml:"type"`
		Firmware string `xml:"firmware,attr"`
		Loader   struct {
			Readonly string `xml:"readonly,attr"`
			Secure   string `xml:"secure,attr"`
			Type     string `xml:"type,attr"`
			Value    string `xml:",chardata"`
		} `xml:"loader"`
		NVRAM struct {
			Template string `xml:"template,attr"`
			Value    string `xml:",chardata"`
		} `xml:"nvram"`
		FirmwareFeatures []struct {
			Enabled string `xml:"enabled,attr"`
			Name    string `xml:"name,attr"`
		} `xml:"firmware>feature"`
		Boot []struct {
			Dev string `xml:"dev,attr"`
		} `xml:"boot"`
	} `xml:"os"`
	CPU struct {
		Mode  string `xml:"mode,attr"`
		Model struct {
			Value string `xml:",chardata"`
		} `xml:"model"`
		Topology struct {
			Sockets uint `xml:"sockets,attr"`
			Cores   uint `xml:"cores,attr"`
			Threads uint `xml:"threads,attr"`
		} `xml:"topology"`
	} `xml:"cpu"`
	Devices struct {
		Controllers []struct {
			Type  string `xml:"type,attr"`
			Index int    `xml:"index,attr"`
		} `xml:"controller"`
		Channels []struct {
			Type   string `xml:"type,attr"`
			Target struct {
				Type string `xml:"type,attr"`
				Name string `xml:"name,attr"`
			} `xml:"target"`
		} `xml:"channel"`
		Disks      []domainDiskXML      `xml:"disk"`
		Interfaces []domainInterfaceXML `xml:"interface"`
		Graphics   []struct {
			Type   string `xml:"type,attr"`
			Port   int    `xml:"port,attr"`
			Listen string `xml:"listen,attr"`
		} `xml:"graphics"`
		Serials     []domainSerialXML `xml:"serial"`
		Consoles    []domainSerialXML `xml:"console"`
		HostDevices []struct {
			Mode   string              `xml:"mode,attr"`
			Type   string              `xml:"type,attr"`
			Source domainHostDevSource `xml:"source"`
		} `xml:"hostdev"`
	} `xml:"devices"`
}

type domainBandwidthXML struct {
	Average uint64 `xml:"average,attr"`
	Peak    uint64 `xml:"peak,attr"`
	Burst   uint64 `xml:"burst,attr"`
}

type domainDetails struct {
	OSType      string
	Arch        string
	Machine     string
	VCPUs       uint
	MemoryBytes uint64
	Firmware    domain.VMFirmware
	SecureBoot  bool
	CloudInit   bool
	GuestAgent  bool
	Disks       []domain.Disk
	Interfaces  []domain.NetworkInterface
	Graphics    []domain.Graphics
	Serials     []domain.SerialConsole
	Passthrough []domain.PassthroughDevice
	BootDevices []domain.VMBootDevice
	CPUModel    string
	CPUSockets  uint
	CPUCores    uint
	CPUThreads  uint
}

func parseDomainXML(text string) (domainDetails, error) {
	var parsed domainXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		return domainDetails{}, fmt.Errorf("decode domain xml: %w", err)
	}
	firmware, secureBoot := firmwareFromDomainXML(parsed.OS.Firmware, parsed.OS.Loader.Type, parsed.OS.Loader.Secure, parsed.OS.NVRAM.Template, parsed.OS.NVRAM.Value)
	for _, feature := range parsed.OS.FirmwareFeatures {
		if strings.EqualFold(strings.TrimSpace(feature.Name), "secure-boot") && xmlBoolean(feature.Enabled) {
			secureBoot = true
			break
		}
	}
	details := domainDetails{
		OSType:      strings.TrimSpace(parsed.OS.Type.Value),
		Arch:        parsed.OS.Type.Arch,
		Machine:     parsed.OS.Type.Machine,
		VCPUs:       parsed.VCPU.Value,
		MemoryBytes: domainMemoryBytes(parsed.Memory.Unit, parsed.Memory.Value),
		Firmware:    firmware,
		SecureBoot:  secureBoot,
		Disks:       make([]domain.Disk, 0, len(parsed.Devices.Disks)),
		GuestAgent:  hasGuestAgentChannelXML(text),
		BootDevices: make([]domain.VMBootDevice, 0, len(parsed.OS.Boot)),
		CPUModel:    cpuModelFromXML(parsed.CPU.Mode, parsed.CPU.Model.Value),
		CPUSockets:  parsed.CPU.Topology.Sockets,
		CPUCores:    parsed.CPU.Topology.Cores,
		CPUThreads:  parsed.CPU.Topology.Threads,
	}
	for _, boot := range parsed.OS.Boot {
		if device, ok := domain.NormalizeVMBootDevices([]string{boot.Dev}); ok {
			details.BootDevices = append(details.BootDevices, device...)
		}
	}
	if len(details.BootDevices) == 0 {
		details.BootDevices = bootDevicesFromXML(parsed.Devices.Disks, parsed.Devices.Interfaces)
	}
	for _, disk := range parsed.Devices.Disks {
		source := firstNonEmpty(disk.Source.File, disk.Source.Dev, disk.Source.Volume)
		if strings.EqualFold(strings.TrimSpace(disk.Serial), "cloud-init") {
			details.CloudInit = true
		}
		item := domain.Disk{
			Type:     disk.Type,
			Device:   disk.Device,
			Target:   disk.Target.Dev,
			Bus:      disk.Target.Bus,
			Format:   disk.Driver.Type,
			Source:   source,
			ReadOnly: disk.ReadOnly != nil,
		}
		if disk.IoTune.TotalBytesSec > 0 || disk.IoTune.ReadBytesSec > 0 || disk.IoTune.WriteBytesSec > 0 ||
			disk.IoTune.TotalIopsSec > 0 || disk.IoTune.ReadIopsSec > 0 || disk.IoTune.WriteIopsSec > 0 {
			item.QoS = &domain.DiskQoS{
				TotalBytesSec: disk.IoTune.TotalBytesSec, ReadBytesSec: disk.IoTune.ReadBytesSec, WriteBytesSec: disk.IoTune.WriteBytesSec,
				TotalIOPS: disk.IoTune.TotalIopsSec, ReadIOPS: disk.IoTune.ReadIopsSec, WriteIOPS: disk.IoTune.WriteIopsSec,
			}
		}
		details.Disks = append(details.Disks, item)
	}
	details.Interfaces = make([]domain.NetworkInterface, 0, len(parsed.Devices.Interfaces))
	for _, iface := range parsed.Devices.Interfaces {
		item := domain.NetworkInterface{
			Type:    iface.Type,
			MAC:     iface.MAC.Address,
			Source:  firstNonEmpty(iface.Source.Network, iface.Source.Bridge, iface.Source.Dev),
			Model:   iface.Model.Type,
			Address: iface.Address.Type,
		}
		if iface.Bandwidth.Inbound.Average > 0 || iface.Bandwidth.Inbound.Peak > 0 || iface.Bandwidth.Inbound.Burst > 0 ||
			iface.Bandwidth.Outbound.Average > 0 || iface.Bandwidth.Outbound.Peak > 0 || iface.Bandwidth.Outbound.Burst > 0 {
			item.QoS = &domain.NetworkQoS{
				InboundAverageBytes: iface.Bandwidth.Inbound.Average, InboundPeakBytes: iface.Bandwidth.Inbound.Peak,
				InboundBurstBytes: iface.Bandwidth.Inbound.Burst, OutboundAverageBytes: iface.Bandwidth.Outbound.Average,
				OutboundPeakBytes: iface.Bandwidth.Outbound.Peak, OutboundBurstBytes: iface.Bandwidth.Outbound.Burst,
			}
		}
		details.Interfaces = append(details.Interfaces, item)
	}
	details.Graphics = make([]domain.Graphics, 0, len(parsed.Devices.Graphics))
	for _, graphics := range parsed.Devices.Graphics {
		details.Graphics = append(details.Graphics, domain.Graphics{
			Type:   graphics.Type,
			Listen: graphics.Listen,
			Port:   graphics.Port,
		})
	}
	details.Serials = serialConsolesFromXML(parsed.Devices.Consoles, parsed.Devices.Serials)
	details.Passthrough = make([]domain.PassthroughDevice, 0, len(parsed.Devices.HostDevices))
	for _, hostdev := range parsed.Devices.HostDevices {
		device, ok := passthroughFromHostDevXML(hostdev.Mode, hostdev.Type, hostdev.Source)
		if ok {
			details.Passthrough = append(details.Passthrough, device)
		}
	}
	return details, nil
}

func domainMemoryBytes(unit string, value uint64) uint64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "b", "bytes":
		return value
	case "kb", "kib", "k":
		return value << 10
	case "mb", "mib", "m":
		return value << 20
	case "gb", "gib", "g":
		return value << 30
	case "tb", "tib", "t":
		return value << 40
	default:
		return value << 10
	}
}

func cpuModelFromXML(mode, model string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "host-passthrough", "host-model":
		return mode
	default:
		return strings.TrimSpace(model)
	}
}

type bootDeviceOrder struct {
	order  int
	device domain.VMBootDevice
}

func bootDevicesFromXML(disks []domainDiskXML, interfaces []domainInterfaceXML) []domain.VMBootDevice {
	ordered := make([]bootDeviceOrder, 0)
	seen := make(map[domain.VMBootDevice]struct{})
	appendDevice := func(order int, device domain.VMBootDevice) {
		if order <= 0 {
			return
		}
		if _, exists := seen[device]; exists {
			return
		}
		seen[device] = struct{}{}
		ordered = append(ordered, bootDeviceOrder{order: order, device: device})
	}
	for _, disk := range disks {
		device := domain.VMBootDeviceDisk
		if strings.EqualFold(strings.TrimSpace(disk.Device), "cdrom") {
			device = domain.VMBootDeviceCDROM
		}
		appendDevice(disk.Boot.Order, device)
	}
	for _, networkInterface := range interfaces {
		appendDevice(networkInterface.Boot.Order, domain.VMBootDeviceNetwork)
	}
	slices.SortStableFunc(ordered, func(left, right bootDeviceOrder) int {
		return cmp.Compare(left.order, right.order)
	})
	result := make([]domain.VMBootDevice, 0, len(ordered))
	for _, item := range ordered {
		result = append(result, item.device)
	}
	return result
}

func serialConsolesFromXML(consoles, serials []domainSerialXML) []domain.SerialConsole {
	devices := make([]domain.SerialConsole, 0, len(consoles)+len(serials))
	aliasesByPort := make(map[int]string, len(serials))
	for _, serial := range serials {
		if strings.EqualFold(strings.TrimSpace(serial.Type), "pty") && strings.TrimSpace(serial.Alias.Name) != "" {
			aliasesByPort[serial.Target.Port] = strings.TrimSpace(serial.Alias.Name)
		}
	}
	seen := make(map[string]struct{}, len(consoles)+len(serials))
	appendDevice := func(device domainSerialXML) {
		if !strings.EqualFold(strings.TrimSpace(device.Type), "pty") {
			return
		}
		alias := strings.TrimSpace(device.Alias.Name)
		if alias == "" {
			alias = aliasesByPort[device.Target.Port]
		}
		key := alias
		if key == "" {
			key = fmt.Sprintf("%s:%d", strings.TrimSpace(device.Target.Type), device.Target.Port)
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		labelIndex := device.Target.Port + 1
		if labelIndex < 1 {
			labelIndex = len(devices) + 1
		}
		devices = append(devices, domain.SerialConsole{
			Device:     alias,
			Label:      fmt.Sprintf("Serial %d", labelIndex),
			TargetPort: device.Target.Port,
			TargetType: strings.TrimSpace(device.Target.Type),
		})
	}
	for _, console := range consoles {
		appendDevice(console)
	}
	for _, serial := range serials {
		appendDevice(serial)
	}
	return devices
}

func firmwareFromDomainXML(firmware, loaderType, loaderSecure, nvramTemplate, nvramPath string) (domain.VMFirmware, bool) {
	secureBoot := xmlBoolean(loaderSecure)
	isUEFI := strings.EqualFold(strings.TrimSpace(firmware), "efi") ||
		strings.EqualFold(strings.TrimSpace(firmware), "uefi") ||
		strings.EqualFold(strings.TrimSpace(loaderType), "pflash") ||
		strings.TrimSpace(nvramTemplate) != "" || strings.TrimSpace(nvramPath) != "" || secureBoot
	if isUEFI {
		return domain.VMFirmwareUEFI, secureBoot
	}
	return domain.VMFirmwareBIOS, false
}

func xmlBoolean(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "1", "on":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseMemoryBytes(value string, unit string) (uint64, error) {
	number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(unit) {
	case "", "b", "bytes":
		return number, nil
	case "kib", "kb":
		return number * 1024, nil
	case "mib", "mb":
		return number * 1024 * 1024, nil
	case "gib", "gb":
		return number * 1024 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("unsupported memory unit %q", unit)
	}
}
