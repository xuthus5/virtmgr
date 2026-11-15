package domain

import (
	"strings"
	"time"
)

type APITokenScope string

const (
	APITokenScopeRead  APITokenScope = "read"
	APITokenScopeWrite APITokenScope = "write"
)

func ValidAPITokenScope(scope APITokenScope) bool {
	return scope == APITokenScopeRead || scope == APITokenScopeWrite
}

type APIToken struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Scopes     []APITokenScope `json:"scopes"`
	CreatedAt  time.Time       `json:"createdAt"`
	ExpiresAt  *time.Time      `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time      `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time      `json:"revokedAt,omitempty"`
	Expired    bool            `json:"expired"`
}

type APITokenCreateSpec struct {
	Name          string          `json:"name"`
	Scopes        []APITokenScope `json:"scopes"`
	ExpiresInDays int             `json:"expiresInDays"`
}

type APITokenCreated struct {
	Token    string   `json:"token"`
	APIToken APIToken `json:"apiToken"`
}

type APITokenList struct {
	Items []APIToken `json:"items"`
	Total int        `json:"total"`
}

type APITokenPrincipal struct {
	Token APIToken
	User  User
}

func (token APIToken) Active(now time.Time) bool {
	if token.RevokedAt != nil {
		return false
	}
	return token.ExpiresAt == nil || token.ExpiresAt.After(now)
}

func NormalizeAPITokenScopes(values []APITokenScope) ([]APITokenScope, bool) {
	seen := make(map[APITokenScope]bool, len(values))
	for _, value := range values {
		scope := APITokenScope(strings.ToLower(strings.TrimSpace(string(value))))
		if !ValidAPITokenScope(scope) {
			return nil, false
		}
		seen[scope] = true
	}
	if len(seen) == 0 {
		return nil, false
	}
	if seen[APITokenScopeWrite] {
		seen[APITokenScopeRead] = true
	}
	result := make([]APITokenScope, 0, len(seen))
	if seen[APITokenScopeRead] {
		result = append(result, APITokenScopeRead)
	}
	if seen[APITokenScopeWrite] {
		result = append(result, APITokenScopeWrite)
	}
	return result, true
}
