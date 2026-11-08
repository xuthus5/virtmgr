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
