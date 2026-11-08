package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const cloneTimeout = 2 * time.Hour

func (s *Service) Clone(ctx context.Context, command CloneCommand) (domain.Task, error) {
	command.UUID = strings.TrimSpace(command.UUID)
	command.Spec.Name = strings.TrimSpace(command.Spec.Name)
	command.Spec.TargetPool = strings.TrimSpace(command.Spec.TargetPool)
	detail := cloneSpecDetail(command.UUID, command.Spec)
	if !domain.CanOperateVMs(command.Request.Session.User.Role) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, domain.ErrReadOnly)
	}
	if s.tasks == nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, domain.ErrUnavailable)
	}
	if command.UUID == "" || !vmNamePattern.MatchString(command.Spec.Name) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, domain.ErrInvalidInput)
	}
	if command.Spec.TargetPool != "" && !networkPattern.MatchString(command.Spec.TargetPool) {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, domain.ErrInvalidInput)
	}
	source, err := s.repository.GetVM(ctx, command.UUID)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, err)
	}
	detail["sourceName"] = source.Name
	detail["sourceState"] = string(source.State)
	if (source.State != domain.VMStateShutoff && source.State != domain.VMStateCrashed) || !source.Persistent {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, domain.ErrActionNotAllowed)
	}
	vms, err := s.repository.ListVMs(ctx)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, err)
	}
	for _, vm := range vms {
		if vm.Name == command.Spec.Name {
			return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, domain.ErrConflict)
		}
	}

	startStep := fmt.Sprintf("准备克隆 %s", command.Spec.Name)
	taskID, err := s.tasks.Start(ctx, "vm.clone", "vm", command.UUID, command.Request.Session.User.Username, startStep)
	if err != nil {
		return domain.Task{}, s.mutationFailure(ctx, command.Request, "vm.clone", command.UUID, detail, err)
	}
	now := time.Now().UTC()
	task := domain.Task{
		ID: taskID, Kind: "vm.clone", TargetKind: "vm", TargetID: command.UUID,
		Status: domain.TaskStatusRunning, Progress: 0, Step: startStep,
		Actor: command.Request.Session.User.Username, CreatedAt: now, UpdatedAt: now,
	}
	jobContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), cloneTimeout)
	go func() {
		defer cancel()
		s.runClone(jobContext, task.ID, command, detail)
	}()
	return task, nil
}

func (s *Service) runClone(ctx context.Context, taskID string, command CloneCommand, detail map[string]any) {
	progress := 0
	vm, err := s.repository.CloneVM(ctx, command.UUID, command.Spec, func(percent int, step string) {
		if percent < progress {
			percent = progress
		}
		if percent > 100 {
			percent = 100
		}
		progress = percent
		_ = s.tasks.Progress(ctx, taskID, progress, step)
	})
	if err != nil {
		_ = s.recordMutationAudit(ctx, command.Request, "vm.clone", command.UUID, "failure", detail, err)
		_ = s.tasks.Fail(ctx, taskID, progress, "克隆虚拟机失败", err)
		return
	}
	detail["uuid"] = vm.UUID
	detail["name"] = vm.Name
	detail["state"] = string(vm.State)
	if err := s.recordMutationAudit(ctx, command.Request, "vm.clone", vm.UUID, "success", detail, nil); err != nil {
		_ = s.tasks.Fail(ctx, taskID, 100, "克隆完成但审计写入失败", err)
		return
	}
	_ = s.tasks.Succeed(ctx, taskID, fmt.Sprintf("虚拟机 %s 克隆完成", vm.Name))
}

func cloneSpecDetail(sourceUUID string, spec domain.VMCloneSpec) map[string]any {
	detail := map[string]any{
		"sourceUUID": sourceUUID,
		"name":       spec.Name,
		"autostart":  spec.Autostart,
		"start":      spec.Start,
	}
	if spec.TargetPool != "" {
		detail["targetPool"] = spec.TargetPool
	}
	return detail
}
