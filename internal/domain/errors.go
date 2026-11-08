package domain

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrUnavailable        = errors.New("unavailable")
	ErrInvalidInput       = errors.New("invalid input")
	ErrTooLarge           = errors.New("request too large")
	ErrReadOnly           = errors.New("read only")
	ErrActionNotAllowed   = errors.New("action not allowed")
	ErrDiskResize         = errors.New("disk resize unsupported")
	ErrVolumeInUse        = errors.New("storage volume in use")
	ErrSnapshotRisky      = errors.New("snapshot revert is risky")
	ErrConsoleUnavailable = errors.New("console unavailable")
	ErrConsoleToken       = errors.New("console token invalid")
	ErrNetworkActive      = errors.New("network active")
	ErrNetworkUnsupported = errors.New("network update unsupported")
	ErrCSRF               = errors.New("csrf validation failed")
	ErrSelfModification   = errors.New("cannot modify current user")
)
