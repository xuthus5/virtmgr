package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

func (s *Store) ListVMTemplates(ctx context.Context) ([]domain.VMTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, vcpus, memory_mib, disk_format, network, iso_path,
			firmware, secure_boot, cloud_init, cloud_init_user_data, cloud_init_network_config,
			cloud_init_vendor_data, autostart, start,
			created_at, updated_at
		FROM vm_templates
		ORDER BY name COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list VM templates: %w", err)
	}
	defer rows.Close()
	templates := make([]domain.VMTemplate, 0)
	for rows.Next() {
		template, err := scanVMTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate VM templates: %w", err)
	}
	return templates, nil
}

func (s *Store) GetVMTemplate(ctx context.Context, id int64) (domain.VMTemplate, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, vcpus, memory_mib, disk_format, network, iso_path,
			firmware, secure_boot, cloud_init, cloud_init_user_data, cloud_init_network_config,
			cloud_init_vendor_data, autostart, start,
			created_at, updated_at
		FROM vm_templates WHERE id = ?`, id)
	template, err := scanVMTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VMTemplate{}, domain.ErrNotFound
	}
	return template, err
}

func (s *Store) VMTemplateByName(ctx context.Context, name string) (domain.VMTemplate, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, vcpus, memory_mib, disk_format, network, iso_path,
			firmware, secure_boot, cloud_init, cloud_init_user_data, cloud_init_network_config,
			cloud_init_vendor_data, autostart, start,
			created_at, updated_at
		FROM vm_templates WHERE name = ? COLLATE NOCASE`, name)
	template, err := scanVMTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VMTemplate{}, domain.ErrNotFound
	}
	return template, err
}

func (s *Store) CreateVMTemplate(ctx context.Context, spec domain.VMTemplateSpec) (domain.VMTemplate, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO vm_templates(
			name, description, vcpus, memory_mib, disk_format, network, iso_path,
			firmware, secure_boot, cloud_init, cloud_init_user_data, cloud_init_network_config,
			cloud_init_vendor_data, autostart, start,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spec.Name, spec.Description, spec.VCPUs, spec.MemoryMiB, spec.DiskFormat,
		spec.Network, spec.ISOPath, spec.Firmware, boolInt(spec.SecureBoot),
		boolInt(spec.CloudInit), spec.CloudInitUserData, spec.CloudInitNetworkConfig, spec.CloudInitVendorData, boolInt(spec.Autostart),
		boolInt(spec.Start), now.Unix(), now.Unix(),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.VMTemplate{}, domain.ErrConflict
		}
		return domain.VMTemplate{}, fmt.Errorf("create VM template: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.VMTemplate{}, fmt.Errorf("read VM template id: %w", err)
	}
	return s.GetVMTemplate(ctx, id)
}

func (s *Store) UpdateVMTemplate(ctx context.Context, id int64, spec domain.VMTemplateSpec) (domain.VMTemplate, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE vm_templates SET
			name = ?, description = ?, vcpus = ?, memory_mib = ?, disk_format = ?,
			network = ?, iso_path = ?, firmware = ?, secure_boot = ?, cloud_init = ?,
			cloud_init_user_data = ?, cloud_init_network_config = ?, cloud_init_vendor_data = ?,
			autostart = ?, start = ?, updated_at = ?
		WHERE id = ?`,
		spec.Name, spec.Description, spec.VCPUs, spec.MemoryMiB, spec.DiskFormat,
		spec.Network, spec.ISOPath, spec.Firmware, boolInt(spec.SecureBoot),
		boolInt(spec.CloudInit), spec.CloudInitUserData, spec.CloudInitNetworkConfig, spec.CloudInitVendorData, boolInt(spec.Autostart),
		boolInt(spec.Start), time.Now().UTC().Unix(), id,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.VMTemplate{}, domain.ErrConflict
		}
		return domain.VMTemplate{}, fmt.Errorf("update VM template: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.VMTemplate{}, fmt.Errorf("read updated VM template count: %w", err)
	}
	if affected == 0 {
		return domain.VMTemplate{}, domain.ErrNotFound
	}
	return s.GetVMTemplate(ctx, id)
}

func (s *Store) DeleteVMTemplate(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM vm_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete VM template: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted VM template count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type vmTemplateScanner interface {
	Scan(dest ...any) error
}

func scanVMTemplate(scanner vmTemplateScanner) (domain.VMTemplate, error) {
	var template domain.VMTemplate
	var secureBoot int
	var cloudInit int
	var autostart int
	var start int
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&template.ID, &template.Name, &template.Description, &template.VCPUs,
		&template.MemoryMiB, &template.DiskFormat, &template.Network, &template.ISOPath,
		&template.Firmware, &secureBoot, &cloudInit, &template.CloudInitUserData,
		&template.CloudInitNetworkConfig, &template.CloudInitVendorData, &autostart, &start, &createdAt, &updatedAt,
	); err != nil {
		return domain.VMTemplate{}, err
	}
	template.SecureBoot = secureBoot != 0
	template.CloudInit = cloudInit != 0
	template.Autostart = autostart != 0
	template.Start = start != 0
	template.CreatedAt = time.Unix(createdAt, 0).UTC()
	template.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return template, nil
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
