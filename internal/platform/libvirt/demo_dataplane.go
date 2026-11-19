package libvirtbackend

import (
	"context"

	"virtmgr/internal/domain"
)

func (d *DemoBackend) RecoverDataPlaneOperation(ctx context.Context, _ domain.DataPlaneOperation) (domain.DataPlaneRecovery, error) {
	if err := ctx.Err(); err != nil {
		return domain.DataPlaneRecovery{}, err
	}
	return domain.DataPlaneRecovery{
		Status:  domain.DataPlaneRecoveryNoop,
		Message: "演示模式没有需要恢复的真实 libvirt 数据面资源",
	}, nil
}
