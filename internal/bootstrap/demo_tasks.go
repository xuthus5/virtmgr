package bootstrap

import (
	"context"
	"fmt"
	"time"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/sqlite"
)

func seedDemoTasks(ctx context.Context, store *sqlite.Store) error {
	existing, err := store.ListTasks(ctx, domain.TaskQuery{Page: 1, PageSize: 1})
	if err != nil {
		return fmt.Errorf("check demo tasks: %w", err)
	}
	if existing.Total > 0 {
		return nil
	}
	now := time.Now().UTC()
	tasks := []domain.Task{
		{
			ID: "task-demo-clone", Kind: "vm.clone", TargetKind: "vm", TargetID: "22222222-2222-4222-8222-222222222222",
			Status: domain.TaskStatusRunning, Progress: 68, Step: "正在复制 demo-db-01.qcow2", Actor: "admin",
			CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-20 * time.Second),
		},
		{
			ID: "task-demo-iso-upload", Kind: "storage.volume.upload", TargetKind: "storage_volume", TargetID: "iso-store/debian-13-netinst.iso",
			Status: domain.TaskStatusSucceeded, Progress: 100, Step: "ISO 上传完成", Actor: "admin",
			CreatedAt: now.Add(-25 * time.Minute), UpdatedAt: now.Add(-18 * time.Minute),
		},
		{
			ID: "task-demo-snapshot-revert", Kind: "vm.snapshot.revert", TargetKind: "vm_snapshot", TargetID: "44444444-4444-4444-8444-444444444444/before-upgrade",
			Status: domain.TaskStatusFailed, Progress: 100, Step: "回滚快照", Error: "libvirt: requested operation is not supported", Actor: "operator",
			CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now.Add(-55 * time.Minute),
		},
		{
			ID: "task-demo-network-interrupted", Kind: "network.create", TargetKind: "network", TargetID: "lab-isolated",
			Status: domain.TaskStatusInterrupted, Progress: 35, Step: "服务重启，任务已中断", Error: "VirtMgr 进程在任务完成前退出", Actor: "admin",
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-115 * time.Minute),
		},
	}
	for _, task := range tasks {
		if err := store.CreateTask(ctx, task); err != nil {
			return fmt.Errorf("seed demo task %s: %w", task.ID, err)
		}
	}
	return nil
}
