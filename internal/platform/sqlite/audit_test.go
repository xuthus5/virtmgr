package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestRecordAudit(t *testing.T) {
	store := openTestDB(t)
	event := domain.AuditEvent{
		OccurredAt: time.Unix(1_700_000_000, 0),
		RequestID:  "req-1",
		Actor:      "admin",
		Role:       domain.RoleAdmin,
		Action:     "vm.start",
		TargetKind: "vm",
		TargetID:   "uuid-1",
		SourceIP:   "127.0.0.1",
		Result:     "success",
		Detail:     map[string]any{"previousState": "shutoff"},
	}
	if err := store.RecordAudit(context.Background(), event); err != nil {
		t.Fatalf("RecordAudit() error = %v", err)
	}
	var actor, action, result, detailJSON string
	if err := store.db.QueryRow(`SELECT actor, action, result, detail_json FROM audit_events`).Scan(&actor, &action, &result, &detailJSON); err != nil {
		t.Fatal(err)
	}
	if actor != "admin" || action != "vm.start" || result != "success" {
		t.Fatalf("audit row = %q %q %q", actor, action, result)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(detailJSON), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["previousState"] != "shutoff" {
		t.Fatalf("detail = %+v", detail)
	}
}

func TestListAuditEvents(t *testing.T) {
	store := openTestDB(t)
	events := []domain.AuditEvent{
		{
			OccurredAt: time.Unix(1_700_000_000, 0), RequestID: "req-1", Actor: "admin", Role: domain.RoleAdmin,
			Action: "vm.start", TargetKind: "vm", TargetID: "uuid-1", SourceIP: "127.0.0.1", Result: "success",
			Detail: map[string]any{"previousState": "shutoff"},
		},
		{
			OccurredAt: time.Unix(1_700_000_100, 0), RequestID: "req-2", Actor: "operator", Role: domain.RoleOperator,
			Action: "storage.volume.create", TargetKind: "storage_volume", TargetID: "default/data.qcow2", SourceIP: "10.0.0.2", Result: "success",
			Detail: map[string]any{"format": "qcow2", "capacityBytes": 1024},
		},
		{
			OccurredAt: time.Unix(1_700_000_200, 0), RequestID: "req-3", Actor: "admin", Role: domain.RoleAdmin,
			Action: "vm.start", TargetKind: "vm", TargetID: "uuid-2", SourceIP: "127.0.0.1", Result: "failure",
			Detail: map[string]any{"error": "domain is locked"},
		},
	}
	for _, event := range events {
		if err := store.RecordAudit(context.Background(), event); err != nil {
			t.Fatalf("RecordAudit() error = %v", err)
		}
	}

	firstPage, err := store.ListAuditEvents(context.Background(), domain.AuditEventQuery{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if firstPage.Total != 3 || len(firstPage.Items) != 2 || firstPage.Page != 1 || firstPage.PageSize != 2 {
		t.Fatalf("first page = %+v", firstPage)
	}
	if firstPage.Items[0].RequestID != "req-3" || firstPage.Items[1].RequestID != "req-2" {
		t.Fatalf("first page order = %+v", firstPage.Items)
	}
	if !firstPage.Items[0].OccurredAt.Equal(time.Unix(1_700_000_200, 0)) || firstPage.Items[0].Detail["error"] != "domain is locked" {
		t.Fatalf("first page event = %+v", firstPage.Items[0])
	}

	secondPage, err := store.ListAuditEvents(context.Background(), domain.AuditEventQuery{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("ListAuditEvents() second page error = %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].RequestID != "req-1" || secondPage.Items[0].Detail["previousState"] != "shutoff" {
		t.Fatalf("second page = %+v", secondPage)
	}
}

func TestListAuditEventsFilters(t *testing.T) {
	store := openTestDB(t)
	events := []domain.AuditEvent{
		{OccurredAt: time.Unix(1_700_000_000, 0), Actor: "admin", Role: domain.RoleAdmin, Action: "vm.start", TargetKind: "vm", TargetID: "uuid-1", Result: "success"},
		{OccurredAt: time.Unix(1_700_000_100, 0), Actor: "admin", Role: domain.RoleAdmin, Action: "vm.start", TargetKind: "vm", TargetID: "uuid-2", Result: "failure"},
		{OccurredAt: time.Unix(1_700_000_200, 0), Actor: "operator", Role: domain.RoleOperator, Action: "storage.volume.create", TargetKind: "storage_volume", TargetID: "default/data.qcow2", Result: "success"},
	}
	for _, event := range events {
		if err := store.RecordAudit(context.Background(), event); err != nil {
			t.Fatalf("RecordAudit() error = %v", err)
		}
	}

	filtered, err := store.ListAuditEvents(context.Background(), domain.AuditEventQuery{
		Actor: " admin ", Action: "vm.start", TargetKind: "vm", TargetID: " uuid-2 ", Result: "failure",
	})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].TargetID != "uuid-2" {
		t.Fatalf("filtered events = %+v", filtered)
	}

	empty, err := store.ListAuditEvents(context.Background(), domain.AuditEventQuery{Action: "vm.delete"})
	if err != nil {
		t.Fatalf("ListAuditEvents() empty error = %v", err)
	}
	if empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("empty events = %+v", empty)
	}

	rangeFiltered, err := store.ListAuditEvents(context.Background(), domain.AuditEventQuery{
		From: time.Unix(1_700_000_050, 0), To: time.Unix(1_700_000_150, 0),
	})
	if err != nil || rangeFiltered.Total != 1 || rangeFiltered.Items[0].TargetID != "uuid-2" {
		t.Fatalf("range filtered events = %+v, error = %v", rangeFiltered, err)
	}
	exported, err := store.ExportAuditEvents(context.Background(), domain.AuditEventQuery{TargetID: "uuid-1"})
	if err != nil || len(exported) != 1 || exported[0].Actor != "admin" {
		t.Fatalf("exported events = %+v, error = %v", exported, err)
	}
}

func TestListAuditEventsOrdersSameSecondByID(t *testing.T) {
	store := openTestDB(t)
	occurredAt := time.Unix(1_700_000_000, 0)
	for _, action := range []string{"vm.start", "vm.shutdown"} {
		if err := store.RecordAudit(context.Background(), domain.AuditEvent{
			OccurredAt: occurredAt, Actor: "admin", Role: domain.RoleAdmin, Action: action,
			TargetKind: "vm", TargetID: "uuid-1", Result: "success",
		}); err != nil {
			t.Fatalf("RecordAudit() error = %v", err)
		}
	}
	result, err := store.ListAuditEvents(context.Background(), domain.AuditEventQuery{})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(result.Items) != 2 || result.Items[0].Action != "vm.shutdown" || result.Items[1].Action != "vm.start" {
		t.Fatalf("events = %+v", result.Items)
	}
	if result.Page != 1 || result.PageSize != domain.DefaultAuditPageSize {
		t.Fatalf("pagination = %d/%d", result.Page, result.PageSize)
	}
}

func TestDeleteAuditEventsBefore(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	for _, timestamp := range []time.Time{time.Unix(1_700_000_000, 0), time.Unix(1_700_000_100, 0), time.Unix(1_700_000_200, 0)} {
		if err := store.RecordAudit(ctx, domain.AuditEvent{
			OccurredAt: timestamp, Actor: "admin", Role: domain.RoleAdmin, Action: "vm.start", TargetKind: "vm", TargetID: "vm-a", Result: "success",
		}); err != nil {
			t.Fatalf("RecordAudit() error = %v", err)
		}
	}
	deleted, err := store.DeleteAuditEventsBefore(ctx, time.Unix(1_700_000_150, 0))
	if err != nil || deleted != 2 {
		t.Fatalf("DeleteAuditEventsBefore() = %d, %v", deleted, err)
	}
	remaining, err := store.ListAuditEvents(ctx, domain.AuditEventQuery{Page: 1, PageSize: 20})
	if err != nil || remaining.Total != 1 || !remaining.Items[0].OccurredAt.Equal(time.Unix(1_700_000_200, 0)) {
		t.Fatalf("remaining events = %+v, %v", remaining, err)
	}
}
