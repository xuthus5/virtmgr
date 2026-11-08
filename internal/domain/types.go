package domain

import "time"

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"createdAt"`
}

type UserList struct {
	Items []User `json:"items"`
	Total int    `json:"total"`
}

func ValidRole(role Role) bool {
	return role == RoleViewer || role == RoleOperator || role == RoleAdmin
}

type Session struct {
	TokenHash string
	CSRFToken string
	User      User
	ExpiresAt time.Time
}

type ListQuery struct {
	Query string
	State string
}

type ReadOnlyProvider interface {
	ReadOnly() bool
}
