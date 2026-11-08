package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"virtmgr/internal/domain"
)

type Repository interface {
	Host(ctx context.Context) (domain.Host, error)
	ListHostDevices(ctx context.Context) ([]domain.HostDevice, error)
	ListVMs(ctx context.Context) ([]domain.VM, error)
	ListStoragePools(ctx context.Context) ([]domain.StoragePool, error)
	ListNetworks(ctx context.Context) ([]domain.VirtualNetwork, error)
}

type MetricsProvider interface {
	HostNamed(ctx context.Context, hostName string) (domain.HostMetrics, error)
}

type Service struct {
	repository Repository
	metrics    MetricsProvider
}

type Option func(*Service)

func WithMetrics(provider MetricsProvider) Option {
	return func(service *Service) {
		service.metrics = provider
	}
}

func New(repository Repository, options ...Option) *Service {
	service := &Service{repository: repository}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Metrics(ctx context.Context) (domain.HostMetrics, error) {
	if s.metrics == nil {
		return domain.HostMetrics{}, domain.ErrUnavailable
	}
	host, err := s.repository.Host(ctx)
	if err != nil {
		return domain.HostMetrics{}, err
	}
	return s.metrics.HostNamed(ctx, host.Name)
}

func (s *Service) Devices(ctx context.Context) (domain.HostDeviceList, error) {
	devices, err := s.repository.ListHostDevices(ctx)
	if err != nil {
		return domain.HostDeviceList{}, err
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Type != devices[j].Type {
			return devices[i].Type < devices[j].Type
		}
		return strings.ToLower(devices[i].Name) < strings.ToLower(devices[j].Name)
	})
	return domain.HostDeviceList{Items: devices, Total: len(devices)}, nil
}

func (s *Service) Get(ctx context.Context) (domain.HostDetails, error) {
	host, err := s.repository.Host(ctx)
	if err != nil {
		return domain.HostDetails{}, err
	}
	vms, err := s.repository.ListVMs(ctx)
	if err != nil {
		return domain.HostDetails{}, err
	}
	pools, err := s.repository.ListStoragePools(ctx)
	if err != nil {
		return domain.HostDetails{}, err
	}
	networks, err := s.repository.ListNetworks(ctx)
	if err != nil {
		return domain.HostDetails{}, err
	}

	sort.Slice(vms, func(i, j int) bool { return strings.ToLower(vms[i].Name) < strings.ToLower(vms[j].Name) })
	sort.Slice(pools, func(i, j int) bool { return strings.ToLower(pools[i].Name) < strings.ToLower(pools[j].Name) })
	sort.Slice(networks, func(i, j int) bool { return strings.ToLower(networks[i].Name) < strings.ToLower(networks[j].Name) })

	details := domain.HostDetails{Host: host, VMs: vms, StoragePools: pools, Networks: networks, Counts: domain.VMCounts{Total: len(vms)}}
	for _, vm := range vms {
		details.AllocatedVCPUs += uint64(vm.VCPUs)
		details.AllocatedMemoryBytes += vm.MaxMemoryBytes
		switch vm.State {
		case domain.VMStateRunning, domain.VMStateBlocked:
			details.Counts.Running++
		case domain.VMStatePaused, domain.VMStatePMSuspended:
			details.Counts.Paused++
		case domain.VMStateShutoff:
			details.Counts.Shutoff++
		case domain.VMStateCrashed:
			details.Counts.Failed++
		}
	}
	if details.Counts.Failed > 0 {
		details.Warnings = append(details.Warnings, fmt.Sprintf("%d 台虚拟机处于异常状态", details.Counts.Failed))
	}
	for _, pool := range pools {
		if pool.State == domain.StoragePoolStateDegraded || pool.State == domain.StoragePoolStateInaccessible {
			details.Warnings = append(details.Warnings, fmt.Sprintf("存储池 %s 状态异常（%s）", pool.Name, pool.State))
		}
	}
	return details, nil
}
