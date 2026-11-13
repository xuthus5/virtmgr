package sqlite

import (
	"context"
	"fmt"
	"os"
)

// ExportDatabase creates a consistent point-in-time copy without stopping the
// service. VACUUM INTO is supported by SQLite for online database snapshots.
func (s *Store) ExportDatabase(ctx context.Context, destination string) error {
	if destination == "" {
		return fmt.Errorf("database export destination is required")
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale database export: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, destination); err != nil {
		return fmt.Errorf("export database: %w", err)
	}
	return nil
}
