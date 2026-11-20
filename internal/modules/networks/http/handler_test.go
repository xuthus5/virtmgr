package networkshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"virtmgr/internal/domain"
	networksapp "virtmgr/internal/modules/networks/app"
	"virtmgr/internal/platform/authctx"

	"github.com/go-chi/chi/v5"
)

type handlerRepository struct {
	networks       []domain.VirtualNetwork
	bridges        []domain.HostBridge
	err            error
	created        domain.VirtualNetworkDefinition
	updated        domain.VirtualNetworkDefinition
	actionFor      string
	action         domain.VirtualNetworkAction
	addedHost      domain.NetworkDHCPHost
	deletedHost    domain.NetworkDHCPHost
	addedDNSHost   domain.NetworkDNSHost
	deletedDNSHost domain.NetworkDNSHost
	deleted        string
	leaseFor       string
	leases         []domain.DHCPLease
	diagnostics    domain.NetworkDiagnostics
}

func (f *handlerRepository) ListNetworks(context.Context) ([]domain.VirtualNetwork, error) {
	return f.networks, f.err
}

func (f *handlerRepository) ListHostBridges(context.Context) ([]domain.HostBridge, error) {
	return f.bridges, f.err
}

func (f *handlerRepository) ListDHCPLeases(_ context.Context, networkName string) ([]domain.DHCPLease, error) {
	f.leaseFor = networkName
	return f.leases, f.err
}

func (f *handlerRepository) NetworkDiagnostics(_ context.Context, networkName string) (domain.NetworkDiagnostics, error) {
	result := f.diagnostics
	if result.Network == "" {
		result.Network = networkName
	}
	return result, f.err
}

func (f *handlerRepository) CreateNetwork(_ context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	f.created = definition
	return domain.VirtualNetwork{Name: definition.Name, Active: definition.Start, ForwardMode: definition.Mode}, nil
}

func (f *handlerRepository) UpdateNetwork(_ context.Context, definition domain.VirtualNetworkDefinition) (domain.VirtualNetwork, error) {
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	f.updated = definition
	return domain.VirtualNetwork{Name: definition.Name, ForwardMode: definition.Mode, Autostart: definition.Autostart}, nil
}

func (f *handlerRepository) AddNetworkDHCPHost(_ context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	f.addedHost = host
	return domain.VirtualNetwork{Name: name}, nil
}

func (f *handlerRepository) DeleteNetworkDHCPHost(_ context.Context, name string, host domain.NetworkDHCPHost) (domain.VirtualNetwork, error) {
	if f.err != nil {
		return domain.VirtualNetwork{}, f.err
	}
	f.deletedHost = host
	return domain.VirtualNetwork{Name: name}, nil
}

func (f *handlerRepository) AddNetworkDNSHost(_ context.Context, name string, host domain.NetworkDNSHost) (domain.VirtualNetwork, error) {
	f.addedDNSHost = host
	return domain.VirtualNetwork{Name: name, DNSHosts: []domain.NetworkDNSHost{host}}, nil
}

func (f *handlerRepository) DeleteNetworkDNSHost(_ context.Context, name string, host domain.NetworkDNSHost) (domain.VirtualNetwork, error) {
	f.deletedDNSHost = host
	return domain.VirtualNetwork{Name: name}, nil
}

func (f *handlerRepository) PerformNetworkAction(_ context.Context, name string, action domain.VirtualNetworkAction) error {
	f.actionFor = name
	f.action = action
	return f.err
}

func (f *handlerRepository) DeleteNetwork(_ context.Context, name string) error {
	f.deleted = name
	return f.err
}

func allowAll(next http.Handler) http.Handler { return next }

func withSession(request *http.Request, role domain.Role) *http.Request {
	return request.WithContext(authctx.WithSession(request.Context(), domain.Session{
		User: domain.User{ID: 1, Username: "admin", Role: role},
	}))
}

func newTestRouter(repository *handlerRepository) *chi.Mux {
	router := chi.NewRouter()
	New(networksapp.New(repository, nil, false)).Mount(router, allowAll)
	return router
}

func TestNetworkRoutes(t *testing.T) {
	router := newTestRouter(&handlerRepository{networks: []domain.VirtualNetwork{{Name: "default"}}})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNetworkHostBridgeRoute(t *testing.T) {
	router := newTestRouter(&handlerRepository{bridges: []domain.HostBridge{{Name: "br1"}, {Name: "br0"}}})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks/bridges", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"name":"br0"`) || !strings.Contains(recorder.Body.String(), `"total":2`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestNetworkLeaseRoute(t *testing.T) {
	repository := &handlerRepository{leases: []domain.DHCPLease{{
		Family: "ipv4", MAC: "52:54:00:12:34:56", IPAddress: "192.168.122.101", Prefix: 24, Hostname: "demo-web-01",
	}}}
	router := newTestRouter(repository)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks/default/leases", nil))
	if recorder.Code != http.StatusOK || repository.leaseFor != "default" {
		t.Fatalf("status = %d leaseFor=%q body=%s", recorder.Code, repository.leaseFor, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"hostname":"demo-web-01"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}

	router = newTestRouter(&handlerRepository{err: domain.ErrNotFound})
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks/missing/leases", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing network status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	router = newTestRouter(&handlerRepository{})
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks/default/leases", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"items":[]`) {
		t.Fatalf("empty network leases status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReserveNetworkLeaseRoute(t *testing.T) {
	repository := &handlerRepository{
		networks: []domain.VirtualNetwork{{
			Name: "dual-net", Active: true, Persistent: true,
			Addresses: []domain.NetworkAddress{{Family: "ipv6", Address: "fd00:92::1", Prefix: 64, IPv6Mode: "dhcpv6", DHCPEnabled: true, DHCPStart: "fd00:92::2", DHCPEnd: "fd00:92::ffff"}},
		}},
		leases: []domain.DHCPLease{{Family: "ipv6", IPAddress: "fd00:92::50", Prefix: 64, ClientID: "00:03:00:01:52:54:00:12:34:56", Hostname: "db-v6"}},
	}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodPost, "/networks/dual-net/leases/reserve", strings.NewReader(`{"ip":"fd00:92::50"}`)), domain.RoleAdmin)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.addedHost.ID != "00:03:00:01:52:54:00:12:34:56" || repository.addedHost.IP != "fd00:92::50" || repository.addedHost.Name != "db-v6" {
		t.Fatalf("reserved host = %+v", repository.addedHost)
	}
}

func TestNetworkDiagnosticsRoute(t *testing.T) {
	repository := &handlerRepository{diagnostics: domain.NetworkDiagnostics{
		Active: true, Bridge: "virbr0", BridgeReady: true, IPv4Forward: true, IPv6Forward: true, NATIPv6: true,
		DHCPv6Range: true, LeaseCount: 2,
		Checks: []domain.NetworkCheck{{ID: "active", Status: "ok", Title: "网络状态", Detail: "正常"}},
	}}
	router := newTestRouter(repository)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks/default/diagnostics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, fragment := range []string{`"network":"default"`, `"bridge":"virbr0"`, `"ipv6Forwarding":true`, `"natIpv6":true`, `"dhcpv6Range":true`, `"leaseCount":2`, `"checks":[`} {
		if !strings.Contains(recorder.Body.String(), fragment) {
			t.Fatalf("body missing %q: %s", fragment, recorder.Body.String())
		}
	}
}

func TestCreateNetworkRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodPost, "/networks", strings.NewReader(`{
		"name":"lab-net",
		"mode":"nat",
		"subnet":"10.30.0.0/24",
		"autostart":true,
		"start":false
	}`)), domain.RoleOperator)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.created.Name != "lab-net" || repository.created.IPv4Address != "10.30.0.1" || !repository.created.Autostart {
		t.Fatalf("created definition = %+v", repository.created)
	}
}

func TestCreateBridgeNetworkRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodPost, "/networks", strings.NewReader(`{
		"name":"prod-bridge",
		"mode":"bridge",
		"bridge":"br0",
		"autostart":true,
		"start":false
	}`)), domain.RoleOperator)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.created.Mode != "bridge" || repository.created.Bridge != "br0" || repository.created.IPv4Address != "" {
		t.Fatalf("created bridge definition = %+v", repository.created)
	}
}

func TestUpdateNetworkRoute(t *testing.T) {
	repository := &handlerRepository{networks: []domain.VirtualNetwork{{Name: "lab-net"}}}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodPatch, "/networks/lab-net", strings.NewReader(`{
		"mode":"isolated",
		"subnet":"10.31.0.0/24",
		"dhcpStart":"10.31.0.20",
		"dhcpEnd":"10.31.0.120",
		"ipv6Subnet":"fd00:31::/64",
		"ipv6DhcpStart":"fd00:31::20",
		"ipv6DhcpEnd":"fd00:31::120",
		"autostart":true
	}`)), domain.RoleOperator)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.updated.Name != "lab-net" || repository.updated.IPv4Address != "10.31.0.1" || repository.updated.Mode != "isolated" || !repository.updated.Autostart ||
		repository.updated.IPv6Address != "fd00:31::1" || repository.updated.IPv6Prefix != 64 || repository.updated.IPv6DHCPStart != "fd00:31::20" {
		t.Fatalf("updated definition = %+v", repository.updated)
	}
}

func TestNetworkActionRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodPost, "/networks/lab-net/actions/start", nil), domain.RoleAdmin)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.actionFor != "lab-net" || repository.action != domain.VirtualNetworkActionStart {
		t.Fatalf("action = %q %q", repository.actionFor, repository.action)
	}

	request = withSession(httptest.NewRequest(http.MethodPost, "/networks/lab-net/actions/invalid", nil), domain.RoleAdmin)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid action status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNetworkDHCPHostRoutes(t *testing.T) {
	repository := &handlerRepository{networks: []domain.VirtualNetwork{{
		Name: "default", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
			DHCPStart: "192.168.122.2", DHCPEnd: "192.168.122.254",
			Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "web-01"}},
		}},
	}}}
	router := newTestRouter(repository)

	request := withSession(httptest.NewRequest(http.MethodPost, "/networks/default/hosts", strings.NewReader(`{
		"mac":"52:54:00:AB:CD:EF",
		"ip":"192.168.122.60",
		"name":"db-01"
	}`)), domain.RoleOperator)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("add host status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.addedHost.MAC != "52:54:00:ab:cd:ef" || repository.addedHost.IP != "192.168.122.60" || repository.addedHost.Name != "db-01" {
		t.Fatalf("added host = %+v", repository.addedHost)
	}

	request = withSession(httptest.NewRequest(http.MethodDelete, "/networks/default/hosts/52%3A54%3A00%3A12%3A34%3A56", nil), domain.RoleAdmin)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete host status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.deletedHost.MAC != "52:54:00:12:34:56" || repository.deletedHost.IP != "192.168.122.50" {
		t.Fatalf("deleted host = %+v", repository.deletedHost)
	}
}

func TestNetworkDHCPHostImportExportRoutes(t *testing.T) {
	repository := &handlerRepository{networks: []domain.VirtualNetwork{{
		Name: "default", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv4", Address: "192.168.122.1", Prefix: 24, DHCPEnabled: true,
			DHCPStart: "192.168.122.2", DHCPEnd: "192.168.122.254",
			Hosts: []domain.NetworkDHCPHost{{MAC: "52:54:00:12:34:56", IP: "192.168.122.50", Name: "web-01"}},
		}},
	}}}
	router := newTestRouter(repository)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks/default/hosts/export", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Content-Disposition"), "virtmgr-default-dhcp-hosts.csv") {
		t.Fatalf("export status = %d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.HasPrefix(body, "mac,id,ip,name\n") || !strings.Contains(body, "52:54:00:12:34:56,,192.168.122.50,web-01") {
		t.Fatalf("export body = %s", body)
	}

	request := withSession(httptest.NewRequest(http.MethodPost, "/networks/default/hosts/import", strings.NewReader("mac,id,ip,name\n52:54:00:12:34:56,,192.168.122.50,web-01\n52:54:00:AB:CD:EF,,192.168.122.60,db-01\n")), domain.RoleOperator)
	request.Header.Set("Content-Type", "text/csv")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"imported":1`) || !strings.Contains(recorder.Body.String(), `"skipped":1`) {
		t.Fatalf("import status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.addedHost.MAC != "52:54:00:ab:cd:ef" || repository.addedHost.IP != "192.168.122.60" {
		t.Fatalf("imported host = %+v", repository.addedHost)
	}

	request = httptest.NewRequest(http.MethodPost, "/networks/default/hosts/import", strings.NewReader("mac,id,ip,name\n52:54:00:AB:CD:EF,,192.168.122.60,db-01\n"))
	request.Header.Set("Content-Type", "text/csv")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNetworkDHCPHostIPv6DUIDRoutes(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dual-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{
			Family: "ipv6", Address: "fd00:86::1", Prefix: 64, IPv6Mode: "dhcpv6", DHCPEnabled: true,
			DHCPStart: "fd00:86::2", DHCPEnd: "fd00:86::ffff",
			Hosts: []domain.NetworkDHCPHost{{ID: "00:03:00:01:52:54:00:12:34:56", IP: "fd00:86::50", Name: "db-ipv6"}},
		}},
	}
	repository := &handlerRepository{networks: []domain.VirtualNetwork{network}}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodPost, "/networks/dual-net/hosts", strings.NewReader(`{"id":"00:03:00:01:52:54:00:ab:cd:ef","ip":"fd00:86::60","name":"web-ipv6"}`)), domain.RoleOperator)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || repository.addedHost.ID != "00:03:00:01:52:54:00:ab:cd:ef" || repository.addedHost.MAC != "" {
		t.Fatalf("add IPv6 host status = %d body=%s host=%+v", recorder.Code, recorder.Body.String(), repository.addedHost)
	}

	request = withSession(httptest.NewRequest(http.MethodDelete, "/networks/dual-net/hosts/00%3A03%3A00%3A01%3A52%3A54%3A00%3A12%3A34%3A56", nil), domain.RoleAdmin)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || repository.deletedHost.ID != "00:03:00:01:52:54:00:12:34:56" || repository.deletedHost.IP != "fd00:86::50" {
		t.Fatalf("delete IPv6 host status = %d body=%s host=%+v", recorder.Code, recorder.Body.String(), repository.deletedHost)
	}
}

func TestNetworkDNSHostIPv6Routes(t *testing.T) {
	network := domain.VirtualNetwork{
		Name: "dns-net", Active: true, Persistent: true,
		Addresses: []domain.NetworkAddress{{Family: "ipv6", Address: "fd00:87::1", Prefix: 64}},
		DNSHosts:  []domain.NetworkDNSHost{{IP: "fd00:87::50", Hostnames: []string{"db.lab.example"}}},
	}
	repository := &handlerRepository{networks: []domain.VirtualNetwork{network}}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodPost, "/networks/dns-net/dns/hosts", strings.NewReader(`{"ip":"fd00:87::51","hostnames":["Web.Lab.Example.","web.lab.example"]}`)), domain.RoleOperator)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || repository.addedDNSHost.IP != "fd00:87::51" || len(repository.addedDNSHost.Hostnames) != 1 || repository.addedDNSHost.Hostnames[0] != "web.lab.example" {
		t.Fatalf("add DNS host status = %d body=%s host=%+v", recorder.Code, recorder.Body.String(), repository.addedDNSHost)
	}

	request = withSession(httptest.NewRequest(http.MethodDelete, "/networks/dns-net/dns/hosts/fd00%3A87%3A%3A50", nil), domain.RoleAdmin)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || repository.deletedDNSHost.IP != "fd00:87::50" {
		t.Fatalf("delete DNS host status = %d body=%s host=%+v", recorder.Code, recorder.Body.String(), repository.deletedDNSHost)
	}
}

func TestDeleteNetworkRoute(t *testing.T) {
	repository := &handlerRepository{}
	router := newTestRouter(repository)
	request := withSession(httptest.NewRequest(http.MethodDelete, "/networks/lab-net", nil), domain.RoleAdmin)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.deleted != "lab-net" {
		t.Fatalf("deleted = %q", repository.deleted)
	}
}

func TestNetworkMutationsRequireSessionAndOperator(t *testing.T) {
	router := newTestRouter(&handlerRepository{})
	request := httptest.NewRequest(http.MethodPost, "/networks/lab-net/actions/start", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing session status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	request = withSession(httptest.NewRequest(http.MethodDelete, "/networks/lab-net", nil), domain.RoleViewer)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNetworkWriteRoutesUseCSRF(t *testing.T) {
	repository := &handlerRepository{networks: []domain.VirtualNetwork{{Name: "lab-net"}}}
	router := chi.NewRouter()
	New(networksapp.New(repository, nil, false)).Mount(router, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-CSRF-Token") != "valid" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	request := withSession(httptest.NewRequest(http.MethodPost, "/networks/lab-net/actions/stop", nil), domain.RoleAdmin)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	request.Header.Set("X-CSRF-Token", "valid")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("valid csrf status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	request = withSession(httptest.NewRequest(http.MethodPatch, "/networks/lab-net", strings.NewReader(`{"mode":"nat","subnet":"10.0.0.0/24","autostart":false}`)), domain.RoleAdmin)
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("update missing csrf status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	request.Header.Set("X-CSRF-Token", "valid")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update valid csrf status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNetworkRouteError(t *testing.T) {
	router := newTestRouter(&handlerRepository{err: domain.ErrUnavailable})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/networks", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
