package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"virtmgr/internal/domain"
	"virtmgr/internal/version"
)

const (
	maxBackupPackageBytes         = uint64(512 << 30)
	maxBackupPackageUncompressed  = uint64(2 << 40)
	maxBackupPackageMetadataBytes = uint64(8 << 20)
	maxBackupPackageEntries       = 10000
	backupPackageConfigPath       = "domain.xml"
	backupPackageManifestPath     = "package.json"
)

const MaxImportPackageBytes = maxBackupPackageBytes

type ExportCommand struct {
	ID      string
	Writer  io.Writer
	Request MutationContext
}

type ImportCommand struct {
	Spec    domain.BackupImportSpec
	Reader  io.Reader
	Request MutationContext
}

// Export writes a self-contained VirtMgr backup archive. The archive contains
// a package manifest, the inactive domain XML and every backup disk artifact.
func (s *Service) Export(ctx context.Context, command ExportCommand) (resultErr error) {
	command.ID = strings.TrimSpace(command.ID)
	detail := map[string]any{"backupId": command.ID}
	defer func() {
		if resultErr != nil {
			_ = s.recordAudit(ctx, command.Request, "backup.export", command.ID, "failure", detail, resultErr)
		}
	}()
	if command.ID == "" || command.Writer == nil {
		return domain.ErrInvalidInput
	}
	if !domain.CanCreateBackups(command.Request.Session.User.Role) {
		return domain.ErrForbidden
	}
	item, err := s.repository.GetBackup(ctx, command.ID)
	if err != nil {
		return err
	}
	if item.Status != domain.BackupStatusCompleted {
		return fmt.Errorf("%w: only completed backups can be exported", domain.ErrActionNotAllowed)
	}
	detail["name"] = item.Name
	detail["vmName"] = item.VMName
	volumes, err := s.backend.ListStorageVolumes(ctx, item.TargetPool)
	if err != nil {
		return err
	}
	available := make(map[string]struct{}, len(volumes))
	for _, volume := range volumes {
		available[volume.Name] = struct{}{}
	}
	if _, ok := available[item.ConfigVolume]; !ok {
		return fmt.Errorf("%w: backup config artifact is missing", domain.ErrNotFound)
	}
	packageManifest := domain.BackupPackageManifest{
		FormatVersion: domain.BackupPackageFormatVersion,
		ExportedAt:    s.now().UTC(),
		Backup:        item,
		ConfigPath:    backupPackageConfigPath,
		Disks:         make([]domain.BackupPackageDisk, 0, len(item.Disks)),
	}
	for index, disk := range item.Disks {
		if _, ok := available[disk.Artifact]; !ok {
			return fmt.Errorf("%w: backup disk artifact %s is missing", domain.ErrNotFound, disk.Artifact)
		}
		packageManifest.Disks = append(packageManifest.Disks, domain.BackupPackageDisk{
			BackupDisk:  disk,
			ArchivePath: backupPackageDiskPath(index, disk.Format, disk.Artifact),
		})
	}
	manifestBytes, err := json.MarshalIndent(packageManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backup package manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	archive := zip.NewWriter(command.Writer)
	if err := writeZipBytes(archive, backupPackageManifestPath, manifestBytes); err != nil {
		return err
	}
	if err := writeZipVolume(ctx, s.backend, archive, backupPackageConfigPath, item.TargetPool, item.ConfigVolume, item.ConfigBytes); err != nil {
		return err
	}
	for _, disk := range packageManifest.Disks {
		if err := writeZipVolume(ctx, s.backend, archive, disk.ArchivePath, item.TargetPool, disk.Artifact, 0); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("finish backup package: %w", err)
	}
	detail["diskCount"] = len(item.Disks)
	detail["formatVersion"] = domain.BackupPackageFormatVersion
	if err := s.recordAudit(ctx, command.Request, "backup.export", command.ID, "success", detail, nil); err != nil {
		return fmt.Errorf("record backup export audit: %w", err)
	}
	return nil
}

// Import validates and restores a previously exported package into the backup
// catalog. New IDs and artifact names are generated so imports never overwrite
// an existing backup.
func (s *Service) Import(ctx context.Context, command ImportCommand) (result domain.Backup, resultErr error) {
	detail := map[string]any{"repositoryId": strings.TrimSpace(command.Spec.RepositoryID)}
	defer func() {
		if resultErr != nil {
			_ = s.recordAudit(ctx, command.Request, "backup.import", command.Spec.Name, "failure", detail, resultErr)
		}
	}()
	if command.Reader == nil || !domain.CanCreateBackups(command.Request.Session.User.Role) {
		if command.Reader == nil {
			return domain.Backup{}, domain.ErrInvalidInput
		}
		return domain.Backup{}, domain.ErrForbidden
	}
	if s.isReadOnly() {
		return domain.Backup{}, domain.ErrReadOnly
	}
	temp, err := os.CreateTemp("", "virtmgr-backup-import-*.zip")
	if err != nil {
		return domain.Backup{}, fmt.Errorf("create backup import staging file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	written, err := io.Copy(temp, io.LimitReader(command.Reader, int64(maxBackupPackageBytes)+1))
	if err != nil {
		return domain.Backup{}, fmt.Errorf("receive backup package: %w", err)
	}
	if uint64(written) > maxBackupPackageBytes {
		return domain.Backup{}, domain.ErrTooLarge
	}
	if written == 0 {
		return domain.Backup{}, domain.ErrInvalidInput
	}
	if err := temp.Close(); err != nil {
		return domain.Backup{}, fmt.Errorf("close backup import staging file: %w", err)
	}
	archive, err := zip.OpenReader(tempPath)
	if err != nil {
		return domain.Backup{}, fmt.Errorf("%w: invalid backup zip package", domain.ErrInvalidInput)
	}
	defer archive.Close()
	entries, err := indexBackupArchive(archive.File)
	if err != nil {
		return domain.Backup{}, err
	}
	packageFile, ok := entries[backupPackageManifestPath]
	if !ok {
		return domain.Backup{}, fmt.Errorf("%w: backup package manifest is missing", domain.ErrInvalidInput)
	}
	manifestBytes, err := readZipEntry(packageFile, maxBackupPackageMetadataBytes)
	if err != nil {
		return domain.Backup{}, err
	}
	var packageManifest domain.BackupPackageManifest
	if err := json.Unmarshal(manifestBytes, &packageManifest); err != nil {
		return domain.Backup{}, fmt.Errorf("%w: invalid backup package manifest", domain.ErrInvalidInput)
	}
	if packageManifest.FormatVersion != domain.BackupPackageFormatVersion || packageManifest.ConfigPath != backupPackageConfigPath || len(packageManifest.Disks) == 0 {
		return domain.Backup{}, fmt.Errorf("%w: unsupported backup package format", domain.ErrInvalidInput)
	}
	if packageManifest.Backup.DiskCount > 0 && packageManifest.Backup.DiskCount != len(packageManifest.Disks) {
		return domain.Backup{}, fmt.Errorf("%w: backup package disk count mismatch", domain.ErrInvalidInput)
	}
	configFile, ok := entries[packageManifest.ConfigPath]
	if !ok {
		return domain.Backup{}, fmt.Errorf("%w: backup domain XML is missing", domain.ErrInvalidInput)
	}
	configBytes, err := readZipEntry(configFile, maxBackupPackageMetadataBytes)
	if err != nil {
		return domain.Backup{}, err
	}
	if len(configBytes) == 0 {
		return domain.Backup{}, domain.ErrInvalidInput
	}
	name := strings.TrimSpace(command.Spec.Name)
	if name == "" {
		name = strings.TrimSpace(packageManifest.Backup.Name)
	}
	if err := domain.ValidateBackupName(name); err != nil {
		return domain.Backup{}, err
	}
	if err := validateBackupPackageDisks(packageManifest.Disks, entries); err != nil {
		return domain.Backup{}, err
	}
	repository, err := s.resolveRepository(ctx, domain.BackupCreateSpec{Name: name, RepositoryID: command.Spec.RepositoryID})
	if err != nil {
		return domain.Backup{}, err
	}
	backupID, err := newBackupID()
	if err != nil {
		return domain.Backup{}, err
	}
	detail["name"] = name
	detail["backupId"] = backupID
	detail["targetPool"] = repository.PoolName
	detail["diskCount"] = len(packageManifest.Disks)
	now := s.now().UTC()
	configName := backupVolumeNameForImport(backupID, "-domain.xml")
	manifestName := backupVolumeNameForImport(backupID, "-manifest.json")
	createdVolumes := make([]string, 0, len(packageManifest.Disks)+2)
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, volumeName := range createdVolumes {
			_ = s.backend.DeleteStorageVolume(context.WithoutCancel(ctx), repository.PoolName, volumeName)
		}
	}()
	manifestDisks := make([]domain.VMBackupDisk, 0, len(packageManifest.Disks))
	catalogDisks := make([]domain.BackupDisk, 0, len(packageManifest.Disks))
	totalBytes := uint64(0)
	for index, packageDisk := range packageManifest.Disks {
		diskFile, ok := entries[packageDisk.ArchivePath]
		if !ok {
			return domain.Backup{}, fmt.Errorf("%w: backup disk %d is missing", domain.ErrInvalidInput, index)
		}
		artifact := importDiskName(backupID, index, packageDisk.Format, packageDisk.Artifact)
		reader, err := diskFile.Open()
		if err != nil {
			return domain.Backup{}, fmt.Errorf("open backup disk %d: %w", index, err)
		}
		_, uploadErr := s.backend.UploadStorageVolume(ctx, repository.PoolName, domain.StorageVolumeCreateSpec{
			Name: artifact, Format: packageDisk.Format, CapacityBytes: packageDisk.CapacityBytes,
			UploadBytes: diskFile.UncompressedSize64,
		}, reader)
		_ = reader.Close()
		if uploadErr != nil {
			return domain.Backup{}, fmt.Errorf("import backup disk %d: %w", index, uploadErr)
		}
		createdVolumes = append(createdVolumes, artifact)
		manifestDisks = append(manifestDisks, domain.VMBackupDisk{
			SourceVolume: packageDisk.SourceVolume, BackupVolume: artifact, Target: packageDisk.Target,
			Format: packageDisk.Format, CapacityBytes: packageDisk.CapacityBytes, SHA256: packageDisk.SHA256,
		})
		catalogDisks = append(catalogDisks, domain.BackupDisk{
			Sequence: index, Target: packageDisk.Target, SourceVolume: packageDisk.SourceVolume,
			Artifact: artifact, Format: packageDisk.Format, CapacityBytes: packageDisk.CapacityBytes, SHA256: packageDisk.SHA256,
		})
		totalBytes += packageDisk.CapacityBytes
	}
	if _, err := s.backend.UploadStorageVolume(ctx, repository.PoolName, domain.StorageVolumeCreateSpec{
		Name: configName, Format: "raw", CapacityBytes: uint64(len(configBytes)),
	}, strings.NewReader(string(configBytes))); err != nil {
		return domain.Backup{}, fmt.Errorf("import backup domain XML: %w", err)
	}
	createdVolumes = append(createdVolumes, configName)
	configSum := sha256.Sum256(configBytes)
	createdAt := packageManifest.Backup.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	completedAt := packageManifest.Backup.CompletedAt
	if completedAt == nil {
		completedAt = &now
	}
	backupManifest := domain.BackupManifest{
		FormatVersion: domain.VMBackupFormatVersionV2,
		BackupID:      backupID,
		CreatedAt:     createdAt,
		ToolVersion:   version.Version,
		Source:        &domain.BackupManifestSource{VMUUID: packageManifest.Backup.VMUUID, VMName: packageManifest.Backup.VMName, State: domain.VMStateShutoff, Persistent: true},
		Config:        &domain.BackupManifestConfig{Artifact: configName, SizeBytes: uint64(len(configBytes)), SHA256: hex.EncodeToString(configSum[:])},
		Restore:       &domain.BackupManifestRestore{DefaultTargetPool: repository.PoolName, RequiresNewMAC: true, ContainsMemory: false},
		VMUUID:        packageManifest.Backup.VMUUID,
		VMName:        packageManifest.Backup.VMName,
		ConfigVolume:  configName,
		ConfigSHA256:  hex.EncodeToString(configSum[:]),
		Disks:         manifestDisks,
		Online:        packageManifest.Backup.Online,
		Quiesced:      packageManifest.Backup.Quiesced,
		Consistent:    packageManifest.Backup.Consistent,
		ContainsTPM:   packageManifest.Backup.ContainsTPM,
		TPMStateSaved: false,
	}
	if packageManifest.Backup.Online {
		backupManifest.Source.State = domain.VMStateRunning
	}
	manifestBytes, err = json.MarshalIndent(backupManifest, "", "  ")
	if err != nil {
		return domain.Backup{}, fmt.Errorf("encode imported backup manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if _, err := s.backend.UploadStorageVolume(ctx, repository.PoolName, domain.StorageVolumeCreateSpec{
		Name: manifestName, Format: "raw", CapacityBytes: uint64(len(manifestBytes)),
	}, strings.NewReader(string(manifestBytes))); err != nil {
		return domain.Backup{}, fmt.Errorf("import backup manifest: %w", err)
	}
	createdVolumes = append(createdVolumes, manifestName)
	totalBytes += uint64(len(configBytes)) + uint64(len(manifestBytes))
	result, err = s.repository.CreateBackup(ctx, domain.Backup{
		ID: backupID, VMUUID: packageManifest.Backup.VMUUID, VMName: packageManifest.Backup.VMName,
		RepositoryID: repository.ID, RepositoryName: repository.Name, TargetPool: repository.PoolName,
		Name: name, Status: domain.BackupStatusCompleted, FormatVersion: domain.VMBackupFormatVersionV2,
		ManifestVolume: manifestName, ConfigVolume: configName, ConfigBytes: uint64(len(configBytes)),
		TotalBytes: totalBytes, DiskCount: len(catalogDisks), Pinned: false, VerifyStatus: domain.BackupVerifyUnknown,
		CreatedBy: command.Request.Session.User.Username, CreatedAt: createdAt, CompletedAt: completedAt,
		Disks: catalogDisks, Online: packageManifest.Backup.Online, Quiesced: packageManifest.Backup.Quiesced,
		Consistent: packageManifest.Backup.Consistent, ContainsTPM: packageManifest.Backup.ContainsTPM, TPMStateSaved: false,
	})
	if err != nil {
		return domain.Backup{}, err
	}
	committed = true
	detail["totalBytes"] = totalBytes
	if err := s.recordAudit(ctx, command.Request, "backup.import", backupID, "success", detail, nil); err != nil {
		return domain.Backup{}, fmt.Errorf("record backup import audit: %w", err)
	}
	return result, nil
}

func writeZipBytes(archive *zip.Writer, name string, body []byte) error {
	writer, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return fmt.Errorf("create backup package entry %s: %w", name, err)
	}
	if _, err := writer.Write(body); err != nil {
		return fmt.Errorf("write backup package entry %s: %w", name, err)
	}
	return nil
}

func writeZipVolume(ctx context.Context, backend VMBackend, archive *zip.Writer, archivePath, pool, volume string, expectedBytes uint64) error {
	writer, err := archive.CreateHeader(&zip.FileHeader{Name: archivePath, Method: zip.Deflate})
	if err != nil {
		return fmt.Errorf("create backup package entry %s: %w", archivePath, err)
	}
	target := io.Writer(writer)
	if expectedBytes > 0 {
		target = &exactZipWriter{writer: writer, expected: expectedBytes}
	}
	if _, err := backend.DownloadStorageVolume(ctx, pool, volume, target); err != nil {
		return fmt.Errorf("export backup artifact %s: %w", volume, err)
	}
	if exact, ok := target.(*exactZipWriter); ok && exact.written != exact.expected {
		return fmt.Errorf("export backup artifact %s: expected %d bytes, got %d", volume, exact.expected, exact.written)
	}
	return nil
}

type exactZipWriter struct {
	writer   io.Writer
	expected uint64
	written  uint64
}

func (w *exactZipWriter) Write(body []byte) (int, error) {
	if uint64(len(body)) > w.expected-w.written {
		return 0, domain.ErrTooLarge
	}
	n, err := w.writer.Write(body)
	w.written += uint64(n)
	return n, err
}

func indexBackupArchive(files []*zip.File) (map[string]*zip.File, error) {
	if len(files) > maxBackupPackageEntries {
		return nil, domain.ErrTooLarge
	}
	result := make(map[string]*zip.File, len(files))
	total := uint64(0)
	for _, file := range files {
		name := file.Name
		if !safeBackupArchivePath(name) {
			return nil, fmt.Errorf("%w: unsafe backup package path %q", domain.ErrInvalidInput, name)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 || !file.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: backup package entries must be regular files", domain.ErrInvalidInput)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("%w: duplicate backup package path %q", domain.ErrInvalidInput, name)
		}
		if file.UncompressedSize64 > maxBackupPackageUncompressed-total {
			return nil, domain.ErrTooLarge
		}
		total += file.UncompressedSize64
		result[name] = file
	}
	return result, nil
}

func validateBackupPackageDisks(disks []domain.BackupPackageDisk, entries map[string]*zip.File) error {
	paths := make(map[string]struct{}, len(disks))
	for index, disk := range disks {
		if !safeBackupArchivePath(disk.ArchivePath) || disk.ArchivePath == backupPackageManifestPath || disk.ArchivePath == backupPackageConfigPath {
			return fmt.Errorf("%w: backup disk %d has an unsafe archive path", domain.ErrInvalidInput, index)
		}
		if _, exists := paths[disk.ArchivePath]; exists {
			return fmt.Errorf("%w: backup disk %d reuses an archive path", domain.ErrInvalidInput, index)
		}
		paths[disk.ArchivePath] = struct{}{}
		format := strings.TrimSpace(disk.Format)
		if format == "" || len(format) > 32 || strings.IndexFunc(format, func(char rune) bool {
			return !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '_' && char != '-'
		}) >= 0 {
			return fmt.Errorf("%w: backup disk %d has an invalid format", domain.ErrInvalidInput, index)
		}
		file, ok := entries[disk.ArchivePath]
		if !ok {
			return fmt.Errorf("%w: backup disk %d is missing", domain.ErrInvalidInput, index)
		}
		if disk.CapacityBytes == 0 || file.UncompressedSize64 == 0 || file.UncompressedSize64 > disk.CapacityBytes {
			return fmt.Errorf("%w: backup disk %d has an invalid size", domain.ErrInvalidInput, index)
		}
		if disk.SizeBytes != 0 && disk.SizeBytes != file.UncompressedSize64 {
			return fmt.Errorf("%w: backup disk %d size does not match its manifest", domain.ErrInvalidInput, index)
		}
	}
	return nil
}

func safeBackupArchivePath(name string) bool {
	if name == "" || strings.ContainsRune(name, '\\') || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") {
		return false
	}
	cleaned := path.Clean(name)
	return cleaned == name && cleaned != "." && !strings.HasPrefix(cleaned, "../")
}

func readZipEntry(file *zip.File, limit uint64) ([]byte, error) {
	if file.UncompressedSize64 > limit {
		return nil, domain.ErrTooLarge
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open backup package entry %s: %w", file.Name, err)
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read backup package entry %s: %w", file.Name, err)
	}
	if uint64(len(body)) > limit {
		return nil, domain.ErrTooLarge
	}
	return body, nil
}

func backupPackageDiskPath(index int, format, artifact string) string {
	extension := strings.ToLower(filepath.Ext(artifact))
	if extension == "" {
		extension = "." + strings.ToLower(format)
	}
	return fmt.Sprintf("disks/%04d%s", index, extension)
}

func backupVolumeNameForImport(id, suffix string) string {
	if len(id)+len(suffix) <= 127 {
		return id + suffix
	}
	return id[:max(1, 127-len(suffix))] + suffix
}

func importDiskName(id string, index int, format, original string) string {
	extension := strings.ToLower(filepath.Ext(original))
	if extension == "" {
		extension = "." + strings.ToLower(format)
	}
	return backupVolumeNameForImport(id, "-disk-"+strconv.Itoa(index)+extension)
}
