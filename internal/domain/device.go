package domain

type HostDevice struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Parent       string   `json:"parent,omitempty"`
	Vendor       string   `json:"vendor,omitempty"`
	VendorID     string   `json:"vendorId,omitempty"`
	Product      string   `json:"product,omitempty"`
	ProductID    string   `json:"productId,omitempty"`
	Address      string   `json:"address,omitempty"`
	Driver       string   `json:"driver,omitempty"`
	Active       bool     `json:"active"`
	Persistent   bool     `json:"persistent"`
	Capabilities []string `json:"capabilities"`
}

type HostDeviceList struct {
	Items []HostDevice `json:"items"`
	Total int          `json:"total"`
}
