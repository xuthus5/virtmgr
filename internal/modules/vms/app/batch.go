package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"virtmgr/internal/domain"
)

const maxBatchVMCount = 100

type BatchActionCommand struct {
	Action    domain.VMAction
	UUIDs     []string
	Session   domain.Session
	RequestID string
	SourceIP  string
}

func (s *Service) BatchAction(ctx context.Context, command BatchActionCommand) (domain.VMBatchActionResult, error) {
	action, ok := domain.ValidVMAction(string(command.Action))
	if !ok {
		return domain.VMBatchActionResult{}, domain.ErrInvalidInput
	}
	uuids, err := normalizeBatchUUIDs(command.UUIDs)
	if err != nil {
		return domain.VMBatchActionResult{}, err
	}
	result := domain.VMBatchActionResult{Action: action, Items: make([]domain.VMBatchActionItem, 0, len(uuids))}

	if !domain.CanOperateVMs(command.Session.User.Role) {
		return result, s.batchAuditFailure(ctx, command, action, uuids, domain.ErrForbidden)
	}
	if s.isReadOnly() {
		return result, s.batchAuditFailure(ctx, command, action, uuids, domain.ErrReadOnly)
	}

	for _, uuid := range uuids {
		vm, actionErr := s.PerformAction(ctx, ActionCommand{
			UUID:      uuid,
			Action:    action,
			Session:   command.Session,
			RequestID: command.RequestID,
			SourceIP:  command.SourceIP,
		})
		item := domain.VMBatchActionItem{UUID: uuid, Success: actionErr == nil}
		if actionErr != nil {
			item.Error = batchActionErrorMessage(actionErr)
			result.Failed++
		} else {
			item.Name = vm.Name
			item.VM = &vm
			result.Succeeded++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func normalizeBatchUUIDs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maxBatchVMCount {
		return nil, fmt.Errorf("%w: batch size must be between 1 and %d", domain.ErrInvalidInput, maxBatchVMCount)
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		uuid := strings.TrimSpace(value)
		if uuid == "" {
			return nil, fmt.Errorf("%w: VM UUID must not be empty", domain.ErrInvalidInput)
		}
		if _, ok := seen[uuid]; ok {
			continue
		}
		seen[uuid] = struct{}{}
		result = append(result, uuid)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: at least one VM is required", domain.ErrInvalidInput)
	}
	return result, nil
}

func (s *Service) batchAuditFailure(ctx context.Context, command BatchActionCommand, action domain.VMAction, uuids []string, cause error) error {
	return s.mutationFailure(ctx, MutationContext{
		Session:   command.Session,
		RequestID: command.RequestID,
		SourceIP:  command.SourceIP,
	}, "vm.batch", "batch", map[string]any{
		"action": string(action),
		"uuids":  uuids,
		"count":  len(uuids),
	}, cause)
}

func batchActionErrorMessage(err error) string {
	switch {
	case errors.Is(err, domain.ErrActionNotAllowed):
		return "当前虚拟机状态不允许执行该操作"
	case errors.Is(err, domain.ErrNotFound):
		return "虚拟机不存在"
	case errors.Is(err, domain.ErrUnavailable):
		return "后端服务暂不可用"
	case errors.Is(err, domain.ErrConflict):
		return "虚拟机状态已发生变化"
	default:
		return "操作失败，请稍后重试"
	}
}
