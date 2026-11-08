package domain

import "time"

type NetworkAddress struct {
	Family      string            `json:"family"`
	Address     string            `json:"address"`
	Prefix      int               `json:"prefix"`
	DHCPEnabled bool              `json:"dhcpEnabled,omitempty"`
	DHCPStart   string            `json:"dhcpStart,omitempty"`
	DHCPEnd     string            `json:"dhcpEnd,omitempty"`
	Hosts       []NetworkDHCPHost `json:"hosts,omitempty"`
}

type NetworkDHCPHost struct {
	MAC  string `json:"mac"`
	IP   string `json:"ip"`
	Name string `json:"name,omitempty"`
}

type NetworkDHCPHostCreateSpec struct {
	MAC  string `json:"mac"`
	IP   string `json:"ip"`
	Name string `json:"name,omitempty"`
}

type VirtualNetwork struct {
	Name        string           `json:"name"`
	UUID        string           `json:"uuid,omitempty"`
	Bridge      string           `json:"bridge,omitempty"`
	Active      bool             `json:"active"`
	Persistent  bool             `json:"persistent"`
	Autostart   bool             `json:"autostart"`
	ForwardMode string           `json:"forwardMode,omitempty"`
	Addresses   []NetworkAddress `json:"addresses"`
}

type VirtualNetworkList struct {
	Items []VirtualNetwork `json:"items"`
	Total int              `json:"total"`
}

type DHCPLease struct {
	Interface string     `json:"interface,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Family    string     `json:"family"`
	MAC       string     `json:"mac"`
	IPAddress string     `json:"ipAddress"`
	Prefix    uint       `json:"prefix"`
	Hostname  string     `json:"hostname,omitempty"`
	ClientID  string     `json:"clientId,omitempty"`
	IAID      string     `json:"iaid,omitempty"`
}

type DHCPLeaseList struct {
	Network string      `json:"network"`
	Items   []DHCPLease `json:"items"`
	Total   int         `json:"total"`
}

type VirtualNetworkAction string

const (
	VirtualNetworkActionStart VirtualNetworkAction = "start"
	VirtualNetworkActionStop  VirtualNetworkAction = "stop"
)

func ValidVirtualNetworkAction(action string) (VirtualNetworkAction, bool) {
	switch VirtualNetworkAction(action) {
	case VirtualNetworkActionStart, VirtualNetworkActionStop:
		return VirtualNetworkAction(action), true
	default:
		return "", false
	}
}

type VirtualNetworkCreateSpec struct {
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	Subnet    string `json:"subnet"`
	DHCPStart string `json:"dhcpStart,omitempty"`
	DHCPEnd   string `json:"dhcpEnd,omitempty"`
	Autostart bool   `json:"autostart"`
	Start     bool   `json:"start"`
}

type VirtualNetworkUpdateSpec struct {
	Mode      string `json:"mode"`
	Subnet    string `json:"subnet"`
	DHCPStart string `json:"dhcpStart,omitempty"`
	DHCPEnd   string `json:"dhcpEnd,omitempty"`
	Autostart bool   `json:"autostart"`
}

type VirtualNetworkDefinition struct {
	Name        string
	Mode        string
	IPv4Address string
	Prefix      int
	DHCPStart   string
	DHCPEnd     string
	Autostart   bool
	Start       bool
}

func CanManageNetworks(role Role) bool {
	return role == RoleOperator || role == RoleAdmin
}
