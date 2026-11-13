package domain

import (
	"fmt"
	"strings"
	"time"
)

type AlertMetric string

const (
	AlertMetricHostCPUPercent    AlertMetric = "host_cpu_percent"
	AlertMetricHostMemoryPercent AlertMetric = "host_memory_percent"
	AlertMetricVMCPUPercent      AlertMetric = "vm_cpu_percent"
	AlertMetricVMMemoryPercent   AlertMetric = "vm_memory_percent"
)

func NormalizeAlertMetric(value string) (AlertMetric, error) {
	metric := AlertMetric(strings.ToLower(strings.TrimSpace(value)))
	switch metric {
	case AlertMetricHostCPUPercent, AlertMetricHostMemoryPercent, AlertMetricVMCPUPercent, AlertMetricVMMemoryPercent:
		return metric, nil
	default:
		return "", fmt.Errorf("%w: unsupported alert metric", ErrInvalidInput)
	}
}

type AlertOperator string

const (
	AlertOperatorGreaterThan        AlertOperator = "gt"
	AlertOperatorGreaterThanOrEqual AlertOperator = "gte"
	AlertOperatorLessThan           AlertOperator = "lt"
	AlertOperatorLessThanOrEqual    AlertOperator = "lte"
)

func NormalizeAlertOperator(value string) (AlertOperator, error) {
	operator := AlertOperator(strings.ToLower(strings.TrimSpace(value)))
	switch operator {
	case AlertOperatorGreaterThan, AlertOperatorGreaterThanOrEqual, AlertOperatorLessThan, AlertOperatorLessThanOrEqual:
		return operator, nil
	default:
		return "", fmt.Errorf("%w: unsupported alert operator", ErrInvalidInput)
	}
}

type AlertState string

const (
	AlertStateOK      AlertState = "ok"
	AlertStatePending AlertState = "pending"
	AlertStateFiring  AlertState = "firing"
)

type AlertRule struct {
	ID              int64         `json:"id"`
	Name            string        `json:"name"`
	Metric          AlertMetric   `json:"metric"`
	Operator        AlertOperator `json:"operator"`
	Threshold       float64       `json:"threshold"`
	DurationMinutes int           `json:"durationMinutes"`
	Enabled         bool          `json:"enabled"`
	WebhookURL      string        `json:"webhookUrl"`
	LastState       AlertState    `json:"lastState"`
	LastChangedAt   *time.Time    `json:"lastChangedAt,omitempty"`
	LastNotifiedAt  *time.Time    `json:"lastNotifiedAt,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type AlertRuleSpec struct {
	Name            string        `json:"name"`
	Metric          AlertMetric   `json:"metric"`
	Operator        AlertOperator `json:"operator"`
	Threshold       float64       `json:"threshold"`
	DurationMinutes int           `json:"durationMinutes"`
	Enabled         bool          `json:"enabled"`
	WebhookURL      string        `json:"webhookUrl"`
}

type AlertRuleList struct {
	Items []AlertRule `json:"items"`
	Total int         `json:"total"`
}

type AlertNotification struct {
	Rule       AlertRule  `json:"rule"`
	State      AlertState `json:"state"`
	Value      float64    `json:"value"`
	Target     string     `json:"target,omitempty"`
	Message    string     `json:"message"`
	OccurredAt time.Time  `json:"occurredAt"`
}

type MetricSample struct {
	ID                      int64     `json:"id"`
	Scope                   string    `json:"scope"`
	TargetID                string    `json:"targetId"`
	SampledAt               time.Time `json:"sampledAt"`
	CPUPercent              float64   `json:"cpuPercent"`
	MemoryUsedBytes         uint64    `json:"memoryUsedBytes"`
	MemoryTotalBytes        uint64    `json:"memoryTotalBytes"`
	DiskReadBytesPerSecond  uint64    `json:"diskReadBytesPerSecond"`
	DiskWriteBytesPerSecond uint64    `json:"diskWriteBytesPerSecond"`
	NetworkRxBytesPerSecond uint64    `json:"networkRxBytesPerSecond"`
	NetworkTxBytesPerSecond uint64    `json:"networkTxBytesPerSecond"`
}

type MetricSampleList struct {
	Items []MetricSample `json:"items"`
	Total int            `json:"total"`
}

func MemoryPercent(sample MetricSample) float64 {
	if sample.MemoryTotalBytes == 0 {
		return 0
	}
	return float64(sample.MemoryUsedBytes) / float64(sample.MemoryTotalBytes) * 100
}
