package domain

type Settings struct {
	ReadOnly           bool `json:"readOnly"`
	AuditRetentionDays int  `json:"auditRetentionDays"`
}
