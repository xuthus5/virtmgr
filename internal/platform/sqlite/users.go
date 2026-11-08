package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"virtmgr/internal/domain"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, role domain.Role) (domain.User, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO users(username, password_hash, role, disabled, created_at) VALUES (?, ?, ?, 0, ?)`,
		username, passwordHash, role, now.Unix(),
	)
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.User{}, fmt.Errorf("read user id: %w", err)
	}
	return domain.User{ID: id, Username: username, PasswordHash: passwordHash, Role: role, CreatedAt: now}, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, role, disabled, created_at
		FROM users
		ORDER BY username COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		var disabled int
		var createdUnix int64
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &disabled, &createdUnix); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		user.Disabled = disabled != 0
		user.CreatedAt = time.Unix(createdUnix, 0).UTC()
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (s *Store) UserByID(ctx context.Context, id int64) (domain.User, error) {
	var user domain.User
	var createdUnix int64
	var disabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, disabled, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &disabled, &createdUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("read user by id: %w", err)
	}
	user.Disabled = disabled != 0
	user.CreatedAt = time.Unix(createdUnix, 0).UTC()
	return user, nil
}

func (s *Store) UpdateUser(ctx context.Context, id int64, role domain.Role, disabled bool) (domain.User, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE users SET role = ?, disabled = ? WHERE id = ?`,
		role, boolInt(disabled), id,
	)
	if err != nil {
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.User{}, fmt.Errorf("read updated user count: %w", err)
	}
	if affected == 0 {
		return domain.User{}, domain.ErrNotFound
	}
	return s.UserByID(ctx, id)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) UserByUsername(ctx context.Context, username string) (domain.User, error) {
	var user domain.User
	var createdUnix int64
	var disabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, disabled, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &disabled, &createdUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("read user: %w", err)
	}
	user.Disabled = disabled != 0
	user.CreatedAt = time.Unix(createdUnix, 0).UTC()
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, tokenHash, csrfToken string, userID int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, user_id, csrf_token, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		tokenHash, userID, csrfToken, time.Now().UTC().Unix(), expiresAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash string) (domain.Session, error) {
	var session domain.Session
	var expiresUnix int64
	var createdUnix int64
	var disabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT s.csrf_token, s.expires_at, u.id, u.username, u.password_hash, u.role, u.disabled, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`,
		tokenHash, time.Now().UTC().Unix(),
	).Scan(
		&session.CSRFToken, &expiresUnix, &session.User.ID, &session.User.Username,
		&session.User.PasswordHash, &session.User.Role, &disabled, &createdUnix,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("read session: %w", err)
	}
	if disabled != 0 {
		return domain.Session{}, domain.ErrUnauthorized
	}
	session.TokenHash = tokenHash
	session.ExpiresAt = time.Unix(expiresUnix, 0).UTC()
	session.User.Disabled = false
	session.User.CreatedAt = time.Unix(createdUnix, 0).UTC()
	return session, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.UTC().Unix()); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
