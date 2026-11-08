package domain

import "time"

type ConsoleToken struct {
	Token     string
	VMUUID    string
	UserID    int64
	ExpiresAt time.Time
}

type ConsoleTicket struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	Path      string    `json:"path"`
}
