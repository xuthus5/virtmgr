package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

type Repository interface {
	ListStoragePools(ctx context.Context) ([]domain.StoragePool, error)
	GetStoragePool(ctx context.Context, name string) (domain.StoragePool, error)
	CreateStoragePool(ctx context.Context, spec domain.StoragePoolCreateSpec) (domain.StoragePool, error)
	PerformStoragePoolAction(ctx context.Context, name string, action domain.StoragePoolAction) error
	DeleteStoragePool(ctx context.Context, name string) error
	ListStorageVolumes(ctx context.Context, poolName string) ([]domain.StorageVolume, error)
	ListVMs(ctx context.Context) ([]domain.VM, error)
	CreateStorageVolume(ctx context.Context, poolName string, spec domain.StorageVolumeCreateSpec) (domain.StorageVolume, error)
	UploadStorageVolume(ctx context.Context, poolName string, spec domain.StorageVolumeCreateSpec, reader io.Reader) (domain.StorageVolume, error)
	DeleteStorageVolume(ctx context.Context, poolName, volumeName string) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type TaskRecorder interface {
	Start(ctx context.Context, kind, targetKind, targetID, actor, step string) (string, error)
	Progress(ctx context.Context, id string, progress int, step string) error
	Succeed(ctx context.Context, id, step string) error
	Fail(ctx context.Context, id string, progress int, step string, cause error) error
}

type Service struct {
	repository   Repository
	audit        AuditRecorder
	readOnly     bool
	readOnlyMode domain.ReadOnlyProvider
	tasks        TaskRecorder
}

type Option func(*Service)

func WithTaskRecorder(recorder TaskRecorder) Option {
	return func(service *Service) {
		service.tasks = recorder
	}
}

func WithReadOnlyMode(mode domain.ReadOnlyProvider) Option {
	return func(service *Service) {
		service.readOnlyMode = mode
	}
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type CreateVolumeCommand struct {
	Pool    string
	Spec    domain.StorageVolumeCreateSpec
	Request MutationContext
}

type CreatePoolCommand struct {
	Spec    domain.StoragePoolCreateSpec
	Request MutationContext
}

type StoragePoolActionCommand struct {
	Name    string
	Action  domain.StoragePoolAction
	Request MutationContext
}

type DeletePoolCommand struct {
	Name    string
	Request MutationContext
}

type DeleteVolumeCommand struct {
	Pool    string
	Name    string
	Request MutationContext
}

type UploadVolumeCommand struct {
	Pool    string
	Name    string
	Size    int64
	Body    io.Reader
	Request MutationContext
}

var (
	poolNamePattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,63}$`)
	volumeNamePattern      = regexp.MustCompile(`^[^/\\\x00-\x1f\x7f]{1,127}$`)
	storagePoolHostPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.:-]{0,252}$`)
	volumeGroupPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.-]{0,127}$`)
	volumeFormats          = map[string]string{"qcow2": ".qcow2", "raw": ".raw"}
	minVolumeBytes         = uint64(1 << 20)
	maxVolumeBytes         = uint64(16 << 40)
	maxUploadBytes         = uint64(256 << 30)
)

func New(repository Repository, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{repository: repository, audit: audit, readOnly: readOnly}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) ListPools(ctx context.Context) (domain.StoragePoolList, error) {
	pools, err := s.repository.ListStoragePools(ctx)
	if err != nil {
		return domain.StoragePoolList{}, err
	}
	sort.Slice(pools, func(i, j int) bool {
		return strings.ToLower(pools[i].Name) < strings.ToLower(pools[j].Name)
	})
	return domain.StoragePoolList{Items: pools, Total: len(pools)}, nil
}

func (s *Service) GetPool(ctx context.Context, name string) (domain.StoragePool, error) {
	name = strings.TrimSpace(name)
	if !poolNamePattern.MatchString(name) {
		return domain.StoragePool{}, domain.ErrInvalidInput
	}
	return s.repository.GetStoragePool(ctx, name)
}

func (s *Service) CreatePool(ctx context.Context, command CreatePoolCommand) (domain.StoragePool, error) {
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	detail := storagePoolSpecDetail(command.Spec)
	if !domain.CanManageStorage(command.Request.Session.User.Role) {
		return domain.StoragePool{}, s.poolMutationFailure(ctx, command.Request, "storage.pool.create", command.Spec.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.StoragePool{}, s.poolMutationFailure(ctx, command.Request, "storage.pool.create", command.Spec.Name, detail, domain.ErrReadOnly)
	}
	spec, err := normalizeStoragePoolCreateSpec(command.Spec)
	if err != nil {
		return domain.StoragePool{}, s.poolMutationFailure(ctx, command.Request, "storage.pool.create", command.Spec.Name, detail, err)
	}
	pool, err := s.repository.CreateStoragePool(ctx, spec)
	if err != nil {
		return domain.StoragePool{}, s.poolMutationFailure(ctx, command.Request, "storage.pool.create", spec.Name, storagePoolSpecDetail(spec), err)
	}
	if err := s.recordAudit(ctx, command.Request, "storage.pool.create", "storage_pool", pool.Name, "success", storagePoolSpecDetail(spec), nil); err != nil {
		return domain.StoragePool{}, fmt.Errorf("record storage pool create audit: %w", err)
	}
	return pool, nil
}

func (s *Service) PerformPoolAction(ctx context.Context, command StoragePoolActionCommand) error {
	command.Name = strings.TrimSpace(command.Name)
	detail := map[string]any{"action": string(command.Action)}
	if !domain.CanManageStorage(command.Request.Session.User.Role) {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool."+string(command.Action), command.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool."+string(command.Action), command.Name, detail, domain.ErrReadOnly)
	}
	if !poolNamePattern.MatchString(command.Name) {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool."+string(command.Action), command.Name, detail, domain.ErrInvalidInput)
	}
	if _, ok := domain.ValidStoragePoolAction(string(command.Action)); !ok {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool.action", command.Name, detail, domain.ErrInvalidInput)
	}
	if err := s.repository.PerformStoragePoolAction(ctx, command.Name, command.Action); err != nil {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool."+string(command.Action), command.Name, detail, err)
	}
	if err := s.recordAudit(ctx, command.Request, "storage.pool."+string(command.Action), "storage_pool", command.Name, "success", detail, nil); err != nil {
		return fmt.Errorf("record storage pool action audit: %w", err)
	}
	return nil
}

func (s *Service) DeletePool(ctx context.Context, command DeletePoolCommand) error {
	command.Name = strings.TrimSpace(command.Name)
	detail := map[string]any{"name": command.Name}
	if !domain.CanManageStorage(command.Request.Session.User.Role) {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool.delete", command.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool.delete", command.Name, detail, domain.ErrReadOnly)
	}
	if !poolNamePattern.MatchString(command.Name) {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool.delete", command.Name, detail, domain.ErrInvalidInput)
	}
	if err := s.repository.DeleteStoragePool(ctx, command.Name); err != nil {
		return s.poolMutationFailure(ctx, command.Request, "storage.pool.delete", command.Name, detail, err)
	}
	if err := s.recordAudit(ctx, command.Request, "storage.pool.delete", "storage_pool", command.Name, "success", detail, nil); err != nil {
		return fmt.Errorf("record storage pool delete audit: %w", err)
	}
	return nil
}

func (s *Service) ListVolumes(ctx context.Context, poolName string) (domain.StorageVolumeList, error) {
	poolName = strings.TrimSpace(poolName)
	if !poolNamePattern.MatchString(poolName) {
		return domain.StorageVolumeList{}, domain.ErrInvalidInput
	}
	volumes, err := s.repository.ListStorageVolumes(ctx, poolName)
	if err != nil {
		return domain.StorageVolumeList{}, err
	}
	vms, err := s.repository.ListVMs(ctx)
	if err != nil {
		return domain.StorageVolumeList{}, err
	}
	if volumes == nil {
		volumes = make([]domain.StorageVolume, 0)
	}
	for index := range volumes {
		volumes[index].UsedBy = storageVolumeUsage(volumes[index], vms)
	}
	sort.Slice(volumes, func(i, j int) bool {
		return strings.ToLower(volumes[i].Name) < strings.ToLower(volumes[j].Name)
	})
	return domain.StorageVolumeList{Pool: poolName, Items: volumes, Total: len(volumes)}, nil
}

func storageVolumeUsage(volume domain.StorageVolume, vms []domain.VM) []domain.StorageVolumeUsage {
	result := make([]domain.StorageVolumeUsage, 0)
	for _, vm := range vms {
		for _, disk := range vm.Disks {
			if !storageSourceMatchesVolume(disk.Source, volume) {
				continue
			}
			result = append(result, domain.StorageVolumeUsage{
				VMUUID: vm.UUID, VMName: vm.Name, Target: disk.Target, ReadOnly: disk.ReadOnly,
			})
		}
	}
	return result
}

func storageSourceMatchesVolume(source string, volume domain.StorageVolume) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	volumePath := strings.TrimSpace(volume.Path)
	if volumePath != "" && filepath.Clean(source) == filepath.Clean(volumePath) {
		return true
	}
	return filepath.Base(source) == volume.Name
}

func (s *Service) CreateVolume(ctx context.Context, command CreateVolumeCommand) (domain.StorageVolume, error) {
	command.Pool = strings.TrimSpace(command.Pool)
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	if !domain.CanManageStorage(command.Request.Session.User.Role) {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.create", command.Pool+"/"+command.Spec.Name, storageVolumeSpecDetail(command.Spec), domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.create", command.Pool+"/"+command.Spec.Name, storageVolumeSpecDetail(command.Spec), domain.ErrReadOnly)
	}
	if !poolNamePattern.MatchString(command.Pool) {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.create", "/"+command.Spec.Name, storageVolumeSpecDetail(command.Spec), domain.ErrInvalidInput)
	}
	spec, err := normalizeVolumeCreateSpec(command.Spec)
	if err != nil {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.create", command.Pool+"/"+command.Spec.Name, storageVolumeSpecDetail(command.Spec), err)
	}
	pool, err := s.ensureVolumeCapacity(ctx, command.Pool, domain.StorageProvisionRequiredBytes(spec.Format, spec.CapacityBytes, 0))
	if err != nil {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.create", command.Pool+"/"+spec.Name, storageVolumeSpecDetail(spec), err)
	}
	if pool.Type == "logical" && spec.Format != "raw" {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.create", command.Pool+"/"+spec.Name, storageVolumeSpecDetail(spec),
			fmt.Errorf("%w: logical LVM pools only support raw volumes", domain.ErrInvalidInput))
	}
	volume, err := s.repository.CreateStorageVolume(ctx, command.Pool, spec)
	if err != nil {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.create", command.Pool+"/"+spec.Name, storageVolumeSpecDetail(spec), err)
	}
	detail := storageVolumeSpecDetail(spec)
	detail["path"] = volume.Path
	if err := s.recordMutationAudit(ctx, command.Request, "storage.volume.create", command.Pool+"/"+volume.Name, "success", detail, nil); err != nil {
		return domain.StorageVolume{}, fmt.Errorf("record storage volume create audit: %w", err)
	}
	return volume, nil
}

func (s *Service) UploadVolume(ctx context.Context, command UploadVolumeCommand) (domain.StorageVolume, error) {
	command.Pool = strings.TrimSpace(command.Pool)
	command.Name = strings.TrimSpace(command.Name)
	format, supportedUpload := uploadVolumeFormat(command.Name)
	detail := map[string]any{"name": command.Name, "format": format}
	if command.Size > 0 {
		detail["capacityBytes"] = uint64(command.Size)
	}
	if !domain.CanManageStorage(command.Request.Session.User.Role) {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, domain.ErrReadOnly)
	}
	if !poolNamePattern.MatchString(command.Pool) || !validVolumeName(command.Name) || !supportedUpload {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, domain.ErrInvalidInput)
	}
	if command.Body == nil || command.Size <= 0 {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, domain.ErrInvalidInput)
	}
	if command.Size > int64(maxUploadBytes) {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, domain.ErrTooLarge)
	}
	if command.Size < int64(minVolumeBytes) {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, domain.ErrInvalidInput)
	}
	pool, err := s.ensureVolumeCapacity(ctx, command.Pool, domain.StorageProvisionRequiredBytes(format, uint64(command.Size), 0))
	if err != nil {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, err)
	}
	if pool.Type == "logical" {
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail,
			fmt.Errorf("%w: uploads are not supported by logical LVM pools", domain.ErrActionNotAllowed))
	}
	taskID := ""
	if s.tasks != nil {
		var err error
		taskID, err = s.tasks.Start(ctx, "storage.volume.upload", "storage_volume", command.Pool+"/"+command.Name,
			command.Request.Session.User.Username, "正在上传 "+command.Name)
		if err != nil {
			return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+command.Name, detail, fmt.Errorf("start upload task: %w", err))
		}
	}
	spec := domain.StorageVolumeCreateSpec{Name: command.Name, Format: format, CapacityBytes: uint64(command.Size)}
	body := command.Body
	var tracker *uploadProgressTracker
	if taskID != "" {
		tracker = &uploadProgressTracker{
			ctx: ctx, reader: command.Body, total: command.Size, recorder: s.tasks, taskID: taskID, name: command.Name, lastProgress: -1,
		}
		body = tracker
	}
	volume, err := s.repository.UploadStorageVolume(ctx, command.Pool, spec, body)
	if err != nil {
		if taskID != "" {
			if taskErr := s.tasks.Fail(ctx, taskID, tracker.Progress(), "上传失败", err); taskErr != nil {
				err = errors.Join(err, fmt.Errorf("record upload task failure: %w", taskErr))
			}
		}
		return domain.StorageVolume{}, s.mutationFailure(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+spec.Name, detail, err)
	}
	if taskID != "" {
		if err := s.tasks.Succeed(ctx, taskID, "存储卷上传完成"); err != nil {
			return domain.StorageVolume{}, fmt.Errorf("record upload task success: %w", err)
		}
	}
	detail["path"] = volume.Path
	if err := s.recordMutationAudit(ctx, command.Request, "storage.volume.upload", command.Pool+"/"+volume.Name, "success", detail, nil); err != nil {
		return domain.StorageVolume{}, fmt.Errorf("record storage volume upload audit: %w", err)
	}
	return volume, nil
}

func uploadVolumeFormat(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".iso":
		return "raw", true
	case ".qcow2":
		return "qcow2", true
	case ".raw":
		return "raw", true
	default:
		return "", false
	}
}

func (s *Service) ensureVolumeCapacity(ctx context.Context, poolName string, required uint64) (domain.StoragePool, error) {
	pool, err := s.repository.GetStoragePool(ctx, poolName)
	if err != nil {
		return domain.StoragePool{}, err
	}
	if !domain.StoragePoolHasCapacity(pool, required) {
		return domain.StoragePool{}, fmt.Errorf("%w: storage pool %s requires %d bytes, only %d available", domain.ErrStorageInsufficient, poolName, required, pool.AvailableBytes)
	}
	return pool, nil
}

func (s *Service) DeleteVolume(ctx context.Context, command DeleteVolumeCommand) error {
	command.Pool = strings.TrimSpace(command.Pool)
	command.Name = strings.TrimSpace(command.Name)
	detail := map[string]any{"pool": command.Pool, "name": command.Name}
	if !domain.CanManageStorage(command.Request.Session.User.Role) {
		return s.mutationFailure(ctx, command.Request, "storage.volume.delete", command.Pool+"/"+command.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.mutationFailure(ctx, command.Request, "storage.volume.delete", command.Pool+"/"+command.Name, detail, domain.ErrReadOnly)
	}
	if !poolNamePattern.MatchString(command.Pool) || !validVolumeName(command.Name) {
		return s.mutationFailure(ctx, command.Request, "storage.volume.delete", command.Pool+"/"+command.Name, detail, domain.ErrInvalidInput)
	}
	if err := s.repository.DeleteStorageVolume(ctx, command.Pool, command.Name); err != nil {
		return s.mutationFailure(ctx, command.Request, "storage.volume.delete", command.Pool+"/"+command.Name, detail, err)
	}
	if err := s.recordMutationAudit(ctx, command.Request, "storage.volume.delete", command.Pool+"/"+command.Name, "success", detail, nil); err != nil {
		return fmt.Errorf("record storage volume delete audit: %w", err)
	}
	return nil
}

func normalizeVolumeCreateSpec(spec domain.StorageVolumeCreateSpec) (domain.StorageVolumeCreateSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Format = strings.ToLower(strings.TrimSpace(spec.Format))
	if spec.Format == "" {
		spec.Format = "qcow2"
	}
	extension, ok := volumeFormats[spec.Format]
	if !ok {
		return domain.StorageVolumeCreateSpec{}, fmt.Errorf("%w: volume format must be qcow2 or raw", domain.ErrInvalidInput)
	}
	if !validVolumeName(spec.Name) {
		return domain.StorageVolumeCreateSpec{}, fmt.Errorf("%w: invalid storage volume name", domain.ErrInvalidInput)
	}
	if !strings.HasSuffix(strings.ToLower(spec.Name), extension) {
		return domain.StorageVolumeCreateSpec{}, fmt.Errorf("%w: volume name must end with %s", domain.ErrInvalidInput, extension)
	}
	if spec.CapacityBytes < minVolumeBytes || spec.CapacityBytes > maxVolumeBytes {
		return domain.StorageVolumeCreateSpec{}, fmt.Errorf("%w: capacity must be between %d and %d bytes", domain.ErrInvalidInput, minVolumeBytes, maxVolumeBytes)
	}
	return spec, nil
}

func validVolumeName(name string) bool {
	return name != "." && name != ".." && volumeNamePattern.MatchString(name)
}

func normalizeStoragePoolCreateSpec(spec domain.StoragePoolCreateSpec) (domain.StoragePoolCreateSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Type = strings.ToLower(strings.TrimSpace(spec.Type))
	spec.TargetPath = strings.TrimSpace(spec.TargetPath)
	spec.SourceHost = strings.TrimSpace(spec.SourceHost)
	spec.SourcePath = strings.TrimSpace(spec.SourcePath)
	spec.VolumeGroup = strings.TrimSpace(spec.VolumeGroup)
	if spec.Type == "" {
		spec.Type = "dir"
	}
	if !poolNamePattern.MatchString(spec.Name) {
		return domain.StoragePoolCreateSpec{}, fmt.Errorf("%w: invalid storage pool name", domain.ErrInvalidInput)
	}
	switch spec.Type {
	case "dir":
		if spec.SourceHost != "" || spec.SourcePath != "" || spec.VolumeGroup != "" {
			return domain.StoragePoolCreateSpec{}, fmt.Errorf("%w: dir pools do not accept network or LVM source fields", domain.ErrInvalidInput)
		}
		var err error
		spec.TargetPath, err = normalizeAbsolutePoolPath(spec.TargetPath, false)
		if err != nil {
			return domain.StoragePoolCreateSpec{}, err
		}
	case "netfs":
		if !storagePoolHostPattern.MatchString(spec.SourceHost) || spec.VolumeGroup != "" {
			return domain.StoragePoolCreateSpec{}, fmt.Errorf("%w: NFS pool requires a valid source host and no volume group", domain.ErrInvalidInput)
		}
		var err error
		spec.SourcePath, err = normalizeAbsolutePoolPath(spec.SourcePath, true)
		if err != nil {
			return domain.StoragePoolCreateSpec{}, fmt.Errorf("%w: NFS export path must be absolute", domain.ErrInvalidInput)
		}
		spec.TargetPath, err = normalizeAbsolutePoolPath(spec.TargetPath, false)
		if err != nil {
			return domain.StoragePoolCreateSpec{}, err
		}
	case "logical":
		if spec.SourceHost != "" || spec.SourcePath != "" || !volumeGroupPattern.MatchString(spec.VolumeGroup) {
			return domain.StoragePoolCreateSpec{}, fmt.Errorf("%w: LVM pool requires a valid volume group", domain.ErrInvalidInput)
		}
		path, err := normalizeAbsolutePoolPath(spec.TargetPath, false)
		if err != nil || !strings.HasPrefix(path, "/dev/") {
			return domain.StoragePoolCreateSpec{}, fmt.Errorf("%w: LVM target path must be an absolute /dev path", domain.ErrInvalidInput)
		}
		spec.TargetPath = path
	default:
		return domain.StoragePoolCreateSpec{}, fmt.Errorf("%w: storage pool type must be dir, netfs or logical", domain.ErrInvalidInput)
	}
	return spec, nil
}

func normalizeAbsolutePoolPath(path string, allowRoot bool) (string, error) {
	if path == "" || strings.ContainsAny(path, "\x00\r\n") || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: path must be absolute", domain.ErrInvalidInput)
	}
	path = filepath.Clean(path)
	if path == string(filepath.Separator) && !allowRoot {
		return "", fmt.Errorf("%w: path cannot be the filesystem root", domain.ErrInvalidInput)
	}
	return path, nil
}

func storageVolumeSpecDetail(spec domain.StorageVolumeCreateSpec) map[string]any {
	return map[string]any{
		"name":          spec.Name,
		"format":        spec.Format,
		"capacityBytes": spec.CapacityBytes,
	}
}

func storagePoolSpecDetail(spec domain.StoragePoolCreateSpec) map[string]any {
	return map[string]any{
		"name": spec.Name, "type": spec.Type, "targetPath": spec.TargetPath,
		"sourceHost": spec.SourceHost, "sourcePath": spec.SourcePath, "volumeGroup": spec.VolumeGroup,
		"autostart": spec.Autostart, "start": spec.Start,
	}
}

func (s *Service) mutationFailure(ctx context.Context, request MutationContext, action, targetID string, detail map[string]any, cause error) error {
	if err := s.recordMutationAudit(ctx, request, action, targetID, "failure", detail, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record %s audit: %w", action, err))
	}
	return cause
}

func (s *Service) poolMutationFailure(ctx context.Context, request MutationContext, action, targetID string, detail map[string]any, cause error) error {
	if err := s.recordAudit(ctx, request, action, "storage_pool", targetID, "failure", detail, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record %s audit: %w", action, err))
	}
	return cause
}

func (s *Service) recordMutationAudit(ctx context.Context, request MutationContext, action, targetID, result string, detail map[string]any, cause error) error {
	return s.recordAudit(ctx, request, action, "storage_volume", targetID, result, detail, cause)
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action, targetKind, targetID, result string, detail map[string]any, cause error) error {
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
		OccurredAt: time.Now().UTC(),
		RequestID:  request.RequestID,
		Actor:      request.Session.User.Username,
		Role:       request.Session.User.Role,
		Action:     action,
		TargetKind: targetKind,
		TargetID:   targetID,
		SourceIP:   request.SourceIP,
		Result:     result,
		Detail:     detail,
	})
}
