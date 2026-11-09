package app

import (
	"context"

	"virtmgr/internal/domain"
)

// captureConfigVersion archives a successful persistent-domain mutation. The
// mutation itself already succeeded, so history capture is deliberately
// best-effort and records its own audit result instead of masking the outcome.
func (s *Service) captureConfigVersion(ctx context.Context, request MutationContext, vmUUID string, source domain.VMConfigVersionSource, sourceVersion int, note string) {
	if s.configVersions == nil || vmUUID == "" {
		return
	}
	detail := map[string]any{
		"source":        source,
		"sourceVersion": sourceVersion,
	}
	item, err := s.configVersions.CaptureVM(ctx, vmUUID, domain.VMConfigCapture{
		Source:        source,
		SourceVersion: sourceVersion,
		Actor:         request.Session.User.Username,
		Role:          request.Session.User.Role,
		RequestID:     request.RequestID,
		SourceIP:      request.SourceIP,
		Note:          note,
	})
	if err != nil {
		_ = s.recordMutationAudit(ctx, request, "vm.config.version.create", vmUUID, "failure", detail, err)
		return
	}
	detail["version"] = item.VersionNo
	detail["xmlSha256"] = item.XMLSHA256
	detail["sizeBytes"] = item.SizeBytes
	_ = s.recordMutationAudit(ctx, request, "vm.config.version.create", vmUUID, "success", detail, nil)
}
