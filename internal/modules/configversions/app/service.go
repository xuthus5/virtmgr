package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"virtmgr/internal/domain"
)

type Repository interface {
	ListVMConfigVersions(ctx context.Context, vmUUID string) ([]domain.VMConfigVersion, error)
	LatestVMConfigVersion(ctx context.Context, vmUUID string) (domain.VMConfigVersion, error)
	GetVMConfigVersion(ctx context.Context, vmUUID string, versionNo int) (domain.VMConfigVersion, []byte, error)
	CreateVMConfigVersion(ctx context.Context, item domain.VMConfigVersion, compressed []byte) (domain.VMConfigVersion, error)
	SetVMConfigVersionProtected(ctx context.Context, vmUUID string, versionNo int, protected bool) (domain.VMConfigVersion, error)
	PruneVMConfigVersions(ctx context.Context, vmUUID string, keep int) (int64, error)
}

type VMBackend interface {
	ListVMs(ctx context.Context) ([]domain.VM, error)
	GetVM(ctx context.Context, uuid string) (domain.VM, error)
	GetVMXML(ctx context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error)
	DefineVMXML(ctx context.Context, uuid, xmlText string) (domain.VMXML, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type RollbackCommand struct {
	UUID           string
	VersionNo      int
	ExpectedSHA256 string
	Note           string
	Request        MutationContext
}

type ProtectCommand struct {
	UUID      string
	VersionNo int
	Protected bool
	Request   MutationContext
}

type Service struct {
	repository   Repository
	backend      VMBackend
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
	now          func() time.Time
	captureMu    sync.Mutex
}

type Option func(*Service)

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) { service.readOnlyMode = mode }
}

func WithClock(now func() time.Time) Option {
	return func(service *Service) { service.now = now }
}

func New(repository Repository, backend VMBackend, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{
		repository: repository,
		backend:    backend,
		audit:      audit,
		readOnly:   readOnly,
		now:        func() time.Time { return time.Now().UTC() },
	}
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

// CaptureVM archives the current persistent XML for a VM. It is safe to call
// repeatedly: consecutive snapshots with the same SHA-256 are deduplicated.
func (s *Service) CaptureVM(ctx context.Context, vmUUID string, capture domain.VMConfigCapture) (domain.VMConfigVersion, error) {
	vmUUID = strings.TrimSpace(vmUUID)
	if vmUUID == "" || !domain.ValidVMConfigVersionSource(capture.Source) {
		return domain.VMConfigVersion{}, domain.ErrInvalidInput
	}
	config, err := s.backend.GetVMXML(ctx, vmUUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VMConfigVersion{}, err
	}
	return s.captureXML(ctx, vmUUID, config.XML, capture)
}

// EnsureBaseline captures every persistent VM once after startup when no
// history exists. VMs changed while the service was offline are captured with
// the external source so they remain distinguishable from initial baselines.
func (s *Service) EnsureBaseline(ctx context.Context) error {
	vms, err := s.backend.ListVMs(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, vm := range vms {
		if vm.UUID == "" || !vm.Persistent {
			continue
		}
		source := domain.VMConfigSourceBaseline
		if _, latestErr := s.repository.LatestVMConfigVersion(ctx, vm.UUID); latestErr == nil {
			source = domain.VMConfigSourceExternal
		} else if !errors.Is(latestErr, domain.ErrNotFound) {
			failures = append(failures, fmt.Errorf("inspect history for %s: %w", vm.UUID, latestErr))
			continue
		}
		if _, captureErr := s.CaptureVM(ctx, vm.UUID, domain.VMConfigCapture{
			Source: source,
			Actor:  "system",
			Role:   domain.RoleAdmin,
			Note:   "启动时自动采集持久域 XML",
		}); captureErr != nil {
			failures = append(failures, fmt.Errorf("capture baseline for %s: %w", vm.UUID, captureErr))
		}
	}
	return errors.Join(failures...)
}

func (s *Service) List(ctx context.Context, vmUUID string) (domain.VMConfigVersionList, error) {
	vmUUID = strings.TrimSpace(vmUUID)
	if vmUUID == "" {
		return domain.VMConfigVersionList{}, domain.ErrInvalidInput
	}
	items, err := s.repository.ListVMConfigVersions(ctx, vmUUID)
	if err != nil {
		return domain.VMConfigVersionList{}, err
	}
	current, err := s.backend.GetVMXML(ctx, vmUUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VMConfigVersionList{}, err
	}
	currentSHA := hashXML(current.XML)
	for index := range items {
		items[index].IsCurrent = items[index].XMLSHA256 == currentSHA
	}
	return domain.VMConfigVersionList{
		Items:           items,
		Total:           len(items),
		CurrentSHA256:   currentSHA,
		CurrentCaptured: containsHash(items, currentSHA),
	}, nil
}

func (s *Service) Get(ctx context.Context, vmUUID string, versionNo int) (domain.VMConfigVersionDetail, error) {
	vmUUID = strings.TrimSpace(vmUUID)
	if vmUUID == "" || versionNo < 1 {
		return domain.VMConfigVersionDetail{}, domain.ErrInvalidInput
	}
	item, compressed, err := s.repository.GetVMConfigVersion(ctx, vmUUID, versionNo)
	if err != nil {
		return domain.VMConfigVersionDetail{}, err
	}
	xmlText, err := decompressXML(compressed)
	if err != nil {
		return domain.VMConfigVersionDetail{}, err
	}
	if hashXML(xmlText) != item.XMLSHA256 {
		return domain.VMConfigVersionDetail{}, fmt.Errorf("%w: stored config version checksum mismatch", domain.ErrConflict)
	}
	return domain.VMConfigVersionDetail{VMConfigVersion: item, XML: xmlText}, nil
}

func (s *Service) SetProtected(ctx context.Context, command ProtectCommand) (domain.VMConfigVersion, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	detail := map[string]any{"version": command.VersionNo, "protected": command.Protected}
	if command.UUID == "" || command.VersionNo < 1 {
		return domain.VMConfigVersion{}, domain.ErrInvalidInput
	}
	if !domain.CanEditVMXML(command.Request.Session.User.Role) {
		return domain.VMConfigVersion{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.VMConfigVersion{}, domain.ErrReadOnly
	}
	item, err := s.repository.SetVMConfigVersionProtected(ctx, command.UUID, command.VersionNo, command.Protected)
	if err != nil {
		_ = s.recordAudit(ctx, command.Request, "vm.config.version.pin", command.UUID, "failure", detail, err)
		return domain.VMConfigVersion{}, err
	}
	if err := s.recordAudit(ctx, command.Request, "vm.config.version.pin", command.UUID, "success", detail, nil); err != nil {
		return domain.VMConfigVersion{}, err
	}
	return item, nil
}

func (s *Service) Rollback(ctx context.Context, command RollbackCommand) (domain.VMConfigVersionDetail, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(command.ExpectedSHA256))
	command.Note = strings.TrimSpace(command.Note)
	detail := map[string]any{
		"sourceVersion":  command.VersionNo,
		"expectedSha256": command.ExpectedSHA256,
	}
	if command.Note != "" {
		detail["note"] = command.Note
	}
	if command.UUID == "" || command.VersionNo < 1 || command.ExpectedSHA256 == "" || len(command.Note) > 1024 {
		return domain.VMConfigVersionDetail{}, domain.ErrInvalidInput
	}
	if !domain.CanEditVMXML(command.Request.Session.User.Role) {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, domain.ErrReadOnly)
	}

	vm, err := s.backend.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, err)
	}
	if !vm.Persistent {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, domain.ErrActionNotAllowed)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)

	current, err := s.backend.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, err)
	}
	currentSHA := hashXML(current.XML)
	detail["currentSha256"] = currentSHA
	if currentSHA != command.ExpectedSHA256 {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, fmt.Errorf("%w: current config changed, refresh before rollback", domain.ErrConflict))
	}

	target, compressed, err := s.repository.GetVMConfigVersion(ctx, command.UUID, command.VersionNo)
	if err != nil {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, err)
	}
	targetXML, err := decompressXML(compressed)
	if err != nil {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, err)
	}
	if hashXML(targetXML) != target.XMLSHA256 {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, fmt.Errorf("%w: stored config version checksum mismatch", domain.ErrConflict))
	}
	detail["targetSha256"] = target.XMLSHA256
	if target.XMLSHA256 == currentSHA {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, fmt.Errorf("%w: selected version is already the current config", domain.ErrConflict))
	}

	if _, err := s.captureXML(ctx, command.UUID, current.XML, domain.VMConfigCapture{
		Source:    domain.VMConfigSourcePreRollback,
		Actor:     command.Request.Session.User.Username,
		Role:      command.Request.Session.User.Role,
		RequestID: command.Request.RequestID,
		SourceIP:  command.Request.SourceIP,
		Note:      fmt.Sprintf("回滚到版本 v%d 前的自动备份", command.VersionNo),
	}); err != nil {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, fmt.Errorf("capture current config before rollback: %w", err))
	}

	defined, err := s.backend.DefineVMXML(ctx, command.UUID, targetXML)
	if err != nil {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, err)
	}
	captured, err := s.captureXML(ctx, command.UUID, defined.XML, domain.VMConfigCapture{
		Source:        domain.VMConfigSourceRollback,
		SourceVersion: command.VersionNo,
		Actor:         command.Request.Session.User.Username,
		Role:          command.Request.Session.User.Role,
		RequestID:     command.Request.RequestID,
		SourceIP:      command.Request.SourceIP,
		Note:          command.Note,
	})
	if err != nil {
		return domain.VMConfigVersionDetail{}, s.rollbackFailure(ctx, command, detail, fmt.Errorf("domain XML rolled back but history capture failed: %w", err))
	}
	detail["newVersion"] = captured.VersionNo
	if err := s.recordAudit(ctx, command.Request, "vm.config.rollback", command.UUID, "success", detail, nil); err != nil {
		return domain.VMConfigVersionDetail{}, err
	}
	captured.IsCurrent = true
	return domain.VMConfigVersionDetail{VMConfigVersion: captured, XML: defined.XML}, nil
}

func (s *Service) captureXML(ctx context.Context, vmUUID, xmlText string, capture domain.VMConfigCapture) (domain.VMConfigVersion, error) {
	xmlText = strings.TrimSpace(xmlText)
	if xmlText == "" || len(xmlText) > domain.MaxVMConfigVersionXMLBytes {
		if len(xmlText) > domain.MaxVMConfigVersionXMLBytes {
			return domain.VMConfigVersion{}, domain.ErrTooLarge
		}
		return domain.VMConfigVersion{}, domain.ErrInvalidInput
	}
	capture = capture.Normalized()
	if !domain.ValidVMConfigVersionSource(capture.Source) {
		return domain.VMConfigVersion{}, domain.ErrInvalidInput
	}
	compressed, err := compressXML(xmlText)
	if err != nil {
		return domain.VMConfigVersion{}, err
	}
	digest := hashXML(xmlText)

	s.captureMu.Lock()
	defer s.captureMu.Unlock()

	latest, err := s.repository.LatestVMConfigVersion(ctx, vmUUID)
	switch {
	case err == nil:
		if latest.XMLSHA256 == digest {
			return latest, nil
		}
	case !errors.Is(err, domain.ErrNotFound):
		return domain.VMConfigVersion{}, err
	}
	versionNo := 1
	if err == nil {
		versionNo = latest.VersionNo + 1
	}
	item, err := s.repository.CreateVMConfigVersion(ctx, domain.VMConfigVersion{
		VMUUID:        vmUUID,
		VersionNo:     versionNo,
		XMLSHA256:     digest,
		SizeBytes:     len(xmlText),
		Source:        capture.Source,
		SourceVersion: capture.SourceVersion,
		Actor:         capture.Actor,
		Role:          capture.Role,
		RequestID:     capture.RequestID,
		SourceIP:      capture.SourceIP,
		Note:          capture.Note,
		CreatedAt:     s.now().UTC(),
	}, compressed)
	if err != nil {
		return domain.VMConfigVersion{}, err
	}
	_, _ = s.repository.PruneVMConfigVersions(ctx, vmUUID, domain.VMConfigVersionRetention)
	return item, nil
}

func (s *Service) rollbackFailure(ctx context.Context, command RollbackCommand, detail map[string]any, cause error) error {
	if err := s.recordAudit(ctx, command.Request, "vm.config.rollback", command.UUID, "failure", detail, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record rollback audit: %w", err))
	}
	return cause
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action, targetID, result string, detail map[string]any, cause error) error {
	if s.audit == nil {
		return nil
	}
	if detail == nil {
		detail = map[string]any{}
	}
	if cause != nil {
		detail["error"] = cause.Error()
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: s.now().UTC(),
		RequestID:  request.RequestID,
		Actor:      request.Session.User.Username,
		Role:       request.Session.User.Role,
		Action:     action,
		TargetKind: "vm",
		TargetID:   targetID,
		SourceIP:   request.SourceIP,
		Result:     result,
		Detail:     detail,
	})
}

func hashXML(xmlText string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(xmlText)))
	return hex.EncodeToString(digest[:])
}

func containsHash(items []domain.VMConfigVersion, digest string) bool {
	for _, item := range items {
		if item.XMLSHA256 == digest {
			return true
		}
	}
	return false
}

func compressXML(xmlText string) ([]byte, error) {
	var buffer bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("create XML compressor: %w", err)
	}
	if _, err := writer.Write([]byte(xmlText)); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("compress XML: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish XML compression: %w", err)
	}
	return buffer.Bytes(), nil
}

func decompressXML(compressed []byte) (string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", fmt.Errorf("open compressed XML: %w", err)
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, int64(domain.MaxVMConfigVersionXMLBytes)+1))
	if err != nil {
		return "", fmt.Errorf("decompress XML: %w", err)
	}
	if len(body) > domain.MaxVMConfigVersionXMLBytes {
		return "", domain.ErrTooLarge
	}
	return string(body), nil
}
