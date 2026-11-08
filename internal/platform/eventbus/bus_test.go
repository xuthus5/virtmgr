package eventbus

import (
	"testing"
	"time"

	"virtmgr/internal/domain"
)

func TestPublishRecentAndSubscriber(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	bus := New(2)
	bus.now = func() time.Time { return now }
	initial, channel, cancel := bus.Subscribe(10)
	defer cancel()
	if len(initial) != 0 {
		t.Fatalf("initial = %+v", initial)
	}
	bus.Publish(domain.SystemEvent{Kind: "one", Message: "first"})
	bus.Publish(domain.SystemEvent{Kind: "two", Message: "second"})
	bus.Publish(domain.SystemEvent{Kind: "three", Message: "third"})

	first := <-channel
	second := <-channel
	third := <-channel
	if first.ID != 1 || second.ID != 2 || third.ID != 3 || third.Kind != "three" {
		t.Fatalf("events = %+v %+v %+v", first, second, third)
	}
	if !third.OccurredAt.Equal(now) {
		t.Fatalf("occurredAt = %v", third.OccurredAt)
	}
	recent := bus.Recent(10)
	if len(recent) != 2 || recent[0].ID != 3 || recent[1].ID != 2 {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestSubscriberBufferDropsWithoutBlocking(t *testing.T) {
	bus := New(200)
	_, channel, cancel := bus.Subscribe(0)
	defer cancel()
	for index := 0; index < subscriberBuffer+5; index++ {
		bus.Publish(domain.SystemEvent{Kind: "test"})
	}
	if len(channel) != subscriberBuffer {
		t.Fatalf("channel length = %d", len(channel))
	}
}

func TestCancelSubscriber(t *testing.T) {
	bus := New(10)
	_, channel, cancel := bus.Subscribe(0)
	cancel()
	if _, ok := <-channel; ok {
		t.Fatal("subscriber channel should be closed")
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.subscribers) != 0 {
		t.Fatalf("subscribers = %+v", bus.subscribers)
	}
}
