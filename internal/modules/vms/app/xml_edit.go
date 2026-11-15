package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"virtmgr/internal/domain"
)

const maxVMXMLBytes = 512 << 10

type XMLUpdateCommand struct {
	UUID    string
	XML     string
	Request MutationContext
}

type EjectISOCommand struct {
	UUID    string
	Request MutationContext
}

type AttachISOCommand struct {
	UUID    string
	Pool    string
	Volume  string
	Request MutationContext
}

func (s *Service) AttachISO(ctx context.Context, command AttachISOCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Pool = strings.TrimSpace(command.Pool)
	command.Volume = strings.TrimSpace(command.Volume)
	detail := map[string]any{"pool": command.Pool, "volume": command.Volume}
	if command.UUID == "" || !storagePoolNamePattern.MatchString(command.Pool) || !storageVolumeNamePattern.MatchString(command.Volume) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.isoVolumes == nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, domain.ErrUnavailable)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	detail["persistent"] = vm.Persistent
	if !vm.Persistent || (vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	volumes, err := s.isoVolumes.ListStorageVolumes(ctx, command.Pool)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, err)
	}
	var source string
	for _, volume := range volumes {
		if volume.Name != command.Volume {
			continue
		}
		source = strings.TrimSpace(volume.Path)
		break
	}
	if source == "" {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, domain.ErrNotFound)
	}
	if !strings.HasPrefix(source, "/") || !strings.HasSuffix(strings.ToLower(source), ".iso") {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail,
			fmt.Errorf("%w: selected volume must be an ISO file", domain.ErrInvalidInput))
	}
	current, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, err)
	}
	result, err := domain.AttachVMISOXML(current.XML, source)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, err)
	}
	detail["target"] = result.Target
	detail["source"] = result.Source
	detail["replaced"] = result.Replaced
	if _, err := s.repository.DefineVMXML(ctx, command.UUID, result.XML); err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.attach", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceISOAttach, 0, "挂载或替换安装 ISO 并切换为光盘优先启动")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.iso.attach", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM ISO attach audit: %w", err)
	}
	return updated, nil
}

func (s *Service) EjectISO(ctx context.Context, command EjectISOCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	detail := map[string]any{}
	if command.UUID == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	detail["persistent"] = vm.Persistent
	if !vm.Persistent || (vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	current, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, err)
	}
	result, err := domain.EjectVMISOXML(current.XML)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, err)
	}
	if !result.Ejected {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, fmt.Errorf("%w: no removable ISO CD-ROM is attached", domain.ErrActionNotAllowed))
	}
	detail["target"] = result.Target
	detail["source"] = result.Source
	if _, err := s.repository.DefineVMXML(ctx, command.UUID, result.XML); err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.iso.eject", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceISOEject, 0, "弹出安装 ISO 并切换到硬盘优先启动")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.iso.eject", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM ISO eject audit: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateXML(ctx context.Context, command XMLUpdateCommand) (domain.VMXML, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.XML = strings.TrimSpace(command.XML)
	detail := map[string]any{"xmlBytes": len(command.XML)}
	if command.UUID == "" || command.XML == "" {
		return domain.VMXML{}, domain.ErrInvalidInput
	}
	if len(command.XML) > maxVMXMLBytes {
		return s.xmlUpdateFailure(ctx, command, detail, domain.ErrTooLarge)
	}
	if !domain.CanEditVMXML(command.Request.Session.User.Role) {
		return s.xmlUpdateFailure(ctx, command, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.xmlUpdateFailure(ctx, command, detail, domain.ErrReadOnly)
	}

	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return s.xmlUpdateFailure(ctx, command, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	detail["persistent"] = vm.Persistent
	if !vm.Persistent {
		return s.xmlUpdateFailure(ctx, command, detail, domain.ErrActionNotAllowed)
	}

	identity, err := domain.ParseVMXMLIdentity(command.XML)
	if err != nil {
		return s.xmlUpdateFailure(ctx, command, detail, err)
	}
	if identity.UUID != vm.UUID || identity.Name != vm.Name {
		return s.xmlUpdateFailure(ctx, command, detail, fmt.Errorf("%w: domain name and UUID must match the existing VM", domain.ErrInvalidInput))
	}
	digest := sha256.Sum256([]byte(command.XML))
	detail["xmlSha256"] = hex.EncodeToString(digest[:])

	updated, err := s.repository.DefineVMXML(ctx, command.UUID, command.XML)
	if err != nil {
		return s.xmlUpdateFailure(ctx, command, detail, err)
	}
	if updated.Format != domain.VMXMLFormatConfig {
		updated.Format = domain.VMXMLFormatConfig
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceManualXML, 0, "管理员手动保存持久域 XML")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.xml.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VMXML{}, fmt.Errorf("record VM XML update audit: %w", err)
	}
	return updated, nil
}

func (s *Service) xmlUpdateFailure(ctx context.Context, command XMLUpdateCommand, detail map[string]any, cause error) (domain.VMXML, error) {
	if err := s.recordMutationAudit(ctx, command.Request, "vm.xml.update", command.UUID, "failure", detail, cause); err != nil {
		return domain.VMXML{}, fmt.Errorf("record VM XML update audit: %w", err)
	}
	return domain.VMXML{}, cause
}
