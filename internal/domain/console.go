package domain

import (
	"strings"
	"time"
)

type ConsoleKind string

const (
	ConsoleKindVNC    ConsoleKind = "vnc"
	ConsoleKindSerial ConsoleKind = "serial"
)

func ValidConsoleKind(value string) (ConsoleKind, bool) {
	switch kind := ConsoleKind(strings.ToLower(strings.TrimSpace(value))); kind {
	case ConsoleKindVNC, ConsoleKindSerial:
		return kind, true
	default:
		return "", false
	}
}

type ConsoleToken struct {
	Token     string
	VMUUID    string
	UserID    int64
	Kind      ConsoleKind
	Device    string
	ExpiresAt time.Time
}

type ConsoleTicket struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expiresAt"`
	Path      string      `json:"path"`
	Kind      ConsoleKind `json:"kind"`
	Device    string      `json:"device,omitempty"`
}

type ConsoleKeyAction string

const (
	ConsoleKeyCtrlAltDel       ConsoleKeyAction = "ctrl-alt-del"
	ConsoleKeyCtrlAltBackspace ConsoleKeyAction = "ctrl-alt-backspace"
	ConsoleKeyCtrlAltF1        ConsoleKeyAction = "ctrl-alt-f1"
	ConsoleKeyCtrlAltF2        ConsoleKeyAction = "ctrl-alt-f2"
	ConsoleKeyCtrlAltF3        ConsoleKeyAction = "ctrl-alt-f3"
	ConsoleKeyCtrlAltF4        ConsoleKeyAction = "ctrl-alt-f4"
	ConsoleKeyCtrlAltF5        ConsoleKeyAction = "ctrl-alt-f5"
	ConsoleKeyCtrlAltF6        ConsoleKeyAction = "ctrl-alt-f6"
	ConsoleKeyCtrlAltF7        ConsoleKeyAction = "ctrl-alt-f7"
	ConsoleKeyCtrlAltF8        ConsoleKeyAction = "ctrl-alt-f8"
	ConsoleKeyCtrlAltF9        ConsoleKeyAction = "ctrl-alt-f9"
	ConsoleKeyCtrlAltF10       ConsoleKeyAction = "ctrl-alt-f10"
	ConsoleKeyCtrlAltF11       ConsoleKeyAction = "ctrl-alt-f11"
	ConsoleKeyCtrlAltF12       ConsoleKeyAction = "ctrl-alt-f12"
)

func ConsoleKeycodes(value string) ([]uint, bool) {
	const (
		keyCtrl      = 29
		keyAlt       = 56
		keyBackspace = 14
		keyDelete    = 111
		keyF1        = 59
	)
	keyCodes := []uint{keyCtrl, keyAlt}
	switch ConsoleKeyAction(strings.ToLower(strings.TrimSpace(value))) {
	case ConsoleKeyCtrlAltDel:
		return append(keyCodes, keyDelete), true
	case ConsoleKeyCtrlAltBackspace:
		return append(keyCodes, keyBackspace), true
	case ConsoleKeyCtrlAltF1:
		return append(keyCodes, keyF1), true
	case ConsoleKeyCtrlAltF2:
		return append(keyCodes, keyF1+1), true
	case ConsoleKeyCtrlAltF3:
		return append(keyCodes, keyF1+2), true
	case ConsoleKeyCtrlAltF4:
		return append(keyCodes, keyF1+3), true
	case ConsoleKeyCtrlAltF5:
		return append(keyCodes, keyF1+4), true
	case ConsoleKeyCtrlAltF6:
		return append(keyCodes, keyF1+5), true
	case ConsoleKeyCtrlAltF7:
		return append(keyCodes, keyF1+6), true
	case ConsoleKeyCtrlAltF8:
		return append(keyCodes, keyF1+7), true
	case ConsoleKeyCtrlAltF9:
		return append(keyCodes, keyF1+8), true
	case ConsoleKeyCtrlAltF10:
		return append(keyCodes, keyF1+9), true
	case ConsoleKeyCtrlAltF11:
		return append(keyCodes, 87), true
	case ConsoleKeyCtrlAltF12:
		return append(keyCodes, 88), true
	default:
		return nil, false
	}
}
