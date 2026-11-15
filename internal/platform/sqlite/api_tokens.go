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

func (s *Store) CreateAPIToken(ctx context.Context, userID int64, token domain.APIToken, tokenHash string) (domain.APIToken, error) {
	scopes := make([]string, 0, len(token.Scopes))
	for _, scope := range token.Scopes {
		scopes = append(scopes, string(scope))
	}
	var expiresAt any
	if token.ExpiresAt != nil {
		expiresAt = token.ExpiresAt.UTC().Unix()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_tokens(id, user_id, name, token_hash, scopes, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token.ID, userID, token.Name, tokenHash, strings.Join(scopes, ","), token.CreatedAt.UTC().Unix(), expiresAt,
	)
	if err != nil {
		return domain.APIToken{}, fmt.Errorf("create API token: %w", err)
	}
	return token, nil
}

func (s *Store) ListAPITokensByUser(ctx context.Context, userID int64) ([]domain.APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, scopes, created_at, expires_at, last_used_at, revoked_at
		FROM api_tokens
		WHERE user_id = ?
		ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list API tokens: %w", err)
	}
	defer rows.Close()
	now := time.Now().UTC()
	tokens := make([]domain.APIToken, 0)
	for rows.Next() {
		token, err := scanAPIToken(rows, now)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate API tokens: %w", err)
	}
	return tokens, nil
}

func (s *Store) RevokeAPIToken(ctx context.Context, userID int64, id string, revokedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE api_tokens SET revoked_at = ?
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		revokedAt.UTC().Unix(), id, userID,
	)
	if err != nil {
		return fmt.Errorf("revoke API token: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read revoked API token count: %w", err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) APITokenPrincipalByHash(ctx context.Context, tokenHash string, now time.Time) (domain.APITokenPrincipal, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.scopes, t.created_at, t.expires_at, t.last_used_at, t.revoked_at,
		       u.id, u.username, u.password_hash, u.role, u.disabled, u.created_at
		FROM api_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ? AND t.revoked_at IS NULL
		  AND (t.expires_at IS NULL OR t.expires_at > ?)`, tokenHash, now.UTC().Unix())
	var principal domain.APITokenPrincipal
	var scopes string
	var createdAt int64
	var expiresAt sql.NullInt64
	var lastUsedAt sql.NullInt64
	var revokedAt sql.NullInt64
	var userCreatedAt int64
	var disabled int
	err := row.Scan(
		&principal.Token.ID, &principal.Token.Name, &scopes, &createdAt, &expiresAt, &lastUsedAt, &revokedAt,
		&principal.User.ID, &principal.User.Username, &principal.User.PasswordHash, &principal.User.Role, &disabled, &userCreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.APITokenPrincipal{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.APITokenPrincipal{}, fmt.Errorf("read API token principal: %w", err)
	}
	principal.Token.Scopes = parseAPITokenScopes(scopes)
	principal.Token.CreatedAt = time.Unix(createdAt, 0).UTC()
	if expiresAt.Valid {
		value := time.Unix(expiresAt.Int64, 0).UTC()
		principal.Token.ExpiresAt = &value
	}
	if lastUsedAt.Valid {
		value := time.Unix(lastUsedAt.Int64, 0).UTC()
		principal.Token.LastUsedAt = &value
	}
	if revokedAt.Valid {
		value := time.Unix(revokedAt.Int64, 0).UTC()
		principal.Token.RevokedAt = &value
	}
	principal.User.Disabled = disabled != 0
	principal.User.CreatedAt = time.Unix(userCreatedAt, 0).UTC()
	principal.User.PasswordHash = ""
	return principal, nil
}

func (s *Store) TouchAPIToken(ctx context.Context, id string, usedAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, usedAt.UTC().Unix(), id); err != nil {
		return fmt.Errorf("touch API token: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpiredAPITokens(ctx context.Context, before time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at <= ?`, before.UTC().Unix()); err != nil {
		return fmt.Errorf("delete expired API tokens: %w", err)
	}
	return nil
}

type apiTokenScanner interface {
	Scan(dest ...any) error
}

func scanAPIToken(scanner apiTokenScanner, now time.Time) (domain.APIToken, error) {
	var token domain.APIToken
	var scopes string
	var createdAt int64
	var expiresAt sql.NullInt64
	var lastUsedAt sql.NullInt64
	var revokedAt sql.NullInt64
	if err := scanner.Scan(&token.ID, &token.Name, &scopes, &createdAt, &expiresAt, &lastUsedAt, &revokedAt); err != nil {
		return domain.APIToken{}, fmt.Errorf("scan API token: %w", err)
	}
	token.Scopes = parseAPITokenScopes(scopes)
	token.CreatedAt = time.Unix(createdAt, 0).UTC()
	if expiresAt.Valid {
		value := time.Unix(expiresAt.Int64, 0).UTC()
		token.ExpiresAt = &value
	}
	if lastUsedAt.Valid {
		value := time.Unix(lastUsedAt.Int64, 0).UTC()
		token.LastUsedAt = &value
	}
	if revokedAt.Valid {
		value := time.Unix(revokedAt.Int64, 0).UTC()
		token.RevokedAt = &value
	}
	token.Expired = token.ExpiresAt != nil && !token.ExpiresAt.After(now)
	return token, nil
}

func parseAPITokenScopes(value string) []domain.APITokenScope {
	parts := strings.Split(value, ",")
	scopes := make([]domain.APITokenScope, 0, len(parts))
	for _, part := range parts {
		scope := domain.APITokenScope(strings.TrimSpace(part))
		if domain.ValidAPITokenScope(scope) {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}
