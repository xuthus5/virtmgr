package domain

import "testing"

func TestValidConsoleKind(t *testing.T) {
	for value, want := range map[string]ConsoleKind{
		"vnc":      ConsoleKindVNC,
		" SERIAL ": ConsoleKindSerial,
	} {
		kind, ok := ValidConsoleKind(value)
		if !ok || kind != want {
			t.Fatalf("ValidConsoleKind(%q) = %q, %v", value, kind, ok)
		}
	}
	if _, ok := ValidConsoleKind("spice"); ok {
		t.Fatal("ValidConsoleKind accepted spice")
	}
}

func TestConsoleKeycodes(t *testing.T) {
	tests := map[string][]uint{
		"ctrl-alt-del":       {29, 56, 111},
		"ctrl-alt-backspace": {29, 56, 14},
		"ctrl-alt-f1":        {29, 56, 59},
		"ctrl-alt-f10":       {29, 56, 68},
		"ctrl-alt-f11":       {29, 56, 87},
		"ctrl-alt-f12":       {29, 56, 88},
	}
	for value, want := range tests {
		got, ok := ConsoleKeycodes(value)
		if !ok || len(got) != len(want) {
			t.Fatalf("ConsoleKeycodes(%q) = %v, %v", value, got, ok)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("ConsoleKeycodes(%q) = %v, want %v", value, got, want)
			}
		}
	}
	if keys, ok := ConsoleKeycodes("power"); ok || keys != nil {
		t.Fatalf("ConsoleKeycodes(power) = %v, %v", keys, ok)
	}
}
