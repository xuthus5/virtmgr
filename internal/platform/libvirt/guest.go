package libvirtbackend

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"virtmgr/internal/domain"

	libvirt "libvirt.org/go/libvirt"
)

const guestAgentTimeoutSeconds = 1

const (
	guestListTimeout     = 2 * time.Second
	guestListConcurrency = 4
)

type guestHostnameResponse struct {
	Return struct {
		Hostname string `json:"host-name"`
	} `json:"return"`
}

type guestNetworkResponse struct {
	Return []guestNetworkInterface `json:"return"`
}

type guestNetworkInterface struct {
	Name        string `json:"name"`
	MAC         string `json:"hardware-address"`
	IPAddresses []struct {
		Address string `json:"ip-address"`
		Type    string `json:"ip-address-type"`
		Prefix  int    `json:"prefix"`
	} `json:"ip-addresses"`
}

func populateGuestInfo(dom *libvirt.Domain, vm *domain.VM) {
	if vm.State != domain.VMStateRunning && vm.State != domain.VMStateBlocked {
		return
	}
	rawHostname, err := dom.QemuAgentCommand(`{"execute":"guest-get-host-name"}`, libvirt.DomainQemuAgentCommandTimeout(guestAgentTimeoutSeconds), 0)
	if err != nil {
		return
	}
	hostname, err := parseGuestHostname(rawHostname)
	if err != nil {
		return
	}
	vm.AgentConnected = true
	vm.Hostname = hostname
	rawInterfaces, err := dom.QemuAgentCommand(`{"execute":"guest-network-get-interfaces"}`, libvirt.DomainQemuAgentCommandTimeout(guestAgentTimeoutSeconds), 0)
	if err != nil {
		return
	}
	addresses, err := parseGuestIPAddresses(rawInterfaces)
	if err != nil {
		return
	}
	vm.IPAddresses = addresses
}

func (c *Client) populateGuestInfoBatch(ctx context.Context, vms []domain.VM) {
	indexes := guestInfoCandidateIndexes(vms)
	if len(indexes) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, guestListTimeout)
	defer cancel()
	c.guestMu.Lock()
	defer c.guestMu.Unlock()
	if err := c.ensureGuestConnectionLocked(); err != nil {
		return
	}
	semaphore := make(chan struct{}, guestListConcurrency)
	var workers sync.WaitGroup
	for _, index := range indexes {
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			workers.Wait()
			return
		}
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			defer func() { <-semaphore }()
			if ctx.Err() != nil {
				return
			}
			dom, err := c.guestConn.LookupDomainByUUIDString(vms[index].UUID)
			if err != nil {
				return
			}
			defer func() { _ = dom.Free() }()
			populateGuestInfo(dom, &vms[index])
		}(index)
	}
	workers.Wait()
}

func (c *Client) ensureGuestConnectionLocked() error {
	if c.guestConn != nil {
		alive, err := c.guestConn.IsAlive()
		if err == nil && alive {
			return nil
		}
		_, _ = c.guestConn.Close()
		c.guestConn = nil
	}
	conn, err := libvirt.NewConnect(c.uri)
	if err != nil {
		return fmt.Errorf("connect guest agent libvirt %s: %w", c.uri, err)
	}
	c.guestConn = conn
	return nil
}

func guestInfoCandidateIndexes(vms []domain.VM) []int {
	result := make([]int, 0)
	for index := range vms {
		if vms[index].State == domain.VMStateRunning || vms[index].State == domain.VMStateBlocked {
			result = append(result, index)
		}
	}
	return result
}

func parseGuestHostname(raw string) (string, error) {
	var response guestHostnameResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return "", fmt.Errorf("decode guest hostname response: %w", err)
	}
	return strings.TrimSpace(response.Return.Hostname), nil
}

func parseGuestIPAddresses(raw string) ([]string, error) {
	var response guestNetworkResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return nil, fmt.Errorf("decode guest network response: %w", err)
	}
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, networkInterface := range response.Return {
		for _, address := range networkInterface.IPAddresses {
			value := strings.TrimSpace(address.Address)
			if zone := strings.LastIndexByte(value, '%'); zone >= 0 {
				value = value[:zone]
			}
			ip := net.ParseIP(value)
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || isIPv6LinkLocal(ip) {
				continue
			}
			normalized := ip.String()
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := net.ParseIP(result[i])
		right := net.ParseIP(result[j])
		leftV4 := left != nil && left.To4() != nil
		rightV4 := right != nil && right.To4() != nil
		if leftV4 != rightV4 {
			return leftV4
		}
		return result[i] < result[j]
	})
	return result, nil
}

func isIPv6LinkLocal(ip net.IP) bool {
	return ip.To4() == nil && len(ip) == net.IPv6len && ip[0] == 0xfe && ip[1]&0xc0 == 0x80
}
