package domain

import "time"

type NetworkAddress struct {
	Family      string            `json:"family"`
	Address     string            `json:"address"`
	Prefix      int               `json:"prefix"`
	IPv6Mode    string            `json:"ipv6Mode,omitempty"`
	DHCPEnabled bool              `json:"dhcpEnabled,omitempty"`
	DHCPStart   string            `json:"dhcpStart,omitempty"`
	DHCPEnd     string            `json:"dhcpEnd,omitempty"`
	Hosts       []NetworkDHCPHost `json:"hosts,omitempty"`
}

type NetworkDHCPHost struct {
	MAC  string `json:"mac,omitempty"`
	ID   string `json:"id,omitempty"`
	IP   string `json:"ip"`
	Name string `json:"name,omitempty"`
}

type NetworkDHCPHostCreateSpec struct {
	MAC  string `json:"mac,omitempty"`
	ID   string `json:"id,omitempty"`
	IP   string `json:"ip"`
	Name string `json:"name,omitempty"`
}

type NetworkDHCPHostImportSpec struct {
	Hosts []NetworkDHCPHostCreateSpec `json:"hosts"`
}

type NetworkDHCPHostImportResult struct {
	Network  VirtualNetwork `json:"network"`
	Imported int            `json:"imported"`
	Skipped  int            `json:"skipped"`
}

type VirtualNetwork struct {
	Name        string           `json:"name"`
	UUID        string           `json:"uuid,omitempty"`
	Bridge      string           `json:"bridge,omitempty"`
	Active      bool             `json:"active"`
	Persistent  bool             `json:"persistent"`
	Autostart   bool             `json:"autostart"`
	ForwardMode string           `json:"forwardMode,omitempty"`
	NATIPv6     bool             `json:"natIpv6,omitempty"`
	DNSDomain   string           `json:"dnsDomain,omitempty"`
	DNSServers  []string         `json:"dnsServers,omitempty"`
	Addresses   []NetworkAddress `json:"addresses"`
	Warnings    []string         `json:"warnings,omitempty"`
}

type VirtualNetworkList struct {
	Items []VirtualNetwork `json:"items"`
	Total int              `json:"total"`
}

type HostBridge struct {
	Name string `json:"name"`
}

type HostBridgeList struct {
	Items []HostBridge `json:"items"`
	Total int          `json:"total"`
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

type NetworkCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type NetworkDiagnostics struct {
	Network           string         `json:"network"`
	CheckedAt         time.Time      `json:"checkedAt"`
	Active            bool           `json:"active"`
	Bridge            string         `json:"bridge,omitempty"`
	BridgeReady       bool           `json:"bridgeReady"`
	IPv4Forward       bool           `json:"ipv4Forwarding"`
	IPv6Forward       bool           `json:"ipv6Forwarding"`
	IPv6Configured    bool           `json:"ipv6Configured"`
	NATIPv6           bool           `json:"natIpv6"`
	NAT66Firewall     bool           `json:"nat66Firewall"`
	FirewallBackend   string         `json:"firewallBackend,omitempty"`
	DHCPv6Range       bool           `json:"dhcpv6Range"`
	DHCPv6Service     bool           `json:"dhcpv6Service"`
	RAService         bool           `json:"raService"`
	DNSService        bool           `json:"dnsService"`
	DNSResolution     bool           `json:"dnsResolution"`
	GatewayReachable  bool           `json:"gatewayReachable"`
	OutboundReachable bool           `json:"outboundReachable"`
	RuntimeDrift      bool           `json:"runtimeDrift"`
	DriftDetails      []string       `json:"driftDetails,omitempty"`
	LibvirtVersion    string         `json:"libvirtVersion,omitempty"`
	DnsmasqVersion    string         `json:"dnsmasqVersion,omitempty"`
	LeaseCount        int            `json:"leaseCount"`
	Checks            []NetworkCheck `json:"checks"`
}

type NetworkLeaseReservationSpec struct {
	IP   string `json:"ip"`
	Name string `json:"name,omitempty"`
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
	Name          string   `json:"name"`
	Mode          string   `json:"mode"`
	Subnet        string   `json:"subnet"`
	DHCPStart     string   `json:"dhcpStart,omitempty"`
	DHCPEnd       string   `json:"dhcpEnd,omitempty"`
	IPv6Subnet    string   `json:"ipv6Subnet,omitempty"`
	IPv6Mode      string   `json:"ipv6Mode,omitempty"`
	IPv6DHCPStart string   `json:"ipv6DhcpStart,omitempty"`
	IPv6DHCPEnd   string   `json:"ipv6DhcpEnd,omitempty"`
	NATIPv6       *bool    `json:"natIpv6,omitempty"`
	DNSDomain     string   `json:"dnsDomain,omitempty"`
	DNSServers    []string `json:"dnsServers,omitempty"`
	Bridge        string   `json:"bridge,omitempty"`
	Autostart     bool     `json:"autostart"`
	Start         bool     `json:"start"`
}

type VirtualNetworkUpdateSpec struct {
	Mode          string   `json:"mode"`
	Subnet        string   `json:"subnet"`
	DHCPStart     string   `json:"dhcpStart,omitempty"`
	DHCPEnd       string   `json:"dhcpEnd,omitempty"`
	IPv6Subnet    string   `json:"ipv6Subnet,omitempty"`
	IPv6Mode      string   `json:"ipv6Mode,omitempty"`
	IPv6DHCPStart string   `json:"ipv6DhcpStart,omitempty"`
	IPv6DHCPEnd   string   `json:"ipv6DhcpEnd,omitempty"`
	NATIPv6       *bool    `json:"natIpv6,omitempty"`
	DNSDomain     string   `json:"dnsDomain,omitempty"`
	DNSServers    []string `json:"dnsServers,omitempty"`
	Bridge        string   `json:"bridge,omitempty"`
	Autostart     bool     `json:"autostart"`
}

type VirtualNetworkDefinition struct {
	Name          string
	Mode          string
	IPv4Address   string
	Prefix        int
	DHCPStart     string
	DHCPEnd       string
	IPv6Address   string
	IPv6Prefix    int
	IPv6Mode      string
	IPv6DHCPStart string
	IPv6DHCPEnd   string
	NATIPv6       bool
	DNSDomain     string
	DNSServers    []string
	Bridge        string
	Autostart     bool
	Start         bool
}

func CanManageNetworks(role Role) bool {
	return role == RoleOperator || role == RoleAdmin
}
