package runtimeconfig

import (
	"context"
	"fmt"
	"strconv"
	"sync"
)

const AuditRetentionSettingKey = "audit_retention_days"

type AuditRetention struct {
	mu    sync.RWMutex
	store Store
	days  int
}

func NewAuditRetention(ctx context.Context, store Store, initialDays int) (*AuditRetention, error) {
	if store == nil {
		return nil, fmt.Errorf("audit retention store is required")
	}
	if initialDays < 1 {
		initialDays = 30
	}
	value, found, err := store.GetSetting(ctx, AuditRetentionSettingKey)
	if err != nil {
		return nil, err
	}
	days := initialDays
	if found {
		days, err = strconv.Atoi(value)
		if err != nil || days < 1 {
			return nil, fmt.Errorf("parse persisted audit retention setting")
		}
	} else if err := store.SetSetting(ctx, AuditRetentionSettingKey, strconv.Itoa(initialDays)); err != nil {
		return nil, err
	}
	return &AuditRetention{store: store, days: days}, nil
}

func (r *AuditRetention) Days() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.days
}

func (r *AuditRetention) SetDays(ctx context.Context, days int) error {
	if days < 1 || days > 3650 {
		return fmt.Errorf("audit retention days must be between 1 and 3650")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.store.SetSetting(ctx, AuditRetentionSettingKey, strconv.Itoa(days)); err != nil {
		return err
	}
	r.days = days
	return nil
}
