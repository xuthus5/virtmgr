package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

const portForwardColumns = `id, network, name, family, protocol, host_address, host_port, guest_address, guest_port, enabled, created_at, updated_at`

func (s *Store) ListPortForwards(ctx context.Context, network string) ([]domain.PortForward, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+portForwardColumns+` FROM port_forwards WHERE network = ? ORDER BY created_at ASC, name COLLATE NOCASE ASC`, network)
	if err != nil {
		return nil, fmt.Errorf("list port forwards: %w", err)
	}
	defer rows.Close()
	items := make([]domain.PortForward, 0)
	for rows.Next() {
		item, err := scanPortForward(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate port forwards: %w", err)
	}
	return items, nil
}

func (s *Store) GetPortForward(ctx context.Context, id string) (domain.PortForward, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+portForwardColumns+` FROM port_forwards WHERE id = ?`, id)
	item, err := scanPortForward(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PortForward{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Store) CreatePortForward(ctx context.Context, item domain.PortForward) (domain.PortForward, error) {
	now := item.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated := item.UpdatedAt
	if updated.IsZero() {
		updated = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO port_forwards(id, network, name, family, protocol, host_address, host_port, guest_address, guest_port, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Network, item.Name, item.Family, string(item.Protocol), item.HostAddress, item.HostPort,
		item.GuestAddress, item.GuestPort, boolInt(item.Enabled), now.Unix(), updated.Unix())
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.PortForward{}, domain.ErrConflict
		}
		return domain.PortForward{}, fmt.Errorf("create port forward: %w", err)
	}
	return s.GetPortForward(ctx, item.ID)
}

func (s *Store) UpdatePortForward(ctx context.Context, item domain.PortForward) (domain.PortForward, error) {
	updatedAt := item.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE port_forwards SET name = ?, family = ?, protocol = ?, host_address = ?, host_port = ?,
			guest_address = ?, guest_port = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		item.Name, item.Family, string(item.Protocol), item.HostAddress, item.HostPort,
		item.GuestAddress, item.GuestPort, boolInt(item.Enabled), updatedAt.Unix(), item.ID)
	if err != nil {
		if isUniqueConstraint(err) {
			return domain.PortForward{}, domain.ErrConflict
		}
		return domain.PortForward{}, fmt.Errorf("update port forward: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.PortForward{}, fmt.Errorf("read updated port forward count: %w", err)
	}
	if affected == 0 {
		return domain.PortForward{}, domain.ErrNotFound
	}
	return s.GetPortForward(ctx, item.ID)
}

func (s *Store) DeletePortForward(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM port_forwards WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete port forward: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted port forward count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanPortForward(scanner interface{ Scan(...any) error }) (domain.PortForward, error) {
	var item domain.PortForward
	var enabled int64
	var createdAt, updatedAt int64
	if err := scanner.Scan(&item.ID, &item.Network, &item.Name, &item.Family, &item.Protocol, &item.HostAddress,
		&item.HostPort, &item.GuestAddress, &item.GuestPort, &enabled, &createdAt, &updatedAt); err != nil {
		return domain.PortForward{}, err
	}
	item.Enabled = enabled != 0
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	item.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return item, nil
}
