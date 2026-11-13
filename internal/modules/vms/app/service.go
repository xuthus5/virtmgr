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
	"sync"
	"time"
	"unicode"

	"virtmgr/internal/domain"
	backupsapp "virtmgr/internal/modules/backups/app"
)

type Repository interface {
	ListVMs(ctx context.Context) ([]domain.VM, error)
	GetVM(ctx context.Context, uuid string) (domain.VM, error)
	GetVMXML(ctx context.Context, uuid string, format domain.VMXMLFormat) (domain.VMXML, error)
	DefineVMXML(ctx context.Context, uuid, xmlText string) (domain.VMXML, error)
	UpdateVMResources(ctx context.Context, uuid string, spec domain.VMResourcesSpec) (domain.VM, error)
	ResizeVMDisk(ctx context.Context, uuid, target string, spec domain.VMDiskResizeSpec) (domain.VM, error)
	SetVMAutostart(ctx context.Context, uuid string, enabled bool) error
	AttachHostDevice(ctx context.Context, uuid, deviceName string) (domain.VM, error)
	DetachHostDevice(ctx context.Context, uuid, deviceName string) (domain.VM, error)
	CreateVM(ctx context.Context, spec domain.VMCreateSpec) (domain.VM, error)
	CloneVM(ctx context.Context, uuid string, spec domain.VMCloneSpec, progress func(percent int, step string)) (domain.VM, error)
	BackupVM(ctx context.Context, uuid string, spec domain.VMBackupSpec, progress func(percent int, step string)) (domain.VMBackup, error)
	DeleteVM(ctx context.Context, uuid string) error
	PerformAction(ctx context.Context, uuid string, action domain.VMAction) error
	ListSnapshots(ctx context.Context, uuid string) ([]domain.VMSnapshot, error)
	CreateSnapshot(ctx context.Context, uuid string, spec domain.VMSnapshotCreateSpec) (domain.VMSnapshot, error)
	RevertSnapshot(ctx context.Context, uuid, name string) error
	DeleteSnapshot(ctx context.Context, uuid, name string) error
	OpenConsole(ctx context.Context, uuid string, graphicsIndex uint) (io.ReadWriteCloser, error)
	OpenSerialConsole(ctx context.Context, uuid, device string) (io.ReadWriteCloser, error)
	SendConsoleKey(ctx context.Context, uuid string, keycodes []uint) error
}

type HotplugRepository interface {
	AttachVMDisk(ctx context.Context, uuid string, spec domain.VMDiskAttachSpec) (domain.VM, error)
	DetachVMDisk(ctx context.Context, uuid, target string) (domain.VM, error)
	AttachVMNetwork(ctx context.Context, uuid string, spec domain.VMNetworkAttachSpec) (domain.VM, error)
	DetachVMNetwork(ctx context.Context, uuid, mac string) (domain.VM, error)
}

type VMMetadataRepository interface {
	ListVMMetadata(ctx context.Context) (map[string]domain.VMMetadata, error)
	GetVMMetadata(ctx context.Context, uuid string) (domain.VMMetadata, error)
	UpsertVMMetadata(ctx context.Context, item domain.VMMetadata) (domain.VMMetadata, error)
	DeleteVMMetadata(ctx context.Context, uuid string) error
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type MetricsProvider interface {
	VM(ctx context.Context, uuid string) (domain.VMMetrics, error)
}

type ConfigVersionRecorder interface {
	CaptureVM(ctx context.Context, vmUUID string, capture domain.VMConfigCapture) (domain.VMConfigVersion, error)
}

type TaskRecorder interface {
	Start(ctx context.Context, kind, targetKind, targetID, actor, step string) (string, error)
	Progress(ctx context.Context, id string, progress int, step string) error
	Succeed(ctx context.Context, id, step string) error
	Fail(ctx context.Context, id string, progress int, step string, cause error) error
}

type Service struct {
	repository     Repository
	audit          AuditRecorder
	readOnly       bool
	readOnlyMode   domain.ReadOnlyProvider
	consoleTokens  ConsoleTokenStore
	tasks          TaskRecorder
	metrics        MetricsProvider
	backupService  *backupsapp.Service
	configVersions ConfigVersionRecorder
	metadata       VMMetadataRepository
	serialMu       sync.Mutex
	serialSessions map[string]struct{}
}

type ConsoleTokenStore interface {
	Issue(vmUUID string, userID int64, kind domain.ConsoleKind, device string) (domain.ConsoleToken, error)
	Consume(token string) (domain.ConsoleToken, error)
}

type Option func(*Service)

func WithConsoleTokens(store ConsoleTokenStore) Option {
	return func(service *Service) {
		service.consoleTokens = store
	}
}

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

func WithMetrics(provider MetricsProvider) Option {
	return func(service *Service) {
		service.metrics = provider
	}
}

func WithBackupService(service *backupsapp.Service) Option {
	return func(current *Service) {
		current.backupService = service
	}
}

func WithConfigVersionRecorder(recorder ConfigVersionRecorder) Option {
	return func(service *Service) {
		service.configVersions = recorder
	}
}

func WithVMMetadata(repository VMMetadataRepository) Option {
	return func(service *Service) {
		service.metadata = repository
	}
}

func (s *Service) isReadOnly() bool {
	if s.readOnlyMode != nil {
		return s.readOnlyMode.ReadOnly()
	}
	return s.readOnly
}

type ActionCommand struct {
	UUID      string
	Action    domain.VMAction
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type CreateCommand struct {
	Spec    domain.VMCreateSpec
	Request MutationContext
}

type CloneCommand struct {
	UUID    string
	Spec    domain.VMCloneSpec
	Request MutationContext
}

type BackupCommand struct {
	UUID    string
	Spec    domain.VMBackupSpec
	Request MutationContext
}

type DeleteCommand struct {
	UUID    string
	Request MutationContext
}

type SetAutostartCommand struct {
	UUID    string
	Enabled bool
	Request MutationContext
}

type UpdateResourcesCommand struct {
	UUID    string
	Spec    domain.VMResourcesSpec
	Request MutationContext
}

type ResizeDiskCommand struct {
	UUID    string
	Target  string
	Spec    domain.VMDiskResizeSpec
	Request MutationContext
}

type HostDeviceCommand struct {
	UUID    string
	Device  string
	Request MutationContext
}

type AttachDiskCommand struct {
	UUID    string
	Spec    domain.VMDiskAttachSpec
	Request MutationContext
}

type DetachDiskCommand struct {
	UUID    string
	Target  string
	Request MutationContext
}

type AttachNetworkCommand struct {
	UUID    string
	Spec    domain.VMNetworkAttachSpec
	Request MutationContext
}

type DetachNetworkCommand struct {
	UUID    string
	MAC     string
	Request MutationContext
}

type VMMetadataCommand struct {
	UUID    string
	Spec    domain.VMMetadataSpec
	Request MutationContext
}

type SnapshotCreateCommand struct {
	UUID    string
	Spec    domain.VMSnapshotCreateSpec
	Request MutationContext
}

type SnapshotCommand struct {
	UUID    string
	Name    string
	Request MutationContext
}

type ExportCommand struct {
	UUID    string
	Request MutationContext
}

const vmExportDisclaimer = "该文件仅包含虚拟机配置与元数据，不包含磁盘镜像或内存数据。"

var (
	vmNamePattern             = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	snapshotPattern           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	networkPattern            = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
	cloudInitPoolPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,63}$`)
	storagePoolNamePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,63}$`)
	hostDeviceNamePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	diskTargetPattern         = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,31}$`)
	storageVolumeNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	macAddressPattern         = regexp.MustCompile(`^[0-9A-Fa-f]{2}(:[0-9A-Fa-f]{2}){5}$`)
	maxMemoryMiB              = uint64(1024 * 1024)
	minMemoryMiB              = uint64(128)
	minDiskCapacityBytes      = uint64(1 << 20)
	maxDiskCapacityBytes      = uint64(1 << 50)
	maxVCPUs                  = uint(255)
	maxCloudInitUserDataBytes = 64 << 10
	maxVMMetadataTags         = 20
	maxVMMetadataTagBytes     = 48
	maxVMMetadataNoteBytes    = 2000
	maxVMMetadataOwnerBytes   = 128
	supportedFormats          = map[string]struct{}{"qcow2": {}, "raw": {}}
)

func New(repository Repository, audit AuditRecorder, readOnly bool, options ...Option) *Service {
	service := &Service{repository: repository, audit: audit, readOnly: readOnly}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) List(ctx context.Context, query domain.ListQuery) (domain.VMSummaryList, error) {
	vms, err := s.repository.ListVMs(ctx)
	if err != nil {
		return domain.VMSummaryList{}, err
	}
	if s.metadata != nil {
		metadata, err := s.metadata.ListVMMetadata(ctx)
		if err != nil {
			return domain.VMSummaryList{}, err
		}
		for index := range vms {
			if item, ok := metadata[vms[index].UUID]; ok {
				copy := item
				vms[index].Metadata = &copy
			}
		}
	}
	filtered := make([]domain.VM, 0, len(vms))
	search := strings.ToLower(strings.TrimSpace(query.Query))
	state := strings.ToLower(strings.TrimSpace(query.State))
	tag := strings.ToLower(strings.TrimSpace(query.Tag))
	owner := strings.ToLower(strings.TrimSpace(query.Owner))
	for _, vm := range vms {
		if search != "" && !vmMatchesSearch(vm, search) {
			continue
		}
		if state != "" && string(vm.State) != state {
			continue
		}
		if owner != "" && (vm.Metadata == nil || !strings.Contains(strings.ToLower(vm.Metadata.Owner), owner)) {
			continue
		}
		if tag != "" && !vmHasTag(vm, tag) {
			continue
		}
		filtered = append(filtered, vm)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
	})
	return domain.VMSummaryList{Items: filtered, Total: len(filtered)}, nil
}

func (s *Service) Get(ctx context.Context, uuid string) (domain.VM, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	vm, err := s.repository.GetVM(ctx, uuid)
	if err != nil {
		return domain.VM{}, err
	}
	if s.metadata != nil {
		item, err := s.metadata.GetVMMetadata(ctx, uuid)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return domain.VM{}, err
		}
		if err == nil {
			vm.Metadata = &item
		}
	}
	return vm, nil
}

func (s *Service) GetMetadata(ctx context.Context, uuid string) (domain.VMMetadata, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.VMMetadata{}, domain.ErrInvalidInput
	}
	if _, err := s.repository.GetVM(ctx, uuid); err != nil {
		return domain.VMMetadata{}, err
	}
	if s.metadata == nil {
		return domain.VMMetadata{}, domain.ErrUnavailable
	}
	item, err := s.metadata.GetVMMetadata(ctx, uuid)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.VMMetadata{VMUUID: uuid, Tags: []string{}}, nil
	}
	return item, err
}

func (s *Service) UpdateMetadata(ctx context.Context, command VMMetadataCommand) (domain.VMMetadata, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return domain.VMMetadata{}, domain.ErrInvalidInput
	}
	spec, err := normalizeVMMetadataSpec(command.Spec)
	if err != nil {
		return domain.VMMetadata{}, err
	}
	detail := map[string]any{"tags": spec.Tags, "owner": spec.Owner}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VMMetadata{}, s.mutationFailure(ctx, command.Request, "vm.metadata.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VMMetadata{}, s.mutationFailure(ctx, command.Request, "vm.metadata.update", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.metadata == nil {
		return domain.VMMetadata{}, s.mutationFailure(ctx, command.Request, "vm.metadata.update", command.UUID, detail, domain.ErrUnavailable)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VMMetadata{}, s.mutationFailure(ctx, command.Request, "vm.metadata.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	item, err := s.metadata.UpsertVMMetadata(ctx, domain.VMMetadata{
		VMUUID: command.UUID, Tags: spec.Tags, Note: spec.Note, Owner: spec.Owner,
		UpdatedBy: command.Request.Session.User.Username, UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		return domain.VMMetadata{}, s.mutationFailure(ctx, command.Request, "vm.metadata.update", command.UUID, detail, err)
	}
	if err := s.recordMutationAudit(ctx, command.Request, "vm.metadata.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VMMetadata{}, fmt.Errorf("record VM metadata audit: %w", err)
	}
	return item, nil
}

func (s *Service) Metrics(ctx context.Context, uuid string) (domain.VMMetrics, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.VMMetrics{}, domain.ErrInvalidInput
	}
	if s.metrics == nil {
		return domain.VMMetrics{}, domain.ErrUnavailable
	}
	return s.metrics.VM(ctx, uuid)
}

func (s *Service) GetXML(ctx context.Context, uuid, format string) (domain.VMXML, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.VMXML{}, domain.ErrInvalidInput
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = string(domain.VMXMLFormatLive)
	}
	parsed, ok := domain.ValidVMXMLFormat(format)
	if !ok {
		return domain.VMXML{}, domain.ErrInvalidInput
	}
	return s.repository.GetVMXML(ctx, uuid, parsed)
}

func (s *Service) Export(ctx context.Context, command ExportCommand) (domain.VMExport, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return domain.VMExport{}, domain.ErrInvalidInput
	}

	detail := map[string]any{"uuid": command.UUID, "formatVersion": domain.VMExportFormatVersion}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return s.exportFailure(ctx, command, detail, err)
	}
	detail["name"] = vm.Name

	configXML, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatConfig)
	if err != nil {
		return s.exportFailure(ctx, command, detail, err)
	}

	var liveXML *domain.VMXML
	includeLiveXML := vmHasLiveXML(vm.State)
	detail["includeLiveXml"] = includeLiveXML
	if includeLiveXML {
		result, err := s.repository.GetVMXML(ctx, command.UUID, domain.VMXMLFormatLive)
		if err != nil {
			return s.exportFailure(ctx, command, detail, err)
		}
		liveXML = &result
	}

	snapshots, err := s.ListSnapshots(ctx, command.UUID)
	if err != nil {
		return s.exportFailure(ctx, command, detail, err)
	}
	detail["snapshotCount"] = snapshots.Total

	exportedAt := time.Now().UTC()
	result := domain.VMExport{
		FormatVersion: domain.VMExportFormatVersion,
		ExportedAt:    exportedAt,
		VM:            vm,
		ConfigXML:     configXML,
		LiveXML:       liveXML,
		Snapshots:     snapshots,
		Disclaimer:    vmExportDisclaimer,
	}
	if err := s.recordMutationAudit(ctx, command.Request, "vm.export", command.UUID, "success", detail, nil); err != nil {
		return domain.VMExport{}, fmt.Errorf("record VM export audit: %w", err)
	}
	return result, nil
}

func vmHasLiveXML(state domain.VMState) bool {
	switch state {
	case domain.VMStateRunning, domain.VMStateBlocked, domain.VMStatePaused, domain.VMStateShutdown, domain.VMStatePMSuspended:
		return true
	default:
		return false
	}
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (domain.VM, error) {
	spec := command.Spec
	spec.Name = strings.TrimSpace(spec.Name)
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.create", spec.Name, specDetail(spec), domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.create", spec.Name, specDetail(spec), domain.ErrReadOnly)
	}
	normalized, err := normalizeCreateSpec(spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.create", spec.Name, specDetail(spec), err)
	}
	vm, err := s.repository.CreateVM(ctx, normalized)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.create", normalized.Name, specDetail(normalized), err)
	}
	detail := specDetail(normalized)
	detail["uuid"] = vm.UUID
	detail["state"] = string(vm.State)
	s.captureConfigVersion(ctx, command.Request, vm.UUID, domain.VMConfigSourceCreate, 0, "创建虚拟机后保存初始配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.create", vm.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM create audit: %w", err)
	}
	return vm, nil
}

func (s *Service) Delete(ctx context.Context, command DeleteCommand) error {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return s.mutationFailure(ctx, command.Request, "vm.delete", command.UUID, nil, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.mutationFailure(ctx, command.Request, "vm.delete", command.UUID, nil, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return s.mutationFailure(ctx, command.Request, "vm.delete", command.UUID, nil, err)
	}
	detail := map[string]any{"name": vm.Name, "state": string(vm.State)}
	if vm.State != domain.VMStateShutoff && vm.State != domain.VMStateCrashed {
		return s.mutationFailure(ctx, command.Request, "vm.delete", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	if err := s.repository.DeleteVM(ctx, command.UUID); err != nil {
		return s.mutationFailure(ctx, command.Request, "vm.delete", command.UUID, detail, err)
	}
	if s.metadata != nil {
		if err := s.metadata.DeleteVMMetadata(ctx, command.UUID); err != nil {
			return fmt.Errorf("delete VM metadata: %w", err)
		}
	}
	if err := s.recordMutationAudit(ctx, command.Request, "vm.delete", command.UUID, "success", detail, nil); err != nil {
		return fmt.Errorf("record VM delete audit: %w", err)
	}
	return nil
}

func (s *Service) SetAutostart(ctx context.Context, command SetAutostartCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"newAutostart": command.Enabled}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.autostart", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.autostart", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.autostart", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["previousAutostart"] = vm.Autostart
	if !vm.Persistent {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.autostart", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	if err := s.repository.SetVMAutostart(ctx, command.UUID, command.Enabled); err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.autostart", command.UUID, detail, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.autostart", command.UUID, detail, err)
	}
	if err := s.recordMutationAudit(ctx, command.Request, "vm.autostart", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM autostart audit: %w", err)
	}
	return updated, nil
}

func (s *Service) UpdateResources(ctx context.Context, command UpdateResourcesCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{
		"vcpus":     command.Spec.VCPUs,
		"memoryMiB": command.Spec.MemoryMiB,
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail, domain.ErrReadOnly)
	}
	if command.Spec.VCPUs == 0 || command.Spec.VCPUs > maxVCPUs {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail,
			fmt.Errorf("%w: vCPUs must be between 1 and %d", domain.ErrInvalidInput, maxVCPUs))
	}
	if command.Spec.MemoryMiB < minMemoryMiB || command.Spec.MemoryMiB > maxMemoryMiB {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail,
			fmt.Errorf("%w: memory must be between %d and %d MiB", domain.ErrInvalidInput, minMemoryMiB, maxMemoryMiB))
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	detail["previousVCPUs"] = vm.VCPUs
	detail["previousMemoryMiB"] = vm.MemoryBytes / (1024 * 1024)
	if !vm.Persistent {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	switch vm.State {
	case domain.VMStateShutdown, domain.VMStateUnknown:
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	targetMemoryBytes := command.Spec.MemoryMiB * 1024 * 1024
	if vmResourcesLive(vm.State) {
		maxVCPUs := vm.MaxVCPUs
		if maxVCPUs == 0 {
			maxVCPUs = vm.VCPUs
		}
		maxMemoryBytes := vm.MaxMemoryBytes
		if maxMemoryBytes == 0 {
			maxMemoryBytes = vm.MemoryBytes
		}
		detail["maxVCPUs"] = maxVCPUs
		detail["maxMemoryMiB"] = maxMemoryBytes / (1024 * 1024)
		if command.Spec.VCPUs > maxVCPUs || targetMemoryBytes > maxMemoryBytes {
			return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail, domain.ErrActionNotAllowed)
		}
	}
	updated, err := s.repository.UpdateVMResources(ctx, command.UUID, command.Spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.resources.update", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceResources, 0, "调整 CPU 或内存后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.resources.update", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM resources audit: %w", err)
	}
	return updated, nil
}

func vmResourcesLive(state domain.VMState) bool {
	switch state {
	case domain.VMStateRunning, domain.VMStateBlocked, domain.VMStatePaused, domain.VMStatePMSuspended:
		return true
	default:
		return false
	}
}

func (s *Service) ResizeDisk(ctx context.Context, command ResizeDiskCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Target = strings.TrimSpace(command.Target)
	if command.UUID == "" || !diskTargetPattern.MatchString(command.Target) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{
		"target":        command.Target,
		"capacityBytes": command.Spec.CapacityBytes,
	}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, domain.ErrReadOnly)
	}
	if command.Spec.CapacityBytes < minDiskCapacityBytes || command.Spec.CapacityBytes > maxDiskCapacityBytes {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail,
			fmt.Errorf("%w: disk capacity must be between %d and %d bytes", domain.ErrInvalidInput, minDiskCapacityBytes, maxDiskCapacityBytes))
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	if !vm.Persistent {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	switch vm.State {
	case domain.VMStateShutdown, domain.VMStateUnknown:
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	var disk *domain.Disk
	for index := range vm.Disks {
		if vm.Disks[index].Target == command.Target {
			disk = &vm.Disks[index]
			break
		}
	}
	if disk == nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, domain.ErrNotFound)
	}
	detail["device"] = disk.Device
	detail["format"] = disk.Format
	detail["source"] = disk.Source
	detail["previousCapacityBytes"] = disk.CapacityBytes
	if disk.Device != "disk" || disk.ReadOnly || strings.TrimSpace(disk.Source) == "" {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	if disk.CapacityBytes == 0 {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, domain.ErrDiskResize)
	}
	if command.Spec.CapacityBytes <= disk.CapacityBytes {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail,
			fmt.Errorf("%w: new capacity must exceed current capacity", domain.ErrInvalidInput))
	}
	updated, err := s.repository.ResizeVMDisk(ctx, command.UUID, command.Target, command.Spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.resize", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceDiskResize, 0, "扩容磁盘后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.disk.resize", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM disk resize audit: %w", err)
	}
	return updated, nil
}

func (s *Service) AttachDisk(ctx context.Context, command AttachDiskCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Pool = strings.TrimSpace(command.Spec.Pool)
	command.Spec.Volume = strings.TrimSpace(command.Spec.Volume)
	command.Spec.Target = strings.TrimSpace(command.Spec.Target)
	bus, ok := domain.NormalizeVMDiskBus(string(command.Spec.Bus))
	if !ok {
		return domain.VM{}, domain.ErrInvalidInput
	}
	command.Spec.Bus = bus
	if command.UUID == "" || !storagePoolNamePattern.MatchString(command.Spec.Pool) ||
		!storageVolumeNamePattern.MatchString(command.Spec.Volume) ||
		(command.Spec.Target != "" && !diskTargetPattern.MatchString(command.Spec.Target)) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"pool": command.Spec.Pool, "volume": command.Spec.Volume, "bus": command.Spec.Bus, "target": command.Spec.Target}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.attach", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.attach", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.attach", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	if command.Spec.Target != "" {
		for _, disk := range vm.Disks {
			if disk.Target == command.Spec.Target {
				return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.attach", command.UUID, detail, domain.ErrConflict)
			}
		}
	}
	hotplug, ok := s.repository.(HotplugRepository)
	if !ok {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.attach", command.UUID, detail, domain.ErrUnavailable)
	}
	updated, err := hotplug.AttachVMDisk(ctx, command.UUID, command.Spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.attach", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceDiskHotplug, 0, "热附加磁盘后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.disk.attach", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM disk attach audit: %w", err)
	}
	return updated, nil
}

func (s *Service) DetachDisk(ctx context.Context, command DetachDiskCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Target = strings.TrimSpace(command.Target)
	if command.UUID == "" || !diskTargetPattern.MatchString(command.Target) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"target": command.Target}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.detach", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.detach", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.detach", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	found := false
	for _, disk := range vm.Disks {
		if disk.Target == command.Target && disk.Device == "disk" && !disk.ReadOnly {
			found = true
			detail["source"] = disk.Source
			detail["format"] = disk.Format
			break
		}
	}
	if !found {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.detach", command.UUID, detail, domain.ErrNotFound)
	}
	hotplug, ok := s.repository.(HotplugRepository)
	if !ok {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.detach", command.UUID, detail, domain.ErrUnavailable)
	}
	updated, err := hotplug.DetachVMDisk(ctx, command.UUID, command.Target)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.disk.detach", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceDiskHotplug, 0, "热移除磁盘后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.disk.detach", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM disk detach audit: %w", err)
	}
	return updated, nil
}

func (s *Service) AttachNetwork(ctx context.Context, command AttachNetworkCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Network = strings.TrimSpace(command.Spec.Network)
	model, ok := domain.NormalizeVMNICModel(string(command.Spec.Model))
	if !ok {
		return domain.VM{}, domain.ErrInvalidInput
	}
	command.Spec.Model = model
	if command.UUID == "" || !networkPattern.MatchString(command.Spec.Network) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"network": command.Spec.Network, "model": command.Spec.Model}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.attach", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.attach", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.attach", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	hotplug, ok := s.repository.(HotplugRepository)
	if !ok {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.attach", command.UUID, detail, domain.ErrUnavailable)
	}
	updated, err := hotplug.AttachVMNetwork(ctx, command.UUID, command.Spec)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.attach", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceNetworkHotplug, 0, "热附加网卡后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.network.attach", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM network attach audit: %w", err)
	}
	return updated, nil
}

func (s *Service) DetachNetwork(ctx context.Context, command DetachNetworkCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.MAC = strings.TrimSpace(command.MAC)
	if command.UUID == "" || !macAddressPattern.MatchString(command.MAC) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"mac": command.MAC}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.detach", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.detach", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.detach", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	found := false
	for _, networkInterface := range vm.Interfaces {
		if strings.EqualFold(networkInterface.MAC, command.MAC) {
			found = true
			detail["network"] = networkInterface.Source
			detail["model"] = networkInterface.Model
			break
		}
	}
	if !found {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.detach", command.UUID, detail, domain.ErrNotFound)
	}
	hotplug, ok := s.repository.(HotplugRepository)
	if !ok {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.detach", command.UUID, detail, domain.ErrUnavailable)
	}
	updated, err := hotplug.DetachVMNetwork(ctx, command.UUID, command.MAC)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.network.detach", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceNetworkHotplug, 0, "热移除网卡后保存持久配置")
	if err := s.recordMutationAudit(ctx, command.Request, "vm.network.detach", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record VM network detach audit: %w", err)
	}
	return updated, nil
}

func (s *Service) AttachHostDevice(ctx context.Context, command HostDeviceCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Device = strings.TrimSpace(command.Device)
	if command.UUID == "" || !hostDeviceNamePattern.MatchString(command.Device) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"device": command.Device}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.attach", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.attach", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.attach", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	for _, attached := range vm.Passthrough {
		if attached.Name == command.Device {
			detail["attachedVm"] = vm.Name
			return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.attach", command.UUID, detail, domain.ErrConflict)
		}
	}
	vms, err := s.repository.ListVMs(ctx)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.attach", command.UUID, detail, err)
	}
	for _, candidate := range vms {
		if candidate.UUID == command.UUID {
			continue
		}
		for _, attached := range candidate.Passthrough {
			if attached.Name == command.Device {
				detail["attachedVm"] = candidate.Name
				return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.attach", command.UUID, detail, domain.ErrConflict)
			}
		}
	}
	updated, err := s.repository.AttachHostDevice(ctx, command.UUID, command.Device)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.attach", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourcePassthrough, 0, "添加 PCI/USB 直通设备后保存持久配置")
	addPassthroughAuditDetail(detail, updated, command.Device)
	if err := s.recordMutationAudit(ctx, command.Request, "vm.device.attach", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record host device attach audit: %w", err)
	}
	return updated, nil
}

func (s *Service) DetachHostDevice(ctx context.Context, command HostDeviceCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Device = strings.TrimSpace(command.Device)
	if command.UUID == "" || !hostDeviceNamePattern.MatchString(command.Device) {
		return domain.VM{}, domain.ErrInvalidInput
	}
	detail := map[string]any{"device": command.Device}
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.detach", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.detach", command.UUID, detail, domain.ErrReadOnly)
	}
	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.detach", command.UUID, detail, err)
	}
	detail["name"] = vm.Name
	if !hasPassthroughDevice(vm.Passthrough, command.Device) {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.detach", command.UUID, detail, domain.ErrNotFound)
	}
	updated, err := s.repository.DetachHostDevice(ctx, command.UUID, command.Device)
	if err != nil {
		return domain.VM{}, s.mutationFailure(ctx, command.Request, "vm.device.detach", command.UUID, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourcePassthrough, 0, "移除 PCI/USB 直通设备后保存持久配置")
	addPassthroughAuditDetail(detail, vm, command.Device)
	if err := s.recordMutationAudit(ctx, command.Request, "vm.device.detach", command.UUID, "success", detail, nil); err != nil {
		return domain.VM{}, fmt.Errorf("record host device detach audit: %w", err)
	}
	return updated, nil
}

func hasPassthroughDevice(devices []domain.PassthroughDevice, name string) bool {
	for _, device := range devices {
		if device.Name == name {
			return true
		}
	}
	return false
}

func addPassthroughAuditDetail(detail map[string]any, vm domain.VM, name string) {
	for _, device := range vm.Passthrough {
		if device.Name != name {
			continue
		}
		detail["deviceType"] = device.Type
		if device.VendorID != "" {
			detail["vendorId"] = device.VendorID
		}
		if device.ProductID != "" {
			detail["productId"] = device.ProductID
		}
		if device.Address != "" {
			detail["address"] = device.Address
		}
		return
	}
}

func (s *Service) PerformAction(ctx context.Context, command ActionCommand) (domain.VM, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	if command.UUID == "" {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if _, ok := domain.ValidVMAction(string(command.Action)); !ok {
		return domain.VM{}, domain.ErrInvalidInput
	}
	if !domain.CanOperateVMs(command.Session.User.Role) {
		return domain.VM{}, s.auditFailure(ctx, command, nil, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VM{}, s.auditFailure(ctx, command, nil, domain.ErrReadOnly)
	}

	vm, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.auditFailure(ctx, command, nil, err)
	}
	if !domain.ContainsVMAction(vm.AllowedActions, command.Action) {
		return domain.VM{}, s.auditFailure(ctx, command, &vm, domain.ErrActionNotAllowed)
	}
	if err := s.repository.PerformAction(ctx, command.UUID, command.Action); err != nil {
		return domain.VM{}, s.auditFailure(ctx, command, &vm, err)
	}
	updated, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.VM{}, s.auditFailure(ctx, command, &vm, err)
	}
	if err := s.recordAudit(ctx, command, vm, "success", string(updated.State), nil); err != nil {
		return domain.VM{}, fmt.Errorf("record successful VM action audit: %w", err)
	}
	return updated, nil
}

func (s *Service) ListSnapshots(ctx context.Context, uuid string) (domain.VMSnapshotList, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return domain.VMSnapshotList{}, domain.ErrInvalidInput
	}
	snapshots, err := s.repository.ListSnapshots(ctx, uuid)
	if err != nil {
		return domain.VMSnapshotList{}, err
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].CreatedAt != snapshots[j].CreatedAt {
			return snapshots[i].CreatedAt > snapshots[j].CreatedAt
		}
		return strings.ToLower(snapshots[i].Name) < strings.ToLower(snapshots[j].Name)
	})
	return domain.VMSnapshotList{Items: snapshots, Total: len(snapshots)}, nil
}

func (s *Service) CreateSnapshot(ctx context.Context, command SnapshotCreateCommand) (domain.VMSnapshot, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	command.Spec.Description = strings.TrimSpace(command.Spec.Description)
	detail := snapshotDetail(command.UUID, command.Spec.Name, command.Spec.Description)
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.VMSnapshot{}, s.snapshotFailure(ctx, command.Request, "vm.snapshot.create", command.UUID+"/"+command.Spec.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.VMSnapshot{}, s.snapshotFailure(ctx, command.Request, "vm.snapshot.create", command.UUID+"/"+command.Spec.Name, detail, domain.ErrReadOnly)
	}
	if command.UUID == "" || !snapshotPattern.MatchString(command.Spec.Name) || len(command.Spec.Description) > 1024 {
		return domain.VMSnapshot{}, s.snapshotFailure(ctx, command.Request, "vm.snapshot.create", command.UUID+"/"+command.Spec.Name, detail, domain.ErrInvalidInput)
	}
	snapshot, err := s.repository.CreateSnapshot(ctx, command.UUID, command.Spec)
	if err != nil {
		return domain.VMSnapshot{}, s.snapshotFailure(ctx, command.Request, "vm.snapshot.create", command.UUID+"/"+command.Spec.Name, detail, err)
	}
	detail["stateAtSnapshot"] = string(snapshot.State)
	detail["createdAt"] = snapshot.CreatedAt
	if err := s.recordSnapshotAudit(ctx, command.Request, "vm.snapshot.create", command.UUID+"/"+snapshot.Name, "success", detail, nil); err != nil {
		return domain.VMSnapshot{}, fmt.Errorf("record VM snapshot create audit: %w", err)
	}
	return snapshot, nil
}

func (s *Service) RevertSnapshot(ctx context.Context, command SnapshotCommand) error {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Name = strings.TrimSpace(command.Name)
	detail := snapshotDetail(command.UUID, command.Name, "")
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.revert", command.UUID+"/"+command.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.revert", command.UUID+"/"+command.Name, detail, domain.ErrReadOnly)
	}
	if command.UUID == "" || !snapshotPattern.MatchString(command.Name) {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.revert", command.UUID+"/"+command.Name, detail, domain.ErrInvalidInput)
	}
	if err := s.repository.RevertSnapshot(ctx, command.UUID, command.Name); err != nil {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.revert", command.UUID+"/"+command.Name, detail, err)
	}
	s.captureConfigVersion(ctx, command.Request, command.UUID, domain.VMConfigSourceSnapshotRevert, 0, "回滚快照后保存持久配置")
	if err := s.recordSnapshotAudit(ctx, command.Request, "vm.snapshot.revert", command.UUID+"/"+command.Name, "success", detail, nil); err != nil {
		return fmt.Errorf("record VM snapshot revert audit: %w", err)
	}
	return nil
}

func (s *Service) DeleteSnapshot(ctx context.Context, command SnapshotCommand) error {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Name = strings.TrimSpace(command.Name)
	detail := snapshotDetail(command.UUID, command.Name, "")
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.delete", command.UUID+"/"+command.Name, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.delete", command.UUID+"/"+command.Name, detail, domain.ErrReadOnly)
	}
	if command.UUID == "" || !snapshotPattern.MatchString(command.Name) {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.delete", command.UUID+"/"+command.Name, detail, domain.ErrInvalidInput)
	}
	if err := s.repository.DeleteSnapshot(ctx, command.UUID, command.Name); err != nil {
		return s.snapshotFailure(ctx, command.Request, "vm.snapshot.delete", command.UUID+"/"+command.Name, detail, err)
	}
	if err := s.recordSnapshotAudit(ctx, command.Request, "vm.snapshot.delete", command.UUID+"/"+command.Name, "success", detail, nil); err != nil {
		return fmt.Errorf("record VM snapshot delete audit: %w", err)
	}
	return nil
}

func snapshotDetail(uuid, name, description string) map[string]any {
	detail := map[string]any{"vmUuid": uuid, "name": name}
	if description != "" {
		detail["description"] = description
	}
	return detail
}

func (s *Service) auditFailure(ctx context.Context, command ActionCommand, vm *domain.VM, cause error) error {
	var target domain.VM
	if vm != nil {
		target = *vm
	}
	if err := s.recordAudit(ctx, command, target, "failure", "", cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record failed VM action audit: %w", err))
	}
	return cause
}

func (s *Service) recordAudit(ctx context.Context, command ActionCommand, vm domain.VM, result, nextState string, cause error) error {
	detail := map[string]any{
		"action": string(command.Action),
	}
	if vm.State != "" {
		detail["previousState"] = string(vm.State)
	}
	if nextState != "" {
		detail["newState"] = nextState
	}
	if cause != nil {
		detail["error"] = cause.Error()
	}
	return s.recordMutationAudit(ctx, MutationContext{
		Session:   command.Session,
		RequestID: command.RequestID,
		SourceIP:  command.SourceIP,
	}, "vm."+string(command.Action), command.UUID, result, detail, cause)
}

func (s *Service) mutationFailure(ctx context.Context, request MutationContext, action, targetID string, detail map[string]any, cause error) error {
	if err := s.recordMutationAudit(ctx, request, action, targetID, "failure", detail, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record %s audit: %w", action, err))
	}
	return cause
}

func (s *Service) exportFailure(ctx context.Context, command ExportCommand, detail map[string]any, cause error) (domain.VMExport, error) {
	if err := s.recordMutationAudit(ctx, command.Request, "vm.export", command.UUID, "failure", detail, cause); err != nil {
		return domain.VMExport{}, errors.Join(cause, fmt.Errorf("record vm.export audit: %w", err))
	}
	return domain.VMExport{}, cause
}

func (s *Service) recordMutationAudit(ctx context.Context, request MutationContext, action, targetID, result string, detail map[string]any, cause error) error {
	return s.recordMutationAuditForKind(ctx, request, "vm", action, targetID, result, detail, cause)
}

func (s *Service) recordSnapshotAudit(ctx context.Context, request MutationContext, action, targetID, result string, detail map[string]any, cause error) error {
	return s.recordMutationAuditForKind(ctx, request, "vm_snapshot", action, targetID, result, detail, cause)
}

func (s *Service) recordMutationAuditForKind(ctx context.Context, request MutationContext, targetKind, action, targetID, result string, detail map[string]any, cause error) error {
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

func (s *Service) snapshotFailure(ctx context.Context, request MutationContext, action, targetID string, detail map[string]any, cause error) error {
	if err := s.recordSnapshotAudit(ctx, request, action, targetID, "failure", detail, cause); err != nil {
		return errors.Join(cause, fmt.Errorf("record %s audit: %w", action, err))
	}
	return cause
}

func normalizeVMMetadataSpec(spec domain.VMMetadataSpec) (domain.VMMetadataSpec, error) {
	tags := make([]string, 0, len(spec.Tags))
	seen := make(map[string]struct{}, len(spec.Tags))
	for _, raw := range spec.Tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if len(tag) > maxVMMetadataTagBytes || strings.IndexFunc(tag, unicode.IsControl) >= 0 {
			return domain.VMMetadataSpec{}, fmt.Errorf("%w: invalid VM tag", domain.ErrInvalidInput)
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		tags = append(tags, tag)
	}
	if len(tags) > maxVMMetadataTags {
		return domain.VMMetadataSpec{}, fmt.Errorf("%w: too many VM tags", domain.ErrInvalidInput)
	}
	spec.Tags = tags
	spec.Note = strings.TrimSpace(spec.Note)
	spec.Owner = strings.TrimSpace(spec.Owner)
	if len(spec.Note) > maxVMMetadataNoteBytes || len(spec.Owner) > maxVMMetadataOwnerBytes ||
		strings.IndexFunc(spec.Note, isNUL) >= 0 || strings.IndexFunc(spec.Owner, unicode.IsControl) >= 0 {
		return domain.VMMetadataSpec{}, domain.ErrInvalidInput
	}
	return spec, nil
}

func isNUL(value rune) bool {
	return value == 0
}

func vmMatchesSearch(vm domain.VM, search string) bool {
	if strings.Contains(strings.ToLower(vm.Name), search) || strings.Contains(strings.ToLower(vm.UUID), search) {
		return true
	}
	if vm.Metadata == nil {
		return false
	}
	if strings.Contains(strings.ToLower(vm.Metadata.Owner), search) || strings.Contains(strings.ToLower(vm.Metadata.Note), search) {
		return true
	}
	for _, tag := range vm.Metadata.Tags {
		if strings.Contains(strings.ToLower(tag), search) {
			return true
		}
	}
	return false
}

func vmHasTag(vm domain.VM, tag string) bool {
	if vm.Metadata == nil {
		return false
	}
	for _, candidate := range vm.Metadata.Tags {
		if strings.ToLower(candidate) == tag {
			return true
		}
	}
	return false
}

func normalizeCreateSpec(spec domain.VMCreateSpec) (domain.VMCreateSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.DiskPath = strings.TrimSpace(spec.DiskPath)
	spec.DiskFormat = strings.ToLower(strings.TrimSpace(spec.DiskFormat))
	spec.Network = strings.TrimSpace(spec.Network)
	spec.ISOPath = strings.TrimSpace(spec.ISOPath)
	if spec.DiskFormat == "" {
		spec.DiskFormat = "qcow2"
	}
	if spec.Network == "" {
		spec.Network = "default"
	}
	if !vmNamePattern.MatchString(spec.Name) {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: invalid VM name", domain.ErrInvalidInput)
	}
	if spec.VCPUs == 0 || spec.VCPUs > maxVCPUs {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: vCPUs must be between 1 and %d", domain.ErrInvalidInput, maxVCPUs)
	}
	if spec.MemoryMiB < minMemoryMiB || spec.MemoryMiB > maxMemoryMiB {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: memory must be between %d and %d MiB", domain.ErrInvalidInput, minMemoryMiB, maxMemoryMiB)
	}
	if !validAbsolutePath(spec.DiskPath) {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: disk path must be absolute", domain.ErrInvalidInput)
	}
	spec.DiskPath = filepath.Clean(spec.DiskPath)
	if _, ok := supportedFormats[spec.DiskFormat]; !ok {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: disk format must be qcow2 or raw", domain.ErrInvalidInput)
	}
	diskBus, ok := domain.NormalizeVMDiskBus(string(spec.DiskBus))
	if !ok {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: disk bus must be virtio, sata or scsi", domain.ErrInvalidInput)
	}
	spec.DiskBus = diskBus
	if !networkPattern.MatchString(spec.Network) {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: invalid network name", domain.ErrInvalidInput)
	}
	nicModel, ok := domain.NormalizeVMNICModel(string(spec.NICModel))
	if !ok {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: NIC model must be virtio or e1000", domain.ErrInvalidInput)
	}
	spec.NICModel = nicModel
	if spec.ISOPath != "" {
		if !validAbsolutePath(spec.ISOPath) {
			return domain.VMCreateSpec{}, fmt.Errorf("%w: ISO path must be absolute", domain.ErrInvalidInput)
		}
		spec.ISOPath = filepath.Clean(spec.ISOPath)
	}
	bootOrder, ok := domain.NormalizeVMBootOrder(string(spec.BootOrder))
	if !ok {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: boot order must be auto, disk-first or cdrom-first", domain.ErrInvalidInput)
	}
	if bootOrder == domain.VMBootOrderCDROMFirst && spec.ISOPath == "" {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: cdrom-first boot order requires an ISO path", domain.ErrInvalidInput)
	}
	spec.BootOrder = bootOrder
	if spec.CloudInit != nil {
		userData := strings.TrimSpace(spec.CloudInit.UserData)
		networkConfig := strings.TrimSpace(spec.CloudInit.NetworkConfig)
		vendorData := strings.TrimSpace(spec.CloudInit.VendorData)
		pool := strings.TrimSpace(spec.CloudInit.Pool)
		total := len(userData) + len(networkConfig) + len(vendorData)
		if userData == "" || total > maxCloudInitUserDataBytes || containsCloudInitNUL(userData, networkConfig, vendorData) {
			return domain.VMCreateSpec{}, fmt.Errorf("%w: cloud-init data must include user-data and be at most %d bytes in total with no NUL", domain.ErrInvalidInput, maxCloudInitUserDataBytes)
		}
		if !cloudInitPoolPattern.MatchString(pool) {
			return domain.VMCreateSpec{}, fmt.Errorf("%w: invalid cloud-init storage pool", domain.ErrInvalidInput)
		}
		spec.CloudInit = &domain.CloudInitSpec{UserData: userData, NetworkConfig: networkConfig, VendorData: vendorData, Pool: pool}
	}
	firmware, ok := domain.NormalizeVMFirmware(string(spec.Firmware))
	if !ok {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: firmware must be bios or uefi", domain.ErrInvalidInput)
	}
	if spec.SecureBoot && firmware != domain.VMFirmwareUEFI {
		return domain.VMCreateSpec{}, fmt.Errorf("%w: secure boot requires UEFI firmware", domain.ErrInvalidInput)
	}
	spec.Firmware = firmware
	return spec, nil
}

func containsCloudInitNUL(values ...string) bool {
	for _, value := range values {
		if strings.ContainsRune(value, '\x00') {
			return true
		}
	}
	return false
}

func validAbsolutePath(path string) bool {
	return path != "" && !strings.ContainsRune(path, '\x00') && filepath.IsAbs(path)
}

func specDetail(spec domain.VMCreateSpec) map[string]any {
	detail := map[string]any{
		"name":       spec.Name,
		"vcpus":      spec.VCPUs,
		"memoryMiB":  spec.MemoryMiB,
		"diskPath":   spec.DiskPath,
		"diskFormat": spec.DiskFormat,
		"diskBus":    spec.DiskBus,
		"network":    spec.Network,
		"nicModel":   spec.NICModel,
		"bootOrder":  spec.BootOrder,
		"firmware":   spec.Firmware,
		"secureBoot": spec.SecureBoot,
		"cloudInit":  spec.CloudInit != nil,
		"autostart":  spec.Autostart,
		"start":      spec.Start,
	}
	if spec.ISOPath != "" {
		detail["isoPath"] = spec.ISOPath
	}
	if spec.CloudInit != nil {
		detail["cloudInitPool"] = spec.CloudInit.Pool
		detail["cloudInitUserDataBytes"] = len(spec.CloudInit.UserData)
	}
	return detail
}
