package eventbus

import (
	"sync"
	"time"

	"virtmgr/internal/domain"
)

const subscriberBuffer = 64

type Bus struct {
	mu          sync.Mutex
	now         func() time.Time
	nextID      uint64
	max         int
	items       []domain.SystemEvent
	subscribers map[uint64]chan domain.SystemEvent
	nextSubID   uint64
}

func New(max int) *Bus {
	if max < 1 {
		max = 200
	}
	return &Bus{now: time.Now, max: max, subscribers: make(map[uint64]chan domain.SystemEvent)}
}

func (b *Bus) Publish(event domain.SystemEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	event.ID = b.nextID
	if event.OccurredAt.IsZero() {
		event.OccurredAt = b.now().UTC()
	}
	b.items = append(b.items, event)
	if len(b.items) > b.max {
		copy(b.items, b.items[len(b.items)-b.max:])
		b.items = b.items[:b.max]
	}
	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (b *Bus) Recent(limit int) []domain.SystemEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit < 1 || limit > len(b.items) {
		limit = len(b.items)
	}
	result := make([]domain.SystemEvent, 0, limit)
	for index := len(b.items) - 1; index >= len(b.items)-limit; index-- {
		result = append(result, b.items[index])
	}
	return result
}

func (b *Bus) Subscribe(limit int) ([]domain.SystemEvent, <-chan domain.SystemEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	initial := make([]domain.SystemEvent, 0)
	if limit < 1 || limit > len(b.items) {
		limit = len(b.items)
	}
	for index := len(b.items) - 1; index >= len(b.items)-limit; index-- {
		initial = append(initial, b.items[index])
	}
	b.nextSubID++
	id := b.nextSubID
	channel := make(chan domain.SystemEvent, subscriberBuffer)
	b.subscribers[id] = channel
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subscriber, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(subscriber)
		}
	}
	return initial, channel, cancel
}
