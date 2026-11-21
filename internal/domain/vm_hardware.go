package domain

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type VMWatchdogModel string

const (
	VMWatchdogModelI6300ESB VMWatchdogModel = "i6300esb"
	VMWatchdogModelIB700    VMWatchdogModel = "ib700"
)

type VMWatchdogAction string

const (
	VMWatchdogActionReset    VMWatchdogAction = "reset"
	VMWatchdogActionShutdown VMWatchdogAction = "shutdown"
	VMWatchdogActionPoweroff VMWatchdogAction = "poweroff"
	VMWatchdogActionPause    VMWatchdogAction = "pause"
	VMWatchdogActionDump     VMWatchdogAction = "dump"
)

type VMWatchdog struct {
	Model  VMWatchdogModel  `json:"model"`
	Action VMWatchdogAction `json:"action"`
}

type VMHardware struct {
	RNG      bool        `json:"rng"`
	Watchdog *VMWatchdog `json:"watchdog,omitempty"`
}

func NormalizeVMWatchdog(watchdog *VMWatchdog) (*VMWatchdog, error) {
	if watchdog == nil {
		return nil, nil
	}
	model := VMWatchdogModel(strings.ToLower(strings.TrimSpace(string(watchdog.Model))))
	switch model {
	case VMWatchdogModelI6300ESB, VMWatchdogModelIB700:
	default:
		return nil, fmt.Errorf("%w: watchdog model must be i6300esb or ib700", ErrInvalidInput)
	}
	action := VMWatchdogAction(strings.ToLower(strings.TrimSpace(string(watchdog.Action))))
	switch action {
	case VMWatchdogActionReset, VMWatchdogActionShutdown, VMWatchdogActionPoweroff, VMWatchdogActionPause, VMWatchdogActionDump:
	default:
		return nil, fmt.Errorf("%w: unsupported watchdog action", ErrInvalidInput)
	}
	return &VMWatchdog{Model: model, Action: action}, nil
}

func NormalizeVMHardware(hardware VMHardware) (VMHardware, error) {
	watchdog, err := NormalizeVMWatchdog(hardware.Watchdog)
	if err != nil {
		return VMHardware{}, err
	}
	return VMHardware{RNG: hardware.RNG, Watchdog: watchdog}, nil
}

func SetVMXMLHardware(text string, hardware VMHardware) (string, error) {
	normalized, err := NormalizeVMHardware(hardware)
	if err != nil {
		return "", err
	}
	root, err := parseVMXMLTree(text)
	if err != nil {
		return "", err
	}
	domainNode := firstVMXMLChild(root, "domain")
	if domainNode == nil {
		return "", fmt.Errorf("%w: domain XML has no domain element", ErrInvalidInput)
	}
	devicesNode := firstVMXMLChild(domainNode, "devices")
	if devicesNode == nil {
		return "", fmt.Errorf("%w: domain XML has no devices element", ErrInvalidInput)
	}
	devicesNode.Children = removeVMXMLChildren(devicesNode.Children, func(node *vmXMLNode) bool {
		name := vmXMLNodeName(node)
		return name == "rng" || name == "watchdog"
	})
	if normalized.RNG {
		devicesNode.Children = append(devicesNode.Children, &vmXMLNode{
			Token: xml.StartElement{Name: xml.Name{Local: "rng"}, Attr: []xml.Attr{{Name: xml.Name{Local: "model"}, Value: "virtio"}}},
			Children: []*vmXMLNode{{
				Token:    xml.StartElement{Name: xml.Name{Local: "backend"}, Attr: []xml.Attr{{Name: xml.Name{Local: "model"}, Value: "random"}}},
				Children: []*vmXMLNode{{Token: xml.CharData("/dev/urandom")}},
			}},
		})
	}
	if normalized.Watchdog != nil {
		devicesNode.Children = append(devicesNode.Children, &vmXMLNode{
			Token: xml.StartElement{Name: xml.Name{Local: "watchdog"}, Attr: []xml.Attr{
				{Name: xml.Name{Local: "model"}, Value: string(normalized.Watchdog.Model)},
				{Name: xml.Name{Local: "action"}, Value: string(normalized.Watchdog.Action)},
			}},
		})
	}
	return encodeVMXMLDocument(root)
}
