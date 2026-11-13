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
	mu      sync.Mutex
	rules   []domain.AlertRule
	samples []domain.MetricSample
	audits  []domain.AuditEvent
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

func (f *alertRepository) RecordAudit(_ context.Context, event domain.AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, event)
	return nil
}

type alertMetrics struct {
	host domain.HostMetrics
}

func (f alertMetrics) HostNamed(context.Context, string) (domain.HostMetrics, error) {
	return f.host, nil
}

func (alertMetrics) VM(context.Context, string) (domain.VMMetrics, error) {
	return domain.VMMetrics{}, nil
}

type alertHosts struct{}

func (alertHosts) Host(context.Context) (domain.Host, error) {
	return domain.Host{Name: "host-a"}, nil
}

func (alertHosts) ListVMs(context.Context) ([]domain.VM, error) { return nil, nil }

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
