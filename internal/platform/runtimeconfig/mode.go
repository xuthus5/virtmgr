package runtimeconfig

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"virtmgr/internal/domain"
)

const ReadOnlySettingKey = "read_only"

type Store interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
}

type Mode struct {
	mu       sync.RWMutex
	store    Store
	readOnly bool
}

var _ domain.ReadOnlyProvider = (*Mode)(nil)

func New(ctx context.Context, store Store, initialReadOnly bool) (*Mode, error) {
	if store == nil {
		return nil, fmt.Errorf("runtime settings store is required")
	}
	value, found, err := store.GetSetting(ctx, ReadOnlySettingKey)
	if err != nil {
		return nil, err
	}
	readOnly := initialReadOnly
	if found {
		readOnly, err = strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("parse persisted read-only setting: %w", err)
		}
	} else if err := store.SetSetting(ctx, ReadOnlySettingKey, strconv.FormatBool(initialReadOnly)); err != nil {
		return nil, err
	}
	return &Mode{store: store, readOnly: readOnly}, nil
}

func (m *Mode) ReadOnly() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.readOnly
}

func (m *Mode) SetReadOnly(ctx context.Context, readOnly bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.store.SetSetting(ctx, ReadOnlySettingKey, strconv.FormatBool(readOnly)); err != nil {
		return err
	}
	m.readOnly = readOnly
	return nil
}
