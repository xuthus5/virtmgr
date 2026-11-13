package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"virtmgr/internal/domain"
)

const (
	maxTemplateDescriptionRunes = 512
	maxTemplateCloudInitBytes   = 64 << 10
	minTemplateMemoryMiB        = uint64(128)
	maxTemplateMemoryMiB        = uint64(1024 * 1024)
	maxTemplateVCPUs            = uint(255)
)

var (
	templateNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	templateNetworkPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
)

type Repository interface {
	ListVMTemplates(ctx context.Context) ([]domain.VMTemplate, error)
	GetVMTemplate(ctx context.Context, id int64) (domain.VMTemplate, error)
	VMTemplateByName(ctx context.Context, name string) (domain.VMTemplate, error)
	CreateVMTemplate(ctx context.Context, spec domain.VMTemplateSpec) (domain.VMTemplate, error)
	UpdateVMTemplate(ctx context.Context, id int64, spec domain.VMTemplateSpec) (domain.VMTemplate, error)
	DeleteVMTemplate(ctx context.Context, id int64) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type Service struct {
	repository   Repository
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) {
		service.readOnlyMode = mode
	}
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type CreateCommand struct {
	Spec    domain.VMTemplateSpec
	Request MutationContext
}

type UpdateCommand struct {
	ID      int64
	Spec    domain.VMTemplateSpec
	Request MutationContext
}

type DeleteCommand struct {
	ID      int64
	Request MutationContext
}

func New(repository Repository, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{repository: repository, audit: audit, readOnly: readOnly}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
}

func (s *Service) List(ctx context.Context) (domain.VMTemplateList, error) {
	templates, err := s.repository.ListVMTemplates(ctx)
	if err != nil {
		return domain.VMTemplateList{}, err
	}
	if templates == nil {
		templates = []domain.VMTemplate{}
	}
	return domain.VMTemplateList{Items: templates, Total: len(templates)}, nil
}

func (s *Service) Get(ctx context.Context, id int64) (domain.VMTemplate, error) {
	if id < 1 {
		return domain.VMTemplate{}, domain.ErrInvalidInput
	}
	return s.repository.GetVMTemplate(ctx, id)
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (domain.VMTemplate, error) {
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VMTemplate{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VMTemplate{}, domain.ErrReadOnly
	}
	spec, err := normalizeSpec(command.Spec)
	if err != nil {
		return domain.VMTemplate{}, err
	}
	if _, err := s.repository.VMTemplateByName(ctx, spec.Name); err == nil {
		return domain.VMTemplate{}, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.VMTemplate{}, err
	}
	template, err := s.repository.CreateVMTemplate(ctx, spec)
	if err != nil {
		return domain.VMTemplate{}, err
	}
	if err := s.recordAudit(ctx, command.Request, "vm_template.create", template.ID, templateDetail(template)); err != nil {
		return domain.VMTemplate{}, fmt.Errorf("record VM template creation audit: %w", err)
	}
	return template, nil
}

func (s *Service) Update(ctx context.Context, command UpdateCommand) (domain.VMTemplate, error) {
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VMTemplate{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VMTemplate{}, domain.ErrReadOnly
	}
	if command.ID < 1 {
		return domain.VMTemplate{}, domain.ErrInvalidInput
	}
	spec, err := normalizeSpec(command.Spec)
	if err != nil {
		return domain.VMTemplate{}, err
	}
	existing, err := s.repository.GetVMTemplate(ctx, command.ID)
	if err != nil {
		return domain.VMTemplate{}, err
	}
	if !strings.EqualFold(existing.Name, spec.Name) {
		if _, err := s.repository.VMTemplateByName(ctx, spec.Name); err == nil {
			return domain.VMTemplate{}, domain.ErrConflict
		} else if !errors.Is(err, domain.ErrNotFound) {
			return domain.VMTemplate{}, err
		}
	}
	template, err := s.repository.UpdateVMTemplate(ctx, command.ID, spec)
	if err != nil {
		return domain.VMTemplate{}, err
	}
	detail := templateDetail(template)
	detail["previousName"] = existing.Name
	if err := s.recordAudit(ctx, command.Request, "vm_template.update", template.ID, detail); err != nil {
		return domain.VMTemplate{}, fmt.Errorf("record VM template update audit: %w", err)
	}
	return template, nil
}

func (s *Service) Delete(ctx context.Context, command DeleteCommand) error {
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.ErrReadOnly
	}
	if command.ID < 1 {
		return domain.ErrInvalidInput
	}
	template, err := s.repository.GetVMTemplate(ctx, command.ID)
	if err != nil {
		return err
	}
	if err := s.repository.DeleteVMTemplate(ctx, command.ID); err != nil {
		return err
	}
	if err := s.recordAudit(ctx, command.Request, "vm_template.delete", template.ID, templateDetail(template)); err != nil {
		return fmt.Errorf("record VM template deletion audit: %w", err)
	}
	return nil
}

func normalizeSpec(spec domain.VMTemplateSpec) (domain.VMTemplateSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Description = strings.TrimSpace(spec.Description)
	spec.DiskFormat = strings.ToLower(strings.TrimSpace(spec.DiskFormat))
	spec.Network = strings.TrimSpace(spec.Network)
	spec.ISOPath = strings.TrimSpace(spec.ISOPath)
	spec.CloudInitUserData = strings.TrimSpace(spec.CloudInitUserData)
	spec.CloudInitNetworkConfig = strings.TrimSpace(spec.CloudInitNetworkConfig)
	spec.CloudInitVendorData = strings.TrimSpace(spec.CloudInitVendorData)
	if spec.DiskFormat == "" {
		spec.DiskFormat = "qcow2"
	}
	if spec.Network == "" {
		spec.Network = "default"
	}
	if !templateNamePattern.MatchString(spec.Name) {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: template name must use letters, digits, '.', '_' or '-'", domain.ErrInvalidInput)
	}
	if utf8.RuneCountInString(spec.Description) > maxTemplateDescriptionRunes {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: template description is too long", domain.ErrInvalidInput)
	}
	if spec.VCPUs == 0 || spec.VCPUs > maxTemplateVCPUs {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: vCPUs must be between 1 and %d", domain.ErrInvalidInput, maxTemplateVCPUs)
	}
	if spec.MemoryMiB < minTemplateMemoryMiB || spec.MemoryMiB > maxTemplateMemoryMiB {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: memory must be between %d and %d MiB", domain.ErrInvalidInput, minTemplateMemoryMiB, maxTemplateMemoryMiB)
	}
	if spec.DiskFormat != "qcow2" && spec.DiskFormat != "raw" {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: disk format must be qcow2 or raw", domain.ErrInvalidInput)
	}
	if !templateNetworkPattern.MatchString(spec.Network) {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: invalid template network name", domain.ErrInvalidInput)
	}
	if spec.ISOPath != "" {
		if strings.ContainsRune(spec.ISOPath, '\x00') || !filepath.IsAbs(spec.ISOPath) {
			return domain.VMTemplateSpec{}, fmt.Errorf("%w: template ISO path must be absolute", domain.ErrInvalidInput)
		}
		spec.ISOPath = filepath.Clean(spec.ISOPath)
	}
	firmware, ok := domain.NormalizeVMFirmware(string(spec.Firmware))
	if !ok {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: firmware must be bios or uefi", domain.ErrInvalidInput)
	}
	if spec.SecureBoot && firmware != domain.VMFirmwareUEFI {
		return domain.VMTemplateSpec{}, fmt.Errorf("%w: secure boot requires UEFI firmware", domain.ErrInvalidInput)
	}
	spec.Firmware = firmware
	if spec.CloudInit {
		total := len(spec.CloudInitUserData) + len(spec.CloudInitNetworkConfig) + len(spec.CloudInitVendorData)
		if spec.CloudInitUserData == "" || total > maxTemplateCloudInitBytes || containsNUL(spec.CloudInitUserData, spec.CloudInitNetworkConfig, spec.CloudInitVendorData) {
			return domain.VMTemplateSpec{}, fmt.Errorf("%w: cloud-init data must include user-data and be at most %d bytes in total with no NUL", domain.ErrInvalidInput, maxTemplateCloudInitBytes)
		}
	} else {
		spec.CloudInitUserData = ""
		spec.CloudInitNetworkConfig = ""
		spec.CloudInitVendorData = ""
	}
	return spec, nil
}

func templateDetail(template domain.VMTemplate) map[string]any {
	detail := map[string]any{
		"name":       template.Name,
		"vcpus":      template.VCPUs,
		"memoryMiB":  template.MemoryMiB,
		"diskFormat": template.DiskFormat,
		"network":    template.Network,
		"firmware":   template.Firmware,
		"secureBoot": template.SecureBoot,
		"cloudInit":  template.CloudInit,
		"autostart":  template.Autostart,
		"start":      template.Start,
	}
	if template.CloudInit {
		detail["cloudInitUserDataBytes"] = len(template.CloudInitUserData)
		detail["cloudInitNetworkConfigBytes"] = len(template.CloudInitNetworkConfig)
		detail["cloudInitVendorDataBytes"] = len(template.CloudInitVendorData)
	}
	if template.ISOPath != "" {
		detail["isoPath"] = template.ISOPath
	}
	return detail
}

func containsNUL(values ...string) bool {
	for _, value := range values {
		if strings.ContainsRune(value, '\x00') {
			return true
		}
	}
	return false
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action string, targetID int64, detail map[string]any) error {
	if s.audit == nil {
		return nil
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Now().UTC(),
		RequestID:  request.RequestID,
		Actor:      request.Session.User.Username,
		Role:       request.Session.User.Role,
		Action:     action,
		TargetKind: "vm_template",
		TargetID:   strconv.FormatInt(targetID, 10),
		SourceIP:   request.SourceIP,
		Result:     "success",
		Detail:     detail,
	})
}
