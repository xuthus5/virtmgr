package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

type alertRepository struct {
	mu               sync.Mutex
	rules            []domain.AlertRule
	samples          []domain.MetricSample
	guestFilesystems []domain.GuestFilesystemSample
	capacitySamples  []domain.CapacitySample
	audits           []domain.AuditEvent
}

func (f *alertRepository) ListAlertRules(context.Context) ([]domain.AlertRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.AlertRule(nil), f.rules...), nil
}

func (f *alertRepository) GetAlertRule(_ context.Context, id int64) (domain.AlertRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rule := range f.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return domain.AlertRule{}, domain.ErrNotFound
}

func (f *alertRepository) CreateAlertRule(_ context.Context, item domain.AlertRule) (domain.AlertRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item.ID = int64(len(f.rules) + 1)
	f.rules = append(f.rules, item)
	return item, nil
}

func (f *alertRepository) UpdateAlertRule(_ context.Context, item domain.AlertRule) (domain.AlertRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for index := range f.rules {
		if f.rules[index].ID == item.ID {
			f.rules[index] = item
			return item, nil
		}
	}
	return domain.AlertRule{}, domain.ErrNotFound
}

func (f *alertRepository) DeleteAlertRule(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for index := range f.rules {
		if f.rules[index].ID == id {
			f.rules = append(f.rules[:index], f.rules[index+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *alertRepository) InsertMetricSample(_ context.Context, sample domain.MetricSample) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.samples = append(f.samples, sample)
	return nil
}

func (f *alertRepository) ListMetricSamples(context.Context, string, string, time.Time, time.Time, int) ([]domain.MetricSample, error) {
	return nil, nil
}

func (f *alertRepository) DeleteMetricSamplesBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (f *alertRepository) InsertGuestFilesystemSample(_ context.Context, sample domain.GuestFilesystemSample) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.guestFilesystems = append(f.guestFilesystems, sample)
	return nil
}

func (f *alertRepository) ListGuestFilesystemSamples(_ context.Context, vmUUID string, from, to time.Time, _ int) ([]domain.GuestFilesystemSample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]domain.GuestFilesystemSample, 0)
	for _, sample := range f.guestFilesystems {
		if sample.VMUUID == vmUUID && !sample.SampledAt.Before(from) && !sample.SampledAt.After(to) {
			items = append(items, sample)
		}
	}
	return items, nil
}

func (f *alertRepository) DeleteGuestFilesystemSamplesBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (f *alertRepository) InsertCapacitySample(_ context.Context, sample domain.CapacitySample) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.capacitySamples = append(f.capacitySamples, sample)
	return nil
}

func (f *alertRepository) ListCapacitySamples(_ context.Context, scope, targetID string, from, to time.Time, _ int) ([]domain.CapacitySample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := make([]domain.CapacitySample, 0)
	for _, sample := range f.capacitySamples {
		if sample.Scope == scope && (targetID == "" || sample.TargetID == targetID) && !sample.SampledAt.Before(from) && !sample.SampledAt.After(to) {
			items = append(items, sample)
		}
	}
	return items, nil
}

func (f *alertRepository) DeleteCapacitySamplesBefore(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (f *alertRepository) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, event)
	return nil
}

type alertMetrics struct {
	host domain.HostMetrics
	vm   domain.VMMetrics
}

func (f alertMetrics) HostNamed(context.Context, string) (domain.HostMetrics, error) {
	return f.host, nil
}

func (f alertMetrics) VM(context.Context, string) (domain.VMMetrics, error) {
	return f.vm, nil
}

type alertHosts struct {
	pools []domain.StoragePool
	vms   []domain.VM
}

func (alertHosts) Host(context.Context) (domain.Host, error) {
	return domain.Host{Name: "host-a"}, nil
}

func (h alertHosts) ListVMs(context.Context) ([]domain.VM, error) { return h.vms, nil }

func (h alertHosts) ListStoragePools(context.Context) ([]domain.StoragePool, error) {
	return h.pools, nil
}

type alertGuestAgent struct{}

func (alertGuestAgent) GetGuestAgentInfo(context.Context, string, bool) (domain.GuestAgentInfo, error) {
	return domain.GuestAgentInfo{Connected: true, Filesystems: []domain.GuestAgentFilesystem{
		{MountPoint: "/", UsedBytes: 40, TotalBytes: 100},
		{MountPoint: "/var", UsedBytes: 80, TotalBytes: 100},
	}}, nil
}

func TestStoragePoolAlertUsesMostUtilizedActivePool(t *testing.T) {
	pools := []domain.StoragePool{
		{Name: "default", State: domain.StoragePoolStateActive, CapacityBytes: 100, AllocationBytes: 40},
		{Name: "images", State: domain.StoragePoolStateActive, CapacityBytes: 200, AllocationBytes: 150},
		{Name: "offline", State: domain.StoragePoolStateInactive, CapacityBytes: 100, AllocationBytes: 100},
	}
	value, target, ok := ruleValue(domain.AlertMetricStoragePoolPercent, domain.ResourceMetrics{}, nil, pools, nil)
	if !ok || target != "images" || value != 75 {
		t.Fatalf("storage pool rule value = %v target=%q ok=%v", value, target, ok)
	}
	if metric, err := domain.NormalizeAlertMetric("storage_pool_percent"); err != nil || metric != domain.AlertMetricStoragePoolPercent {
		t.Fatalf("NormalizeAlertMetric() = %q, %v", metric, err)
	}
}

func TestGuestFilesystemAlertUsesMostUtilizedFilesystem(t *testing.T) {
	filesystems := []guestFilesystemUsage{
		{VMName: "db-01", MountPoint: "/", Percent: 61},
		{VMName: "web-01", MountPoint: "/var/lib", Percent: 92.5},
		{VMName: "web-01", MountPoint: "/boot", Percent: 44},
	}
	value, target, ok := ruleValue(domain.AlertMetricGuestFilesystemPercent, domain.ResourceMetrics{}, nil, nil, filesystems)
	if !ok || target != "web-01 /var/lib" || value != 92.5 {
		t.Fatalf("guest filesystem rule value = %v target=%q ok=%v", value, target, ok)
	}
	if metric, err := domain.NormalizeAlertMetric("guest_filesystem_percent"); err != nil || metric != domain.AlertMetricGuestFilesystemPercent {
		t.Fatalf("NormalizeAlertMetric() = %q, %v", metric, err)
	}
}

func TestAlertRuleTransitionsAndWebhook(t *testing.T) {
	var notifications []domain.AlertNotification
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var notification domain.AlertNotification
		if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
			t.Errorf("decode webhook: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		notifications = append(notifications, notification)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repository := &alertRepository{rules: []domain.AlertRule{{
		ID: 1, Name: "host cpu", Metric: domain.AlertMetricHostCPUPercent, Operator: domain.AlertOperatorGreaterThan,
		Threshold: 80, DurationMinutes: 5, Enabled: true, WebhookURL: server.URL, LastState: domain.AlertStateOK,
	}}}
	metrics := alertMetrics{host: domain.HostMetrics{ResourceMetrics: domain.ResourceMetrics{CPUPercent: 90}}}
	service := New(repository, metrics, alertHosts{}, repository, WithClock(func() time.Time { return now }), WithHTTPClient(server.Client()))

	service.sampleAndEvaluate(context.Background())
	if rule, _ := repository.GetAlertRule(context.Background(), 1); rule.LastState != domain.AlertStatePending || len(notifications) != 0 {
		t.Fatalf("pending rule = %+v notifications=%d", rule, len(notifications))
	}

	now = now.Add(6 * time.Minute)
	service.sampleAndEvaluate(context.Background())
	rule, _ := repository.GetAlertRule(context.Background(), 1)
	if rule.LastState != domain.AlertStateFiring || rule.LastNotifiedAt == nil || len(notifications) != 1 || notifications[0].State != domain.AlertStateFiring {
		t.Fatalf("firing rule = %+v notifications=%+v", rule, notifications)
	}

	metrics.host.CPUPercent = 10
	service = New(repository, metrics, alertHosts{}, repository, WithClock(func() time.Time { return now.Add(time.Minute) }), WithHTTPClient(server.Client()))
	service.sampleAndEvaluate(context.Background())
	rule, _ = repository.GetAlertRule(context.Background(), 1)
	if rule.LastState != domain.AlertStateOK || len(notifications) != 2 || notifications[1].State != domain.AlertStateOK {
		t.Fatalf("recovery rule = %+v notifications=%+v", rule, notifications)
	}
	if len(repository.samples) != 3 {
		t.Fatalf("host samples = %d", len(repository.samples))
	}
	var firing, recovery bool
	for _, event := range repository.audits {
		firing = firing || event.Action == "alert.firing"
		recovery = recovery || event.Action == "alert.recovery"
	}
	if !firing || !recovery {
		t.Fatalf("alert audit actions = %+v", repository.audits)
	}
}

func TestSampleAndEvaluatePersistsIOPSGuestFilesystemsAndCapacity(t *testing.T) {
	now := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	repository := &alertRepository{}
	metrics := alertMetrics{
		host: domain.HostMetrics{ResourceMetrics: domain.ResourceMetrics{CPUPercent: 10}},
		vm: domain.VMMetrics{UUID: "vm-a", ResourceMetrics: domain.ResourceMetrics{
			DiskReadOperationsPerSecond: 120, DiskWriteOperationsPerSecond: 80,
		}},
	}
	hosts := alertHosts{
		vms:   []domain.VM{{UUID: "vm-a", Name: "app-01", State: domain.VMStateRunning}},
		pools: []domain.StoragePool{{Name: "default", State: domain.StoragePoolStateActive, CapacityBytes: 1000, AllocationBytes: 400}},
	}
	service := New(repository, metrics, hosts, repository, WithClock(func() time.Time { return now }), WithGuestAgentSource(alertGuestAgent{}))
	service.sampleAndEvaluate(context.Background())

	if len(repository.samples) != 2 || repository.samples[1].DiskReadIOPS != 120 || repository.samples[1].DiskWriteIOPS != 80 {
		t.Fatalf("metric samples = %+v", repository.samples)
	}
	if len(repository.guestFilesystems) != 2 || repository.guestFilesystems[0].VMName != "app-01" {
		t.Fatalf("guest filesystem samples = %+v", repository.guestFilesystems)
	}
	if len(repository.capacitySamples) != 1 || repository.capacitySamples[0].TargetID != "default" || repository.capacitySamples[0].UsedBytes != 400 {
		t.Fatalf("capacity samples = %+v", repository.capacitySamples)
	}
}

func TestCapacityForecastUsesLinearGrowth(t *testing.T) {
	now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	const gib = uint64(1 << 30)
	repository := &alertRepository{capacitySamples: []domain.CapacitySample{
		{Scope: "storage_pool", TargetID: "data", UsedBytes: 2 * gib, TotalBytes: 10 * gib, SampledAt: now.Add(-72 * time.Hour)},
		{Scope: "storage_pool", TargetID: "data", UsedBytes: 3 * gib, TotalBytes: 10 * gib, SampledAt: now.Add(-48 * time.Hour)},
		{Scope: "storage_pool", TargetID: "data", UsedBytes: 4 * gib, TotalBytes: 10 * gib, SampledAt: now.Add(-24 * time.Hour)},
		{Scope: "storage_pool", TargetID: "data", UsedBytes: 5 * gib, TotalBytes: 10 * gib, SampledAt: now},
	}}
	service := New(repository, alertMetrics{}, alertHosts{}, repository, WithClock(func() time.Time { return now }))
	result, err := service.CapacityForecast(context.Background(), CapacityForecastQuery{Scope: "storage_pool"})
	if err != nil {
		t.Fatalf("CapacityForecast() error = %v", err)
	}
	if result.Total != 1 || result.Items[0].UsagePercent != 50 {
		t.Fatalf("forecast = %+v", result)
	}
	item := result.Items[0]
	if item.GrowthBytesPerDay < float64(gib)-1 || item.GrowthBytesPerDay > float64(gib)+1 {
		t.Fatalf("growth = %v", item.GrowthBytesPerDay)
	}
	if item.DaysToFull == nil || *item.DaysToFull < 4.99 || *item.DaysToFull > 5.01 {
		t.Fatalf("days to full = %v", item.DaysToFull)
	}
}
