package sqlite

import (
	"context"
	"testing"
)

func TestSettingsSetGet(t *testing.T) {
	store := openTestDB(t)
	ctx := context.Background()
	if value, found, err := store.GetSetting(ctx, "read_only"); err != nil || found || value != "" {
		t.Fatalf("missing setting = %q %v %v", value, found, err)
	}
	if err := store.SetSetting(ctx, "read_only", "true"); err != nil {
		t.Fatalf("SetSetting() error = %v", err)
	}
	if value, found, err := store.GetSetting(ctx, "read_only"); err != nil || !found || value != "true" {
		t.Fatalf("stored setting = %q %v %v", value, found, err)
	}
	if err := store.SetSetting(ctx, "read_only", "false"); err != nil {
		t.Fatalf("update setting error = %v", err)
	}
	if value, found, err := store.GetSetting(ctx, "read_only"); err != nil || !found || value != "false" {
		t.Fatalf("updated setting = %q %v %v", value, found, err)
	}
}
