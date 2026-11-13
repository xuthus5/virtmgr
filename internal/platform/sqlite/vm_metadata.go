package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

func (s *Store) ListVMMetadata(ctx context.Context) (map[string]domain.VMMetadata, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT vm_uuid, tags_json, note, owner, updated_by, updated_at
		FROM vm_metadata ORDER BY vm_uuid ASC`)
	if err != nil {
		return nil, fmt.Errorf("list VM metadata: %w", err)
	}
	defer rows.Close()
	items := make(map[string]domain.VMMetadata)
	for rows.Next() {
		item, err := scanVMMetadata(rows)
		if err != nil {
			return nil, fmt.Errorf("scan VM metadata: %w", err)
		}
		items[item.VMUUID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate VM metadata: %w", err)
	}
	return items, nil
}

func (s *Store) GetVMMetadata(ctx context.Context, vmUUID string) (domain.VMMetadata, error) {
	item, err := scanVMMetadata(s.db.QueryRowContext(ctx, `
		SELECT vm_uuid, tags_json, note, owner, updated_by, updated_at
		FROM vm_metadata WHERE vm_uuid = ?`, vmUUID))
	if err == sql.ErrNoRows {
		return domain.VMMetadata{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.VMMetadata{}, fmt.Errorf("get VM metadata: %w", err)
	}
	return item, nil
}

func (s *Store) UpsertVMMetadata(ctx context.Context, item domain.VMMetadata) (domain.VMMetadata, error) {
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return domain.VMMetadata{}, fmt.Errorf("encode VM metadata tags: %w", err)
	}
	updatedAt := item.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO vm_metadata(vm_uuid, tags_json, note, owner, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(vm_uuid) DO UPDATE SET
			tags_json = excluded.tags_json,
			note = excluded.note,
			owner = excluded.owner,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`,
		item.VMUUID, string(tags), item.Note, item.Owner, item.UpdatedBy, updatedAt.UTC().Unix())
	if err != nil {
		return domain.VMMetadata{}, fmt.Errorf("upsert VM metadata: %w", err)
	}
	return s.GetVMMetadata(ctx, item.VMUUID)
}

func (s *Store) DeleteVMMetadata(ctx context.Context, vmUUID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM vm_metadata WHERE vm_uuid = ?`, vmUUID)
	if err != nil {
		return fmt.Errorf("delete VM metadata: %w", err)
	}
	return nil
}

func scanVMMetadata(scanner rowScanner) (domain.VMMetadata, error) {
	var item domain.VMMetadata
	var tagsJSON string
	var updatedAt int64
	if err := scanner.Scan(&item.VMUUID, &tagsJSON, &item.Note, &item.Owner, &item.UpdatedBy, &updatedAt); err != nil {
		return domain.VMMetadata{}, err
	}
	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &item.Tags); err != nil {
			return domain.VMMetadata{}, fmt.Errorf("decode VM metadata tags: %w", err)
		}
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}
