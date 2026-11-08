package runtimeconfig

import (
	"context"
	"errors"
	"testing"
)

type memoryStore struct {
	values   map[string]string
	getErr   error
	setErr   error
	setCalls int
}

func (s *memoryStore) GetSetting(_ context.Context, key string) (string, bool, error) {
	if s.getErr != nil {
		return "", false, s.getErr
	}
	value, found := s.values[key]
	return value, found, nil
}

func (s *memoryStore) SetSetting(_ context.Context, key, value string) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.setCalls++
	s.values[key] = value
	return nil
}

func TestModeSeedsAndPersistsSetting(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{values: map[string]string{}}
	mode, err := New(ctx, store, true)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !mode.ReadOnly() || store.values[ReadOnlySettingKey] != "true" {
		t.Fatalf("initial mode=%v values=%v", mode.ReadOnly(), store.values)
	}
	if err := mode.SetReadOnly(ctx, false); err != nil {
		t.Fatalf("SetReadOnly() error = %v", err)
	}
	if mode.ReadOnly() || store.values[ReadOnlySettingKey] != "false" {
		t.Fatalf("updated mode=%v values=%v", mode.ReadOnly(), store.values)
	}

	reloaded, err := New(ctx, store, true)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if reloaded.ReadOnly() {
		t.Fatal("persisted read-only setting was not restored")
	}
}

func TestModeErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, nil, true); err == nil {
		t.Fatal("New(nil store) error = nil")
	}
	store := &memoryStore{values: map[string]string{ReadOnlySettingKey: "not-a-bool"}}
	if _, err := New(ctx, store, true); err == nil {
		t.Fatal("invalid persisted value error = nil")
	}
	want := errors.New("disk full")
	store = &memoryStore{values: map[string]string{}, setErr: want}
	if _, err := New(ctx, store, true); !errors.Is(err, want) {
		t.Fatalf("seed error = %v", err)
	}
	store = &memoryStore{values: map[string]string{ReadOnlySettingKey: "false"}}
	mode, err := New(ctx, store, true)
	if err != nil {
		t.Fatal(err)
	}
	store.setErr = want
	if err := mode.SetReadOnly(ctx, true); !errors.Is(err, want) {
		t.Fatalf("SetReadOnly() error = %v", err)
	}
	if mode.ReadOnly() {
		t.Fatal("mode changed after persistence failure")
	}
}
