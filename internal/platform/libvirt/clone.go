package libvirtbackend

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

type cloneDisk struct {
	source string
	format string
	target string
}

func (c *Client) CloneVM(ctx context.Context, uuid string, spec domain.VMCloneSpec, progress VMCloneProgress) (domain.VM, error) {
	if err := ctx.Err(); err != nil {
		return domain.VM{}, err
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return domain.VM{}, fmt.Errorf("connect libvirt %s for clone: %w", c.uri, err)
	}
	defer func() { _, _ = conn.Close() }()
	reportCloneProgress(progress, 3, "正在检查源虚拟机")
	source, err := conn.LookupDomainByUUIDString(uuid)
	if err != nil {
		return domain.VM{}, mapDomainLookupError(uuid, err)
	}
	defer func() { _ = source.Free() }()

	state, _, err := source.GetState()
	if err != nil {
		return domain.VM{}, fmt.Errorf("read source domain %s state: %w", uuid, err)
	}
	if state != libvirt.DOMAIN_SHUTOFF && state != libvirt.DOMAIN_CRASHED {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	persistent, err := source.IsPersistent()
	if err != nil {
		return domain.VM{}, fmt.Errorf("check source domain %s persistence: %w", uuid, err)
	}
	if !persistent {
		return domain.VM{}, domain.ErrActionNotAllowed
	}
	if existing, lookupErr := conn.LookupDomainByName(spec.Name); lookupErr == nil {
		_ = existing.Free()
		return domain.VM{}, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_DOMAIN) {
		return domain.VM{}, fmt.Errorf("check clone domain name %s: %w", spec.Name, lookupErr)
	}

	sourceXML, err := source.GetXMLDesc(libvirt.DOMAIN_XML_INACTIVE)
	if err != nil {
		return domain.VM{}, fmt.Errorf("read source domain %s xml: %w", uuid, err)
	}
	disks, err := cloneDisksFromDomainXML(sourceXML)
	if err != nil {
		return domain.VM{}, err
	}

	replacedSources := make(map[string]string, len(disks))
	createdVolumes := make([]*libvirt.StorageVol, 0, len(disks))
	var targetDomain *libvirt.Domain
	committed := false
	defer func() {
		if targetDomain != nil {
			if !committed {
				_ = targetDomain.Undefine()
			}
			_ = targetDomain.Free()
		}
		for _, volume := range createdVolumes {
			if !committed {
				_ = volume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			}
			_ = volume.Free()
		}
	}()

	for index, disk := range disks {
		step := fmt.Sprintf("正在复制磁盘 %d/%d", index+1, len(disks))
		reportCloneProgress(progress, 10+index*65/maxInt(1, len(disks)), step)
		if err := ctx.Err(); err != nil {
			return domain.VM{}, err
		}
		targetPath, targetVolume, cloneErr := cloneDiskVolume(conn, disk, spec, index)
		if cloneErr != nil {
			return domain.VM{}, cloneErr
		}
		createdVolumes = append(createdVolumes, targetVolume)
		replacedSources[disk.source] = targetPath
	}

	reportCloneProgress(progress, 82, "正在定义克隆域")
	targetXML, err := rewriteCloneDomainXML(sourceXML, spec.Name, replacedSources)
	if err != nil {
		return domain.VM{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.VM{}, err
	}
	targetDomain, err = conn.DomainDefineXML(targetXML)
	if err != nil {
		return domain.VM{}, fmt.Errorf("define cloned domain %s: %w", spec.Name, err)
	}
	if spec.Autostart {
		reportCloneProgress(progress, 90, "正在设置开机自启动")
		if err := targetDomain.SetAutostart(true); err != nil {
			return domain.VM{}, fmt.Errorf("set cloned domain %s autostart: %w", spec.Name, err)
		}
	}
	if spec.Start {
		reportCloneProgress(progress, 95, "正在启动克隆虚拟机")
		if err := targetDomain.Create(); err != nil {
			return domain.VM{}, fmt.Errorf("start cloned domain %s: %w", spec.Name, err)
		}
	}
	vm, err := domainFromLibvirt(targetDomain)
	if err != nil {
		return domain.VM{}, err
	}
	c.populateDiskStorageInfoLocked(&vm)
	committed = true
	reportCloneProgress(progress, 100, "克隆完成")
	return vm, nil
}

func cloneDiskVolume(conn *libvirt.Connect, disk cloneDisk, spec domain.VMCloneSpec, index int) (string, *libvirt.StorageVol, error) {
	sourceVolume, err := conn.LookupStorageVolByPath(disk.source)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
			return "", nil, fmt.Errorf("%w: disk %s is not managed by a libvirt storage pool", domain.ErrInvalidInput, disk.source)
		}
		return "", nil, fmt.Errorf("lookup source storage volume %s: %w", disk.source, err)
	}
	defer func() { _ = sourceVolume.Free() }()
	sourceInfo, err := sourceVolume.GetInfo()
	if err != nil {
		return "", nil, fmt.Errorf("read source storage volume %s info: %w", disk.source, err)
	}
	sourceName, err := sourceVolume.GetName()
	if err != nil {
		return "", nil, fmt.Errorf("read source storage volume name: %w", err)
	}
	format, err := cloneDiskFormat(disk.format, sourceName)
	if err != nil {
		return "", nil, err
	}

	targetPoolName := strings.TrimSpace(spec.TargetPool)
	if targetPoolName == "" {
		sourcePool, poolErr := sourceVolume.LookupPoolByVolume()
		if poolErr != nil {
			return "", nil, fmt.Errorf("lookup storage pool for volume %s: %w", disk.source, poolErr)
		}
		targetPoolName, poolErr = sourcePool.GetName()
		_ = sourcePool.Free()
		if poolErr != nil {
			return "", nil, fmt.Errorf("read storage pool name for volume %s: %w", disk.source, poolErr)
		}
	}
	targetPool, err := conn.LookupStoragePoolByName(targetPoolName)
	if err != nil {
		if errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
			return "", nil, domain.ErrNotFound
		}
		return "", nil, fmt.Errorf("lookup target storage pool %s: %w", targetPoolName, err)
	}
	defer func() { _ = targetPool.Free() }()
	poolInfo, err := targetPool.GetInfo()
	if err != nil {
		return "", nil, fmt.Errorf("read target storage pool %s info: %w", targetPoolName, err)
	}
	if poolInfo.State != libvirt.STORAGE_POOL_RUNNING {
		return "", nil, domain.ErrUnavailable
	}

	targetName := cloneVolumeName(spec.Name, index, sourceName, format)
	if existing, lookupErr := targetPool.LookupStorageVolByName(targetName); lookupErr == nil {
		_ = existing.Free()
		return "", nil, domain.ErrConflict
	} else if !errors.Is(lookupErr, libvirt.ERR_NO_STORAGE_VOL) {
		return "", nil, fmt.Errorf("check target storage volume %s: %w", targetName, lookupErr)
	}
	volumeXML := buildCloneVolumeXML(targetName, sourceInfo.Capacity, format)
	targetVolume, err := targetPool.StorageVolCreateXMLFrom(volumeXML, sourceVolume, 0)
	if err != nil {
		if errors.Is(err, libvirt.ERR_STORAGE_VOL_EXIST) {
			return "", nil, domain.ErrConflict
		}
		return "", nil, fmt.Errorf("clone storage volume %s to %s/%s: %w", sourceName, targetPoolName, targetName, err)
	}
	targetPath, err := targetVolume.GetPath()
	if err != nil {
		_ = targetVolume.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		_ = targetVolume.Free()
		return "", nil, fmt.Errorf("read cloned storage volume %s path: %w", targetName, err)
	}
	return targetPath, targetVolume, nil
}

func cloneDisksFromDomainXML(text string) ([]cloneDisk, error) {
	var parsed domainXML
	if err := xml.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("decode source domain xml for clone: %w", err)
	}
	result := make([]cloneDisk, 0, len(parsed.Devices.Disks))
	for _, disk := range parsed.Devices.Disks {
		if disk.Type != "file" || disk.Device != "disk" || disk.ReadOnly != nil {
			continue
		}
		source := strings.TrimSpace(disk.Source.File)
		if source == "" {
			continue
		}
		result = append(result, cloneDisk{source: source, format: disk.Driver.Type, target: disk.Target.Dev})
	}
	return result, nil
}

func cloneDiskFormat(format, sourceName string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		switch strings.ToLower(filepath.Ext(sourceName)) {
		case ".qcow2":
			format = "qcow2"
		case ".raw", ".img":
			format = "raw"
		}
	}
	if format != "qcow2" && format != "raw" {
		return "", fmt.Errorf("%w: clone only supports qcow2 or raw file disks", domain.ErrInvalidInput)
	}
	return format, nil
}

func cloneVolumeName(domainName string, index int, sourceName, format string) string {
	extension := strings.ToLower(filepath.Ext(filepath.Base(sourceName)))
	if extension == "" {
		extension = "." + format
	}
	suffix := fmt.Sprintf("-%d%s", index+1, extension)
	prefix := domainName
	if len(prefix)+len(suffix) > 127 {
		keep := 127 - len(suffix)
		if keep < 1 {
			keep = 1
		}
		prefix = prefix[:keep]
	}
	return prefix + suffix
}

func buildCloneVolumeXML(name string, capacity uint64, format string) string {
	return fmt.Sprintf("<volume type='file'><name>%s</name><capacity unit='bytes'>%d</capacity><target><format type='%s'/></target></volume>",
		escapeXML(name), capacity, escapeXML(format))
}

func rewriteCloneDomainXML(sourceXML, name string, replacedSources map[string]string) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(sourceXML))
	var output strings.Builder
	encoder := xml.NewEncoder(&output)
	stack := make([]string, 0, 16)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read source domain xml for clone: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			local := element.Name.Local
			if parent == "domain" && (local == "uuid" || local == "genid") {
				if err := skipXMLElement(decoder, local); err != nil {
					return "", err
				}
				continue
			}
			if parent == "interface" && local == "mac" {
				if err := skipXMLElement(decoder, local); err != nil {
					return "", err
				}
				continue
			}
			if parent == "os" && local == "nvram" {
				if err := skipXMLElement(decoder, local); err != nil {
					return "", err
				}
				continue
			}
			if parent == "domain" && local == "name" {
				if err := encoder.EncodeToken(element); err != nil {
					return "", err
				}
				if err := skipXMLElement(decoder, local); err != nil {
					return "", err
				}
				if err := encoder.EncodeToken(xml.CharData(name)); err != nil {
					return "", err
				}
				if err := encoder.EncodeToken(xml.EndElement{Name: element.Name}); err != nil {
					return "", err
				}
				continue
			}
			if parent == "disk" && local == "source" {
				for i := range element.Attr {
					if element.Attr[i].Name.Local != "file" {
						continue
					}
					if replacement, ok := replacedSources[element.Attr[i].Value]; ok {
						element.Attr[i].Value = replacement
					}
				}
			}
			if parent == "devices" && local == "graphics" {
				element.Attr = resetGraphicsPort(element.Attr)
			}
			if err := encoder.EncodeToken(element); err != nil {
				return "", err
			}
			stack = append(stack, local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if err := encoder.EncodeToken(element); err != nil {
				return "", err
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return "", err
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", fmt.Errorf("encode cloned domain xml: %w", err)
	}
	return output.String(), nil
}

func skipXMLElement(decoder *xml.Decoder, local string) error {
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("read %s element in source domain xml: %w", local, err)
		}
		switch token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

func resetGraphicsPort(attributes []xml.Attr) []xml.Attr {
	result := make([]xml.Attr, 0, len(attributes)+1)
	hasAutoport := false
	for _, attribute := range attributes {
		switch attribute.Name.Local {
		case "port":
			continue
		case "autoport":
			attribute.Value = "yes"
			hasAutoport = true
		}
		result = append(result, attribute)
	}
	if !hasAutoport {
		result = append(result, xml.Attr{Name: xml.Name{Local: "autoport"}, Value: "yes"})
	}
	return result
}

func reportCloneProgress(progress VMCloneProgress, percent int, step string) {
	if progress != nil {
		progress(percent, step)
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
