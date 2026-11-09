package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

const vmConfigVersionColumns = `
	vm_uuid, version_no, xml_sha256, size_bytes, source, source_version,
	actor, role, request_id, source_ip, note, protected, created_at`

func (s *Store) ListVMConfigVersions(ctx context.Context, vmUUID string) ([]domain.VMConfigVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT`+vmConfigVersionColumns+`
		FROM vm_config_versions
		WHERE vm_uuid = ?
		ORDER BY version_no DESC`, vmUUID)
	if err != nil {
		return nil, fmt.Errorf("list VM config versions: %w", err)
	}
	defer rows.Close()
	items := make([]domain.VMConfigVersion, 0)
	for rows.Next() {
		item, err := scanVMConfigVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate VM config versions: %w", err)
	}
	return items, nil
}

func (s *Store) LatestVMConfigVersion(ctx context.Context, vmUUID string) (domain.VMConfigVersion, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT`+vmConfigVersionColumns+`
		FROM vm_config_versions
		WHERE vm_uuid = ?
		ORDER BY version_no DESC
		LIMIT 1`, vmUUID)
	item, err := scanVMConfigVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VMConfigVersion{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) GetVMConfigVersion(ctx context.Context, vmUUID string, versionNo int) (domain.VMConfigVersion, []byte, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT`+vmConfigVersionColumns+`, xml_data
		FROM vm_config_versions
		WHERE vm_uuid = ? AND version_no = ?`, vmUUID, versionNo)
	var item domain.VMConfigVersion
	var compressed []byte
	var protected int
	var createdAt int64
	if err := row.Scan(
		&item.VMUUID, &item.VersionNo, &item.XMLSHA256, &item.SizeBytes, &item.Source,
		&item.SourceVersion, &item.Actor, &item.Role, &item.RequestID, &item.SourceIP,
		&item.Note, &protected, &createdAt, &compressed,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.VMConfigVersion{}, nil, domain.ErrNotFound
		}
		return domain.VMConfigVersion{}, nil, fmt.Errorf("get VM config version: %w", err)
	}
	item.Protected = protected != 0
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	return item, compressed, nil
}

func (s *Store) CreateVMConfigVersion(ctx context.Context, item domain.VMConfigVersion, compressed []byte) (domain.VMConfigVersion, error) {
	createdAt := item.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vm_config_versions(
			vm_uuid, version_no, xml_sha256, xml_data, size_bytes, source, source_version,
			actor, role, request_id, source_ip, note, protected, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.VMUUID, item.VersionNo, item.XMLSHA256, compressed, item.SizeBytes, item.Source,
		item.SourceVersion, item.Actor, item.Role, item.RequestID, item.SourceIP, item.Note,
		boolInt(item.Protected), createdAt.UTC().Unix(),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.VMConfigVersion{}, domain.ErrConflict
		}
		return domain.VMConfigVersion{}, fmt.Errorf("create VM config version: %w", err)
	}
	item.CreatedAt = createdAt.UTC()
	return item, nil
}

func (s *Store) SetVMConfigVersionProtected(ctx context.Context, vmUUID string, versionNo int, protected bool) (domain.VMConfigVersion, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE vm_config_versions SET protected = ? WHERE vm_uuid = ? AND version_no = ?`,
		boolInt(protected), vmUUID, versionNo)
	if err != nil {
		return domain.VMConfigVersion{}, fmt.Errorf("update VM config version protection: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.VMConfigVersion{}, fmt.Errorf("read protected VM config version count: %w", err)
	}
	if affected == 0 {
		return domain.VMConfigVersion{}, domain.ErrNotFound
	}
	item, _, err := s.GetVMConfigVersion(ctx, vmUUID, versionNo)
	return item, err
}

func (s *Store) PruneVMConfigVersions(ctx context.Context, vmUUID string, keep int) (int64, error) {
	if keep < 1 {
		keep = domain.VMConfigVersionRetention
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM vm_config_versions
		WHERE vm_uuid = ? AND protected = 0 AND version_no NOT IN (
			SELECT version_no FROM vm_config_versions
			WHERE vm_uuid = ? AND protected = 0
			ORDER BY version_no DESC
			LIMIT ?
		)`, vmUUID, vmUUID, keep)
	if err != nil {
		return 0, fmt.Errorf("prune VM config versions: %w", err)
	}
	return result.RowsAffected()
}

type vmConfigVersionScanner interface {
	Scan(dest ...any) error
}

func scanVMConfigVersion(scanner vmConfigVersionScanner) (domain.VMConfigVersion, error) {
	var item domain.VMConfigVersion
	var protected int
	var createdAt int64
	if err := scanner.Scan(
		&item.VMUUID, &item.VersionNo, &item.XMLSHA256, &item.SizeBytes, &item.Source,
		&item.SourceVersion, &item.Actor, &item.Role, &item.RequestID, &item.SourceIP,
		&item.Note, &protected, &createdAt,
	); err != nil {
		return domain.VMConfigVersion{}, err
	}
	item.Protected = protected != 0
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	return item, nil
}
