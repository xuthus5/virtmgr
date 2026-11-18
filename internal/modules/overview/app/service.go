package app

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"virtmgr/internal/domain"
)

type Repository interface {
	Host(ctx context.Context) (domain.Host, error)
	ListVMs(ctx context.Context) ([]domain.VM, error)
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Get(ctx context.Context) (domain.Overview, error) {
	host, err := s.repository.Host(ctx)
	if err != nil {
		return domain.Overview{}, err
	}
	vms, err := s.repository.ListVMs(ctx)
	if err != nil {
		return domain.Overview{}, err
	}
	slices.SortFunc(vms, func(left, right domain.VM) int {
		return cmp.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
	counts := domain.VMCounts{Total: len(vms)}
	var allocatedVCPUs uint64
	var allocatedMemoryBytes uint64
	for _, vm := range vms {
		allocatedVCPUs += uint64(vm.VCPUs)
		allocatedMemoryBytes += vm.MaxMemoryBytes
		switch vm.State {
		case domain.VMStateRunning, domain.VMStateBlocked:
			counts.Running++
		case domain.VMStatePaused, domain.VMStatePMSuspended:
			counts.Paused++
		case domain.VMStateShutoff:
			counts.Shutoff++
		case domain.VMStateCrashed:
			counts.Failed++
		}
	}
	preview := vms
	if len(preview) > 8 {
		preview = preview[:8]
	}
	return domain.Overview{
		Host: host, Counts: counts, AllocatedVCPUs: allocatedVCPUs,
		AllocatedMemoryBytes: allocatedMemoryBytes, VMs: preview,
	}, nil
}
