package bootstrap

import (
	"context"

	"virtmgr/internal/domain"
	"virtmgr/internal/platform/sqlite"
)

func seedDemoImages(ctx context.Context, store *sqlite.Store) error {
	existing, err := store.ListVMImages(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	_, err = store.CreateVMImage(ctx, domain.VMImage{
		Name: "ubuntu-24.04-cloud-base", Description: "Ubuntu 24.04 LTS Cloud Image 演示镜像",
		Pool: "default", Volume: "demo-build-01.qcow2", Format: "qcow2",
		OSFamily: "linux", OSName: "Ubuntu 24.04 LTS", Architecture: "x86_64",
		CapacityBytes: 30 * 1024 * 1024 * 1024, AllocationBytes: 12 * 1024 * 1024 * 1024,
	})
	return err
}
