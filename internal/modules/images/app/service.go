package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"virtmgr/internal/domain"
)

const maxImageDescriptionRunes = 512

var (
	imageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_. -]{0,63}$`)
	imagePoolPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,63}$`)
)

type Repository interface {
	ListVMImages(ctx context.Context) ([]domain.VMImage, error)
	GetVMImage(ctx context.Context, id int64) (domain.VMImage, error)
	CreateVMImage(ctx context.Context, image domain.VMImage) (domain.VMImage, error)
	UpdateVMImageMetadata(ctx context.Context, image domain.VMImage) (domain.VMImage, error)
	DeleteVMImage(ctx context.Context, id int64) error
}

type VolumeRepository interface {
	ListStorageVolumes(ctx context.Context, poolName string) ([]domain.StorageVolume, error)
	StorageVolumeChecksum(ctx context.Context, poolName, volumeName string) (string, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type Service struct {
	repository   Repository
	volumes      VolumeRepository
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) { service.readOnlyMode = mode }
}

func New(repository Repository, volumes VolumeRepository, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{repository: repository, volumes: volumes, audit: audit, readOnly: readOnly}
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

func (s *Service) List(ctx context.Context) (domain.VMImageList, error) {
	items, err := s.repository.ListVMImages(ctx)
	if err != nil {
		return domain.VMImageList{}, err
	}
	if items == nil {
		items = []domain.VMImage{}
	}
	return domain.VMImageList{Items: items, Total: len(items)}, nil
}

func (s *Service) Get(ctx context.Context, id int64) (domain.VMImage, error) {
	if id < 1 {
		return domain.VMImage{}, domain.ErrInvalidInput
	}
	return s.repository.GetVMImage(ctx, id)
}

func (s *Service) Register(ctx context.Context, spec domain.VMImageCreateSpec, request MutationContext) (domain.VMImage, error) {
	if !domain.CanOperateVMs(request.Session.User.Role) {
		return domain.VMImage{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VMImage{}, domain.ErrReadOnly
	}
	spec = normalizeSpec(spec)
	if !imageNamePattern.MatchString(spec.Name) || !imagePoolPattern.MatchString(spec.Pool) || spec.Volume == "" {
		return domain.VMImage{}, domain.ErrInvalidInput
	}
	if !utf8.ValidString(spec.Description) || utf8.RuneCountInString(spec.Description) > maxImageDescriptionRunes || hasControl(spec.Description) {
		return domain.VMImage{}, domain.ErrInvalidInput
	}
	volumes, err := s.volumes.ListStorageVolumes(ctx, spec.Pool)
	if err != nil {
		return domain.VMImage{}, err
	}
	var source *domain.StorageVolume
	for index := range volumes {
		if volumes[index].Name == spec.Volume {
			source = &volumes[index]
			break
		}
	}
	if source == nil {
		return domain.VMImage{}, domain.ErrNotFound
	}
	format := strings.ToLower(strings.TrimSpace(source.Format))
	if format == "" {
		format = formatFromVolumeName(source.Name)
	}
	if format != "qcow2" && format != "raw" {
		return domain.VMImage{}, domain.ErrInvalidInput
	}
	image := domain.VMImage{
		Name: spec.Name, Description: spec.Description, Pool: spec.Pool, Volume: source.Name,
		Format: format, OSFamily: spec.OSFamily, OSName: spec.OSName, Architecture: spec.Architecture,
		CapacityBytes: source.CapacityBytes, AllocationBytes: source.AllocationBytes,
	}
	created, err := s.repository.CreateVMImage(ctx, image)
	if err != nil {
		return domain.VMImage{}, err
	}
	if err := s.recordAudit(ctx, request, "image.register", created.ID, imageDetail(created), nil); err != nil {
		return domain.VMImage{}, fmt.Errorf("record image register audit: %w", err)
	}
	return created, nil
}

func (s *Service) Refresh(ctx context.Context, id int64, request MutationContext) (domain.VMImage, error) {
	if !domain.CanOperateVMs(request.Session.User.Role) {
		return domain.VMImage{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VMImage{}, domain.ErrReadOnly
	}
	image, err := s.Get(ctx, id)
	if err != nil {
		return domain.VMImage{}, err
	}
	volumes, err := s.volumes.ListStorageVolumes(ctx, image.Pool)
	if err != nil {
		return domain.VMImage{}, err
	}
	var source *domain.StorageVolume
	for index := range volumes {
		if volumes[index].Name == image.Volume {
			source = &volumes[index]
			break
		}
	}
	if source == nil {
		return domain.VMImage{}, domain.ErrNotFound
	}
	checksum, err := s.volumes.StorageVolumeChecksum(ctx, image.Pool, image.Volume)
	if err != nil {
		return domain.VMImage{}, err
	}
	image.CapacityBytes = source.CapacityBytes
	image.AllocationBytes = source.AllocationBytes
	image.SHA256 = checksum
	updated, err := s.repository.UpdateVMImageMetadata(ctx, image)
	if err != nil {
		return domain.VMImage{}, err
	}
	if err := s.recordAudit(ctx, request, "image.refresh", updated.ID, imageDetail(updated), nil); err != nil {
		return domain.VMImage{}, fmt.Errorf("record image refresh audit: %w", err)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id int64, request MutationContext) error {
	if !domain.CanOperateVMs(request.Session.User.Role) {
		return domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.ErrReadOnly
	}
	image, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.DeleteVMImage(ctx, id); err != nil {
		return err
	}
	if err := s.recordAudit(ctx, request, "image.unregister", id, imageDetail(image), nil); err != nil {
		return fmt.Errorf("record image unregister audit: %w", err)
	}
	return nil
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action string, id int64, detail map[string]any, cause error) error {
	if s.audit == nil {
		return nil
	}
	result := "success"
	if cause != nil {
		result = "failure"
		detail["error"] = cause.Error()
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: time.Now().UTC(), RequestID: request.RequestID, Actor: request.Session.User.Username,
		Role: request.Session.User.Role, Action: action, TargetKind: "vm_image", TargetID: fmt.Sprintf("%d", id),
		SourceIP: request.SourceIP, Result: result, Detail: detail,
	})
}

func normalizeSpec(spec domain.VMImageCreateSpec) domain.VMImageCreateSpec {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Description = strings.TrimSpace(spec.Description)
	spec.Pool = strings.TrimSpace(spec.Pool)
	spec.Volume = strings.TrimSpace(spec.Volume)
	spec.OSFamily = strings.TrimSpace(spec.OSFamily)
	spec.OSName = strings.TrimSpace(spec.OSName)
	spec.Architecture = strings.TrimSpace(spec.Architecture)
	return spec
}

func imageDetail(image domain.VMImage) map[string]any {
	return map[string]any{
		"name": image.Name, "pool": image.Pool, "volume": image.Volume, "format": image.Format,
		"osFamily": image.OSFamily, "osName": image.OSName, "architecture": image.Architecture,
		"capacityBytes": image.CapacityBytes, "sha256": image.SHA256,
	}
}

func formatFromVolumeName(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.HasSuffix(lower, ".qcow2"):
		return "qcow2"
	case strings.HasSuffix(lower, ".raw"), strings.HasSuffix(lower, ".img"):
		return "raw"
	default:
		return ""
	}
}

func hasControl(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}
