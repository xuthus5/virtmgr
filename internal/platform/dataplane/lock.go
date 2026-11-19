package dataplane

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"virtmgr/internal/domain"
)

type lockHolder struct {
	operation string
	taskID    string
	reason    string
	blocked   bool
}

// LockManager serializes data-plane mutations that target the same VM or VM
// name. It is intentionally process-local: stale locks disappear on restart,
// while crash recovery reconciles persisted operations against libvirt.
type LockManager struct {
	mu      sync.Mutex
	holders map[string]lockHolder
}

type Lease struct {
	manager *LockManager
	keys    []string
	once    sync.Once
}

func NewLockManager() *LockManager {
	return &LockManager{holders: make(map[string]lockHolder)}
}

func (m *LockManager) Acquire(key, operation string) (*Lease, error) {
	return m.AcquireMany([]string{key}, operation)
}

// AcquireMany atomically acquires all supplied resource keys. Keys are
// de-duplicated and sorted so callers that need source and target locks cannot
// introduce lock-order deadlocks.
func (m *LockManager) AcquireMany(keys []string, operation string) (*Lease, error) {
	keys = normalizeKeys(keys)
	operation = strings.TrimSpace(operation)
	if len(keys) == 0 || operation == "" {
		return nil, domain.ErrInvalidInput
	}
	if m == nil {
		return &Lease{}, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		if holder, exists := m.holders[key]; exists {
			return nil, conflictError(holder)
		}
	}
	for _, key := range keys {
		m.holders[key] = lockHolder{operation: operation}
	}
	return &Lease{manager: m, keys: keys}, nil
}

func normalizeKeys(keys []string) []string {
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		normalized = append(normalized, key)
	}
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func conflictError(holder lockHolder) error {
	if holder.blocked {
		message := "该资源的上次数据面操作需要人工处理"
		if holder.reason != "" {
			message += "：" + holder.reason
		}
		return fmt.Errorf("%w: %s", domain.ErrOperationConflict, message)
	}
	message := fmt.Sprintf("该资源正在执行%s", holder.operation)
	if holder.taskID != "" {
		message += "（任务 " + holder.taskID + "）"
	}
	return fmt.Errorf("%w: %s", domain.ErrOperationConflict, message)
}

func (m *LockManager) Block(key, reason string) {
	if m == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "上次数据面操作需要人工处理后才能继续"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.holders[key]; exists {
		return
	}
	m.holders[key] = lockHolder{reason: reason, blocked: true}
}

func (l *Lease) SetTaskID(id string) {
	if l == nil || l.manager == nil {
		return
	}
	l.manager.mu.Lock()
	defer l.manager.mu.Unlock()
	for _, key := range l.keys {
		holder, exists := l.manager.holders[key]
		if !exists {
			continue
		}
		holder.taskID = strings.TrimSpace(id)
		l.manager.holders[key] = holder
	}
}

func (l *Lease) Release() {
	if l == nil || l.manager == nil {
		return
	}
	l.once.Do(func() {
		l.manager.mu.Lock()
		for _, key := range l.keys {
			delete(l.manager.holders, key)
		}
		l.manager.mu.Unlock()
	})
}
