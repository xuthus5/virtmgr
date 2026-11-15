package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

func (s *Store) ListVMImages(ctx context.Context) ([]domain.VMImage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, pool, volume, format, os_family, os_name, architecture,
		       capacity_bytes, allocation_bytes, sha256, created_at, updated_at
		FROM vm_images
		ORDER BY name COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list VM images: %w", err)
	}
	defer rows.Close()
	images := make([]domain.VMImage, 0)
	for rows.Next() {
		image, err := scanVMImage(rows)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate VM images: %w", err)
	}
	return images, nil
}

func (s *Store) GetVMImage(ctx context.Context, id int64) (domain.VMImage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, pool, volume, format, os_family, os_name, architecture,
		       capacity_bytes, allocation_bytes, sha256, created_at, updated_at
		FROM vm_images WHERE id = ?`, id)
	image, err := scanVMImage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VMImage{}, domain.ErrNotFound
	}
	return image, err
}

func (s *Store) CreateVMImage(ctx context.Context, image domain.VMImage) (domain.VMImage, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO vm_images(name, description, pool, volume, format, os_family, os_name, architecture,
		                     capacity_bytes, allocation_bytes, sha256, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		image.Name, image.Description, image.Pool, image.Volume, image.Format, image.OSFamily,
		image.OSName, image.Architecture, image.CapacityBytes, image.AllocationBytes, image.SHA256,
		now.Unix(), now.Unix(),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.VMImage{}, domain.ErrConflict
		}
		return domain.VMImage{}, fmt.Errorf("create VM image: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.VMImage{}, fmt.Errorf("read VM image id: %w", err)
	}
	return s.GetVMImage(ctx, id)
}

func (s *Store) UpdateVMImageMetadata(ctx context.Context, image domain.VMImage) (domain.VMImage, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE vm_images SET
			name = ?, description = ?, os_family = ?, os_name = ?, architecture = ?,
			capacity_bytes = ?, allocation_bytes = ?, sha256 = ?, updated_at = ?
		WHERE id = ?`,
		image.Name, image.Description, image.OSFamily, image.OSName, image.Architecture,
		image.CapacityBytes, image.AllocationBytes, image.SHA256, time.Now().UTC().Unix(), image.ID,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.VMImage{}, domain.ErrConflict
		}
		return domain.VMImage{}, fmt.Errorf("update VM image: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.VMImage{}, fmt.Errorf("read updated VM image count: %w", err)
	}
	if affected == 0 {
		return domain.VMImage{}, domain.ErrNotFound
	}
	return s.GetVMImage(ctx, image.ID)
}

func (s *Store) DeleteVMImage(ctx context.Context, id int64) error {
	var references int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM vm_templates WHERE image_id = ?`, id).Scan(&references); err != nil {
		return fmt.Errorf("count templates using VM image: %w", err)
	}
	if references > 0 {
		return domain.ErrConflict
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM vm_images WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete VM image: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted VM image count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type vmImageScanner interface {
	Scan(dest ...any) error
}

func scanVMImage(scanner vmImageScanner) (domain.VMImage, error) {
	var image domain.VMImage
	var capacity int64
	var allocation int64
	var createdAt int64
	var updatedAt int64
	if err := scanner.Scan(
		&image.ID, &image.Name, &image.Description, &image.Pool, &image.Volume, &image.Format,
		&image.OSFamily, &image.OSName, &image.Architecture, &capacity, &allocation, &image.SHA256,
		&createdAt, &updatedAt,
	); err != nil {
		return domain.VMImage{}, err
	}
	if capacity > 0 {
		image.CapacityBytes = uint64(capacity)
	}
	if allocation > 0 {
		image.AllocationBytes = uint64(allocation)
	}
	image.CreatedAt = time.Unix(createdAt, 0).UTC()
	image.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return image, nil
}
