package bootstrap

import (
	"context"
	"fmt"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/sqlite"
)

func seedDemoTemplates(ctx context.Context, store *sqlite.Store) error {
	existing, err := store.ListVMTemplates(ctx)
	if err != nil {
		return fmt.Errorf("check demo VM templates: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}
	templates := []domain.VMTemplateSpec{
		{
			Name: "linux-base", Description: "通用 Linux 开发机，2 vCPU / 2 GiB，使用默认 NAT 网络。",
			VCPUs: 2, MemoryMiB: 2048, DiskFormat: "qcow2", Network: "default",
			Firmware: domain.VMFirmwareBIOS, Autostart: false, Start: false,
		},
		{
			Name: "secure-uefi", Description: "UEFI + Secure Boot 基线，预置 cloud-init 初始化脚本。",
			VCPUs: 4, MemoryMiB: 4096, DiskFormat: "qcow2", Network: "default",
			Firmware: domain.VMFirmwareUEFI, SecureBoot: true, CloudInit: true,
			CloudInitUserData: "#cloud-config\npackage_update: true\nruncmd:\n  - echo \"VirtMgr secure template ready\" > /var/log/virtmgr-template.log\n",
			Autostart:         false, Start: false,
		},
	}
	for _, spec := range templates {
		if _, err := store.CreateVMTemplate(ctx, spec); err != nil {
			return fmt.Errorf("seed demo VM template %s: %w", spec.Name, err)
		}
	}
	return nil
}
