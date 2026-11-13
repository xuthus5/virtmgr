package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"virtmgr/internal/domain"
)

const (
	sampleInterval       = time.Minute
	historyRetention     = 7 * 24 * time.Hour
	maxHistoryRange      = 7 * 24 * time.Hour
	defaultHistoryRange  = 24 * time.Hour
	maxHistorySamples    = 5000
	defaultAlertDuration = 0
)

type Repository interface {
	ListAlertRules(ctx context.Context) ([]domain.AlertRule, error)
	GetAlertRule(ctx context.Context, id int64) (domain.AlertRule, error)
	CreateAlertRule(ctx context.Context, item domain.AlertRule) (domain.AlertRule, error)
	UpdateAlertRule(ctx context.Context, item domain.AlertRule) (domain.AlertRule, error)
	DeleteAlertRule(ctx context.Context, id int64) error
	InsertMetricSample(ctx context.Context, sample domain.MetricSample) error
	ListMetricSamples(ctx context.Context, scope, targetID string, from, to time.Time, limit int) ([]domain.MetricSample, error)
	DeleteMetricSamplesBefore(ctx context.Context, before time.Time) (int64, error)
}

type MetricsSource interface {
	HostNamed(ctx context.Context, hostName string) (domain.HostMetrics, error)
	VM(ctx context.Context, uuid string) (domain.VMMetrics, error)
}

type HostSource interface {
	Host(ctx context.Context) (domain.Host, error)
	ListVMs(ctx context.Context) ([]domain.VM, error)
}

type AuditRecorder interface {
	RecordAudit(ctx context.Context, event domain.AuditEvent) error
}

type MutationContext struct {
	Session   domain.Session
	RequestID string
	SourceIP  string
}

type AlertRuleCommand struct {
	ID      int64
	Spec    domain.AlertRuleSpec
	Request MutationContext
}

type HistoryQuery struct {
	Scope    string
	TargetID string
	From     time.Time
	To       time.Time
	Limit    int
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func WithErrorHandler(handler func(error)) Option {
	return func(service *Service) { service.onError = handler }
}

func WithHTTPClient(client *http.Client) Option {
	return func(service *Service) {
		if client != nil {
			service.client = client
		}
	}
}

type Service struct {
	repository Repository
	metrics    MetricsSource
	hosts      HostSource
	audit      AuditRecorder
	now        func() time.Time
	client     *http.Client
	onError    func(error)
}

func New(repository Repository, metrics MetricsSource, hosts HostSource, audit AuditRecorder, options ...Option) *Service {
	service := &Service{
		repository: repository,
		metrics:    metrics,
		hosts:      hosts,
		audit:      audit,
		now:        func() time.Time { return time.Now().UTC() },
		client:     &http.Client{Timeout: 5 * time.Second},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) ListRules(ctx context.Context) (domain.AlertRuleList, error) {
	items, err := s.repository.ListAlertRules(ctx)
	if err != nil {
		return domain.AlertRuleList{}, err
	}
	return domain.AlertRuleList{Items: items, Total: len(items)}, nil
}

func (s *Service) CreateRule(ctx context.Context, command AlertRuleCommand) (domain.AlertRule, error) {
	if !canManageAlerts(command.Request.Session.User.Role) {
		return domain.AlertRule{}, domain.ErrForbidden
	}
	spec, err := normalizeAlertSpec(command.Spec)
	if err != nil {
		return domain.AlertRule{}, err
	}
	now := s.now().UTC()
	item, err := s.repository.CreateAlertRule(ctx, domain.AlertRule{
		Name: spec.Name, Metric: spec.Metric, Operator: spec.Operator, Threshold: spec.Threshold,
		DurationMinutes: spec.DurationMinutes, Enabled: spec.Enabled, WebhookURL: spec.WebhookURL,
		LastState: domain.AlertStateOK, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return domain.AlertRule{}, s.auditFailure(ctx, command.Request, "alert.rule.create", spec.Name, err)
	}
	if err := s.recordAudit(ctx, command.Request, "alert.rule.create", fmt.Sprint(item.ID), "success", alertDetail(item), nil); err != nil {
		return domain.AlertRule{}, err
	}
	return item, nil
}

func (s *Service) UpdateRule(ctx context.Context, command AlertRuleCommand) (domain.AlertRule, error) {
	if !canManageAlerts(command.Request.Session.User.Role) {
		return domain.AlertRule{}, domain.ErrForbidden
	}
	if command.ID < 1 {
		return domain.AlertRule{}, domain.ErrInvalidInput
	}
	spec, err := normalizeAlertSpec(command.Spec)
	if err != nil {
		return domain.AlertRule{}, err
	}
	item, err := s.repository.GetAlertRule(ctx, command.ID)
	if err != nil {
		return domain.AlertRule{}, err
	}
	item.Name = spec.Name
	item.Metric = spec.Metric
	item.Operator = spec.Operator
	item.Threshold = spec.Threshold
	item.DurationMinutes = spec.DurationMinutes
	item.Enabled = spec.Enabled
	item.WebhookURL = spec.WebhookURL
	item.UpdatedAt = s.now().UTC()
	updated, err := s.repository.UpdateAlertRule(ctx, item)
	if err != nil {
		return domain.AlertRule{}, s.auditFailure(ctx, command.Request, "alert.rule.update", fmt.Sprint(command.ID), err)
	}
	if err := s.recordAudit(ctx, command.Request, "alert.rule.update", fmt.Sprint(item.ID), "success", alertDetail(updated), nil); err != nil {
		return domain.AlertRule{}, err
	}
	return updated, nil
}

func (s *Service) DeleteRule(ctx context.Context, command AlertRuleCommand) error {
	if !canManageAlerts(command.Request.Session.User.Role) {
		return domain.ErrForbidden
	}
	if command.ID < 1 {
		return domain.ErrInvalidInput
	}
	if err := s.repository.DeleteAlertRule(ctx, command.ID); err != nil {
		return s.auditFailure(ctx, command.Request, "alert.rule.delete", fmt.Sprint(command.ID), err)
	}
	return s.recordAudit(ctx, command.Request, "alert.rule.delete", fmt.Sprint(command.ID), "success", map[string]any{"ruleId": command.ID}, nil)
}

func (s *Service) History(ctx context.Context, input HistoryQuery) (domain.MetricSampleList, error) {
	scope := strings.ToLower(strings.TrimSpace(input.Scope))
	if scope != "host" && scope != "vm" {
		return domain.MetricSampleList{}, domain.ErrInvalidInput
	}
	targetID := strings.TrimSpace(input.TargetID)
	if targetID == "" {
		return domain.MetricSampleList{}, domain.ErrInvalidInput
	}
	to := input.To.UTC()
	if to.IsZero() {
		to = s.now().UTC()
	}
	from := input.From.UTC()
	if from.IsZero() {
		from = to.Add(-defaultHistoryRange)
	}
	if !from.Before(to) || to.Sub(from) > maxHistoryRange {
		return domain.MetricSampleList{}, domain.ErrInvalidInput
	}
	limit := input.Limit
	if limit < 1 || limit > maxHistorySamples {
		limit = 1000
	}
	items, err := s.repository.ListMetricSamples(ctx, scope, targetID, from, to, limit)
	if err != nil {
		return domain.MetricSampleList{}, err
	}
	return domain.MetricSampleList{Items: items, Total: len(items)}, nil
}

func (s *Service) Run(ctx context.Context) {
	s.sampleAndEvaluate(ctx)
	cleanupTicker := time.NewTicker(6 * time.Hour)
	defer cleanupTicker.Stop()
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sampleAndEvaluate(ctx)
		case <-cleanupTicker.C:
			if _, err := s.repository.DeleteMetricSamplesBefore(ctx, s.now().UTC().Add(-historyRetention)); err != nil && s.onError != nil {
				s.onError(err)
			}
		}
	}
}

func (s *Service) sampleAndEvaluate(ctx context.Context) {
	host, err := s.hosts.Host(ctx)
	if err != nil {
		s.report(err)
		return
	}
	hostMetrics, err := s.metrics.HostNamed(ctx, host.Name)
	if err != nil {
		s.report(err)
		return
	}
	now := s.now().UTC()
	hostSample := sampleFromMetrics("host", "host", now, hostMetrics.ResourceMetrics)
	if err := s.repository.InsertMetricSample(ctx, hostSample); err != nil {
		s.report(err)
	}

	vms, err := s.hosts.ListVMs(ctx)
	if err != nil {
		s.report(err)
		return
	}
	vmSamples := make([]domain.MetricSample, 0, len(vms))
	for _, vm := range vms {
		if vm.State != domain.VMStateRunning && vm.State != domain.VMStateBlocked {
			continue
		}
		metrics, metricsErr := s.metrics.VM(ctx, vm.UUID)
		if metricsErr != nil {
			s.report(fmt.Errorf("sample VM %s: %w", vm.UUID, metricsErr))
			continue
		}
		sample := sampleFromMetrics("vm", vm.UUID, now, metrics.ResourceMetrics)
		vmSamples = append(vmSamples, sample)
		if err := s.repository.InsertMetricSample(ctx, sample); err != nil {
			s.report(err)
		}
	}
	s.evaluateRules(ctx, hostMetrics.ResourceMetrics, vmSamples, now)
}

func sampleFromMetrics(scope, targetID string, sampledAt time.Time, metrics domain.ResourceMetrics) domain.MetricSample {
	return domain.MetricSample{
		Scope: scope, TargetID: targetID, SampledAt: sampledAt, CPUPercent: metrics.CPUPercent,
		MemoryUsedBytes: metrics.MemoryUsedBytes, MemoryTotalBytes: metrics.MemoryTotalBytes,
		DiskReadBytesPerSecond: metrics.DiskReadBytesPerSecond, DiskWriteBytesPerSecond: metrics.DiskWriteBytesPerSecond,
		NetworkRxBytesPerSecond: metrics.NetworkRxBytesPerSecond, NetworkTxBytesPerSecond: metrics.NetworkTxBytesPerSecond,
	}
}

func (s *Service) evaluateRules(ctx context.Context, host domain.ResourceMetrics, vms []domain.MetricSample, now time.Time) {
	rules, err := s.repository.ListAlertRules(ctx)
	if err != nil {
		s.report(err)
		return
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		value, target, ok := ruleValue(rule.Metric, host, vms)
		if !ok {
			continue
		}
		s.applyRule(ctx, rule, value, target, now)
	}
}

func ruleValue(metric domain.AlertMetric, host domain.ResourceMetrics, vms []domain.MetricSample) (float64, string, bool) {
	hostMemory := percent(host.MemoryUsedBytes, host.MemoryTotalBytes)
	switch metric {
	case domain.AlertMetricHostCPUPercent:
		return host.CPUPercent, "host", true
	case domain.AlertMetricHostMemoryPercent:
		return hostMemory, "host", true
	case domain.AlertMetricVMCPUPercent:
		return maxVMValue(vms, func(sample domain.MetricSample) float64 { return sample.CPUPercent })
	case domain.AlertMetricVMMemoryPercent:
		return maxVMValue(vms, func(sample domain.MetricSample) float64 {
			return percent(sample.MemoryUsedBytes, sample.MemoryTotalBytes)
		})
	default:
		return 0, "", false
	}
}

func maxVMValue(samples []domain.MetricSample, value func(domain.MetricSample) float64) (float64, string, bool) {
	if len(samples) == 0 {
		return 0, "", false
	}
	best := samples[0]
	bestValue := value(best)
	for _, sample := range samples[1:] {
		current := value(sample)
		if current > bestValue {
			best = sample
			bestValue = current
		}
	}
	return bestValue, best.TargetID, true
}

func (s *Service) applyRule(ctx context.Context, rule domain.AlertRule, value float64, target string, now time.Time) {
	breached := compare(value, rule.Operator, rule.Threshold)
	state := rule.LastState
	if state == "" {
		state = domain.AlertStateOK
	}
	if breached {
		switch state {
		case domain.AlertStateOK:
			changed := now
			rule.LastState = domain.AlertStatePending
			rule.LastChangedAt = &changed
			rule.UpdatedAt = now
			if rule.DurationMinutes <= defaultAlertDuration {
				rule.LastState = domain.AlertStateFiring
			}
			_, _ = s.repository.UpdateAlertRule(ctx, rule)
			if rule.LastState == domain.AlertStateFiring {
				s.notify(ctx, rule, value, target, now)
			}
		case domain.AlertStatePending:
			if rule.LastChangedAt == nil || now.Sub(*rule.LastChangedAt) >= time.Duration(rule.DurationMinutes)*time.Minute {
				rule.LastState = domain.AlertStateFiring
				rule.UpdatedAt = now
				_, _ = s.repository.UpdateAlertRule(ctx, rule)
				s.notify(ctx, rule, value, target, now)
			}
		case domain.AlertStateFiring:
			if rule.LastNotifiedAt == nil && strings.TrimSpace(rule.WebhookURL) != "" {
				s.notify(ctx, rule, value, target, now)
			}
		}
		return
	}
	if state == domain.AlertStatePending || state == domain.AlertStateFiring {
		wasFiring := state == domain.AlertStateFiring
		changed := now
		rule.LastState = domain.AlertStateOK
		rule.LastChangedAt = &changed
		rule.UpdatedAt = now
		_, _ = s.repository.UpdateAlertRule(ctx, rule)
		if wasFiring {
			s.notify(ctx, rule, value, target, now)
		}
	}
}

func (s *Service) notify(ctx context.Context, rule domain.AlertRule, value float64, target string, now time.Time) {
	message := alertMessage(rule, value, target)
	notification := domain.AlertNotification{
		Rule: rule, State: rule.LastState, Value: value, Target: target, Message: message, OccurredAt: now,
	}
	if strings.TrimSpace(rule.WebhookURL) != "" {
		if err := s.postWebhook(ctx, rule.WebhookURL, notification); err != nil {
			s.report(fmt.Errorf("alert %d webhook: %w", rule.ID, err))
		} else {
			notified := now
			rule.LastNotifiedAt = &notified
			rule.UpdatedAt = now
			_, _ = s.repository.UpdateAlertRule(ctx, rule)
		}
	}
	action := "alert.firing"
	if rule.LastState == domain.AlertStateOK {
		action = "alert.recovery"
	}
	_ = s.recordAudit(ctx, MutationContext{Session: domain.Session{User: domain.User{Username: "system:alerts", Role: domain.RoleAdmin}}, RequestID: "alert-evaluator", SourceIP: "scheduler"}, action, fmt.Sprint(rule.ID), string(rule.LastState), map[string]any{
		"ruleName": rule.Name, "metric": rule.Metric, "value": value, "threshold": rule.Threshold,
		"operator": rule.Operator, "target": target, "durationMinutes": rule.DurationMinutes, "message": message,
	}, nil)
}

func (s *Service) postWebhook(ctx context.Context, endpoint string, notification domain.AlertNotification) error {
	body, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "virtmgr-alert/1")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", response.StatusCode)
	}
	return nil
}

func (s *Service) report(err error) {
	if err != nil && s.onError != nil {
		s.onError(err)
	}
}

func (s *Service) recordAudit(ctx context.Context, request MutationContext, action, targetID, result string, detail map[string]any, cause error) error {
	if s.audit == nil {
		return nil
	}
	if detail == nil {
		detail = map[string]any{}
	}
	if cause != nil {
		detail["error"] = cause.Error()
	}
	return s.audit.RecordAudit(ctx, domain.AuditEvent{
		OccurredAt: s.now().UTC(), RequestID: request.RequestID, Actor: request.Session.User.Username,
		Role: request.Session.User.Role, Action: action, TargetKind: "alert_rule", TargetID: targetID,
		SourceIP: request.SourceIP, Result: result, Detail: detail,
	})
}

func (s *Service) auditFailure(ctx context.Context, request MutationContext, action, targetID string, cause error) error {
	if err := s.recordAudit(ctx, request, action, targetID, "failure", nil, cause); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func alertDetail(rule domain.AlertRule) map[string]any {
	return map[string]any{
		"ruleId": rule.ID, "name": rule.Name, "metric": rule.Metric, "operator": rule.Operator,
		"threshold": rule.Threshold, "durationMinutes": rule.DurationMinutes, "enabled": rule.Enabled,
		"webhookConfigured": rule.WebhookURL != "",
	}
}

func alertMessage(rule domain.AlertRule, value float64, target string) string {
	state := "指标恢复正常"
	if rule.LastState == domain.AlertStateFiring {
		state = "指标超过阈值"
	}
	return fmt.Sprintf("告警规则 %s %s：%s %.2f %s %.2f", rule.Name, target, state, value, operatorLabel(rule.Operator), rule.Threshold)
}

func operatorLabel(operator domain.AlertOperator) string {
	switch operator {
	case domain.AlertOperatorGreaterThan:
		return ">"
	case domain.AlertOperatorGreaterThanOrEqual:
		return ">="
	case domain.AlertOperatorLessThan:
		return "<"
	case domain.AlertOperatorLessThanOrEqual:
		return "<="
	default:
		return string(operator)
	}
}

func compare(value float64, operator domain.AlertOperator, threshold float64) bool {
	switch operator {
	case domain.AlertOperatorGreaterThan:
		return value > threshold
	case domain.AlertOperatorGreaterThanOrEqual:
		return value >= threshold
	case domain.AlertOperatorLessThan:
		return value < threshold
	case domain.AlertOperatorLessThanOrEqual:
		return value <= threshold
	default:
		return false
	}
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func canManageAlerts(role domain.Role) bool {
	return role == domain.RoleAdmin
}

var alertNamePattern = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}_. -]{0,63}$`)

func normalizeAlertSpec(spec domain.AlertRuleSpec) (domain.AlertRuleSpec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	if !alertNamePattern.MatchString(spec.Name) {
		return spec, fmt.Errorf("%w: invalid alert rule name", domain.ErrInvalidInput)
	}
	metric, err := domain.NormalizeAlertMetric(string(spec.Metric))
	if err != nil {
		return spec, err
	}
	operator, err := domain.NormalizeAlertOperator(string(spec.Operator))
	if err != nil {
		return spec, err
	}
	spec.Metric = metric
	spec.Operator = operator
	if spec.Threshold < 0 || spec.Threshold > 1_000_000 {
		return spec, fmt.Errorf("%w: threshold is out of range", domain.ErrInvalidInput)
	}
	if spec.DurationMinutes < 0 || spec.DurationMinutes > 24*60 {
		return spec, fmt.Errorf("%w: duration must be between 0 and 1440 minutes", domain.ErrInvalidInput)
	}
	spec.WebhookURL = strings.TrimSpace(spec.WebhookURL)
	if spec.WebhookURL != "" {
		parsed, err := url.Parse(spec.WebhookURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return spec, fmt.Errorf("%w: webhook URL must be an absolute http(s) URL", domain.ErrInvalidInput)
		}
	}
	return spec, nil
}
