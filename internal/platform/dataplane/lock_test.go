package dataplane

import (
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestLockManagerRejectsConflictingOperations(t *testing.T) {
	manager := NewLockManager()
	first, err := manager.Acquire(domain.VMLockKey("vm-1"), "备份")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	first.SetTaskID("task-1")

	_, err = manager.Acquire(domain.VMLockKey("vm-1"), "快照")
	if !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("conflict error = %v, want ErrOperationConflict", err)
	}

	first.Release()
	second, err := manager.Acquire(domain.VMLockKey("vm-1"), "快照")
	if err != nil {
		t.Fatalf("Acquire() after release error = %v", err)
	}
	second.Release()
}

func TestLockManagerAcquireManyIsAtomic(t *testing.T) {
	manager := NewLockManager()
	busy, err := manager.Acquire(domain.VMNameLockKey("web-01"), "导入")
	if err != nil {
		t.Fatal(err)
	}

	_, err = manager.AcquireMany([]string{
		domain.VMLockKey("vm-1"),
		domain.VMNameLockKey("web-01"),
		domain.VMLockKey("vm-1"),
	}, "克隆")
	if !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("AcquireMany() error = %v, want ErrOperationConflict", err)
	}

	unrelated, err := manager.Acquire(domain.VMLockKey("vm-1"), "备份")
	if err != nil {
		t.Fatalf("failed multi-lock leaked a partial lock: %v", err)
	}
	unrelated.Release()
	busy.Release()
}

func TestLockManagerReleasesAllKeysAndHonorsBlock(t *testing.T) {
	manager := NewLockManager()
	lease, err := manager.AcquireMany([]string{
		domain.VMLockKey("vm-1"), domain.StorageVolumeLockKey("default", "disk.qcow2"),
	}, "迁移")
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	for _, key := range []string{domain.VMLockKey("vm-1"), domain.StorageVolumeLockKey("default", "disk.qcow2")} {
		next, err := manager.Acquire(key, "扩容")
		if err != nil {
			t.Fatalf("Acquire(%s) after release error = %v", key, err)
		}
		next.Release()
	}

	manager.Block(domain.VMLockKey("vm-2"), "在线备份 overlay 仍被活动配置引用")
	_, err = manager.Acquire(domain.VMLockKey("vm-2"), "备份")
	if !errors.Is(err, domain.ErrOperationConflict) {
		t.Fatalf("blocked key error = %v, want ErrOperationConflict", err)
	}
	if strings.Contains(err.Error(), "正在执行") || !strings.Contains(err.Error(), "需要人工处理") {
		t.Fatalf("blocked key error message = %q", err)
	}
	if !manager.Blocked(domain.VMLockKey("vm-2")) {
		t.Fatal("Blocked() = false for recovery guard")
	}
	if !manager.Unblock(domain.VMLockKey("vm-2")) || manager.Blocked(domain.VMLockKey("vm-2")) {
		t.Fatal("Unblock() did not clear recovery guard")
	}

	active, err := manager.Acquire(domain.VMLockKey("vm-3"), "备份")
	if err != nil {
		t.Fatal(err)
	}
	if manager.Unblock(domain.VMLockKey("vm-3")) {
		t.Fatal("Unblock() released an active lease")
	}
	active.Release()
}
