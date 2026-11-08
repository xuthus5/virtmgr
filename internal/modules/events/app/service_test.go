package app

import (
	"testing"

	"virtmgr/internal/domain"
)

type fakeRepository struct {
	events    []domain.SystemEvent
	lastLimit int
}

func (f *fakeRepository) Recent(limit int) []domain.SystemEvent {
	f.lastLimit = limit
	if limit > len(f.events) {
		limit = len(f.events)
	}
	return f.events[:limit]
}

func (f *fakeRepository) Subscribe(limit int) ([]domain.SystemEvent, <-chan domain.SystemEvent, func()) {
	f.lastLimit = limit
	channel := make(chan domain.SystemEvent)
	return f.events, channel, func() { close(channel) }
}

func TestListNormalizesLimit(t *testing.T) {
	repository := &fakeRepository{events: []domain.SystemEvent{{ID: 2}, {ID: 1}}}
	service := New(repository)
	list := service.List(0)
	if repository.lastLimit != DefaultLimit || list.Total != 2 {
		t.Fatalf("limit=%d list=%+v", repository.lastLimit, list)
	}
	_ = service.List(1000)
	if repository.lastLimit != MaxLimit {
		t.Fatalf("max limit = %d", repository.lastLimit)
	}
}

func TestStreamUsesNormalizedLimit(t *testing.T) {
	repository := &fakeRepository{events: []domain.SystemEvent{{ID: 1}}}
	service := New(repository)
	initial, channel, cancel := service.Stream(10)
	defer cancel()
	if len(initial) != 1 || repository.lastLimit != 10 || channel == nil {
		t.Fatalf("initial=%+v limit=%d", initial, repository.lastLimit)
	}
}
