package libvirtbackend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"virtmgr/internal/domain"
)

func TestGuestAgentCapabilitiesFromGuestInfo(t *testing.T) {
	var parsed guestInfoResponse
	if err := decodeGuestAgentResponse(`{"return":{"version":"8.2.2","supported_commands":[
  {"name":"guest-get-host-name","enabled":true},
  {"name":"guest-network-get-interfaces","enabled":true},
  {"name":"guest-get-osinfo","enabled":true},
  {"name":"guest-get-fsinfo","enabled":false},
  {"name":"guest-shutdown","enabled":true},
  {"name":"guest-set-time","enabled":true},
  {"name":"guest-fstrim","enabled":true},
  {"name":"guest-fsfreeze-freeze","enabled":true},
  {"name":"guest-fsfreeze-thaw","enabled":true}
]}}`, &parsed); err != nil {
		t.Fatal(err)
	}
	capabilities := guestAgentCapabilities(parsed)
	if !capabilities.Hostname || !capabilities.Network || !capabilities.OSInfo || capabilities.FSInfo {
		t.Fatalf("basic capabilities = %+v", capabilities)
	}
	if !capabilities.Shutdown || !capabilities.Reboot || !capabilities.TimeSync || !capabilities.Trim || !capabilities.Quiesce {
		t.Fatalf("action capabilities = %+v", capabilities)
	}
}

func TestGuestAgentExtendedCapabilitiesAndParsers(t *testing.T) {
	var parsed guestInfoResponse
	if err := decodeGuestAgentResponse(`{"return":{"version":"10.2.2","supported_commands":[
  {"name":"guest-file-open","enabled":true},{"name":"guest-file-read","enabled":true},
  {"name":"guest-file-write","enabled":true},{"name":"guest-file-close","enabled":true},
  {"name":"guest-set-user-password","enabled":true},
  {"name":"guest-ssh-get-authorized-keys","enabled":true},
  {"name":"guest-ssh-add-authorized-keys","enabled":true},
  {"name":"guest-ssh-remove-authorized-keys","enabled":true},
  {"name":"guest-exec","enabled":true},{"name":"guest-exec-status","enabled":true},
  {"name":"guest-get-users","enabled":true},{"name":"guest-get-timezone","enabled":true},
  {"name":"guest-network-get-route","enabled":true}
]}}`, &parsed); err != nil {
		t.Fatal(err)
	}
	capabilities := guestAgentCapabilities(parsed)
	if !capabilities.FileAccess || !capabilities.PasswordReset || !capabilities.SSHKeys || !capabilities.Processes || !capabilities.Users || !capabilities.Timezone || !capabilities.NetworkRoutes {
		t.Fatalf("extended capabilities = %+v", capabilities)
	}
	timezone, err := parseGuestTimezone(`{"return":{"zone":"Asia/Shanghai","offset":28800}}`)
	if err != nil || timezone != "Asia/Shanghai" {
		t.Fatalf("parseGuestTimezone() = %q, %v", timezone, err)
	}
	routes, err := parseGuestRoutes(`{"return":[
  {"destination":"0.0.0.0/0","gateway":"192.168.122.1","interface":"eth0","family":"ipv4","metric":100},
  {"destination":"2001:db8::/64","interface":"eth0","family":"ipv6","metric":100}
]}`)
	if err != nil || len(routes) != 2 || routes[0].Family != "ipv4" || routes[1].Family != "ipv6" {
		t.Fatalf("parseGuestRoutes() = %+v, %v", routes, err)
	}
}

func TestGuestAgentOperationGuards(t *testing.T) {
	for _, path := range []string{"/etc/hosts", "/var/lib/app/config"} {
		if !validGuestAgentFilePath(path) {
			t.Fatalf("path rejected: %s", path)
		}
	}
	for _, path := range []string{"", "relative", "/tmp/a\x00b"} {
		if validGuestAgentFilePath(path) {
			t.Fatalf("path accepted: %q", path)
		}
	}
	processes := parseGuestProcesses("  1 0 root Ss systemd /sbin/init\n412 1 root Ssl sshd sshd: /usr/sbin/sshd -D\nbad line\n")
	if len(processes) != 2 || processes[0].PID != 1 || processes[1].Args != "sshd: /usr/sbin/sshd -D" {
		t.Fatalf("processes = %+v", processes)
	}
	services := parseGuestServices("nginx.service loaded active running A high performance web server\nbad line\nssh.service loaded failed failed OpenBSD Secure Shell server\n")
	if len(services) != 2 || services[0].Unit != "nginx.service" || services[1].Active != "failed" {
		t.Fatalf("services = %+v", services)
	}
	code, hint := guestAgentDiagnosticFromError(errors.New("permission denied by guest policy"))
	if code != "permission_denied" || hint == "" {
		t.Fatalf("diagnostic = %s %s", code, hint)
	}
}

func TestParseGuestAgentDetails(t *testing.T) {
	osInfo, err := parseGuestOSInfo(`{"return":{"id":"ubuntu","name":"Ubuntu","pretty-name":"Ubuntu 24.04 LTS","version":"24.04","kernel-release":"6.8.0","machine":"x86_64","hostname":"db-01"}}`)
	if err != nil || osInfo.PrettyName != "Ubuntu 24.04 LTS" || osInfo.Hostname != "db-01" {
		t.Fatalf("parseGuestOSInfo() = %+v, %v", osInfo, err)
	}

	interfaces, err := parseGuestAgentNetworkInterfaces(`{"return":[
  {"name":"lo","hardware-address":"00:00:00:00:00:00","ip-addresses":[{"ip-address":"127.0.0.1","ip-address-type":"ipv4","prefix":8}]},
  {"name":"eth0","hardware-address":"52:54:00:00:00:01","ip-addresses":[
    {"ip-address":"2001:db8::10","ip-address-type":"ipv6","prefix":64},
    {"ip-address":"10.0.0.10","ip-address-type":"ipv4","prefix":24},
    {"ip-address":"fe80::1%eth0","ip-address-type":"ipv6","prefix":64}
  ]}
]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(interfaces) != 1 || interfaces[0].Name != "eth0" || len(interfaces[0].Addresses) != 2 {
		t.Fatalf("interfaces = %+v", interfaces)
	}
	if interfaces[0].Addresses[0].Address != "10.0.0.10" || interfaces[0].Addresses[1].Address != "2001:db8::10" {
		t.Fatalf("address order = %+v", interfaces[0].Addresses)
	}

	filesystems, err := parseGuestAgentFilesystems(`{"return":[{"name":"/dev/vda1","mountpoint":"/","type":"ext4","total-bytes":100,"used-bytes":40}]}`)
	if err != nil || len(filesystems) != 1 || filesystems[0].FreeBytes != 60 {
		t.Fatalf("filesystems = %+v, %v", filesystems, err)
	}
}

func TestGuestAgentResponseError(t *testing.T) {
	var parsed guestInfoResponse
	err := decodeGuestAgentResponse(`{"error":{"class":"GenericError","desc":"agent command failed"}}`, &parsed)
	if err == nil || !strings.Contains(err.Error(), "agent command failed") {
		t.Fatalf("decode error = %v", err)
	}
}

func TestAddGuestAgentChannelXML(t *testing.T) {
	original := `<domain type='kvm'><name>web-01</name><devices><disk type='file' device='disk'/></devices></domain>`
	updated, err := addGuestAgentChannelXML(original)
	if err != nil {
		t.Fatal(err)
	}
	if !hasGuestAgentChannelXML(updated) {
		t.Fatalf("channel not detected: %s", updated)
	}
	for _, fragment := range []string{"<controller type='virtio-serial' index='0'/>", "name='org.qemu.guest_agent.0'"} {
		if !strings.Contains(updated, fragment) {
			t.Fatalf("updated XML missing %q: %s", fragment, updated)
		}
	}
	idempotent, err := addGuestAgentChannelXML(updated)
	if err != nil || idempotent != updated {
		t.Fatalf("addGuestAgentChannelXML() was not idempotent: %v", err)
	}
}

func TestAddGuestAgentChannelXMLReusesVirtioSerialController(t *testing.T) {
	original := `<domain type='kvm'><name>web-01</name><devices><controller type='virtio-serial' index='0'/><disk type='file' device='disk'/></devices></domain>`
	updated, err := addGuestAgentChannelXML(original)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(updated, "type='virtio-serial'") != 1 {
		t.Fatalf("controller was duplicated: %s", updated)
	}
}

func TestGuestAgentActionSpecValidation(t *testing.T) {
	valid := []domain.GuestAgentActionSpec{
		{Action: domain.GuestAgentActionShutdown},
		{Action: domain.GuestAgentActionTrim},
		{Action: domain.GuestAgentActionTrim, Mount: "/"},
		{Action: domain.GuestAgentActionTrim, Mount: "/var/lib", MinimumBytes: 4096},
	}
	for _, spec := range valid {
		if !validGuestAgentActionSpec(spec) {
			t.Fatalf("valid spec rejected: %+v", spec)
		}
	}
	invalid := []domain.GuestAgentActionSpec{
		{Action: domain.GuestAgentActionShutdown, Mount: "/"},
		{Action: domain.GuestAgentActionTrim, Mount: "var/lib"},
		{Action: domain.GuestAgentActionTrim, Mount: "/var/lib\x00"},
		{Action: "arbitrary"},
	}
	for _, spec := range invalid {
		if validGuestAgentActionSpec(spec) {
			t.Fatalf("invalid spec accepted: %+v", spec)
		}
	}
}

func TestBuildDomainXMLGuestAgentChannel(t *testing.T) {
	defaultXML := buildDomainXML(domain.VMCreateSpec{
		Name: "web-01", VCPUs: 2, MemoryMiB: 2048, DiskPath: "/var/lib/libvirt/images/web-01.qcow2",
		DiskFormat: "qcow2", Network: "default",
	})
	if !hasGuestAgentChannelXML(defaultXML) {
		t.Fatalf("default VM XML has no guest agent channel: %s", defaultXML)
	}
	disabled := false
	withoutAgent := buildDomainXML(domain.VMCreateSpec{
		Name: "web-02", VCPUs: 2, MemoryMiB: 2048, DiskPath: "/var/lib/libvirt/images/web-02.qcow2",
		DiskFormat: "qcow2", Network: "default", GuestAgent: &disabled,
	})
	if hasGuestAgentChannelXML(withoutAgent) {
		t.Fatalf("disabled VM XML has a guest agent channel: %s", withoutAgent)
	}
}

func TestDemoGuestAgentLifecycle(t *testing.T) {
	backend := NewDemo()
	info, err := backend.GetGuestAgentInfo(context.Background(), "11111111-1111-4111-8111-111111111111", false)
	if err != nil || info.State != domain.GuestAgentStateConnected || !info.Capabilities.Quiesce {
		t.Fatalf("connected info = %+v, %v", info, err)
	}
	legacy, err := backend.GetGuestAgentInfo(context.Background(), "44444444-4444-4444-8444-444444444444", false)
	if err != nil || legacy.State != domain.GuestAgentStateNotConfigured {
		t.Fatalf("legacy info = %+v, %v", legacy, err)
	}
	if _, err := backend.EnsureGuestAgentChannel(context.Background(), "44444444-4444-4444-8444-444444444444"); err != nil {
		t.Fatal(err)
	}
	legacy, err = backend.GetGuestAgentInfo(context.Background(), "44444444-4444-4444-8444-444444444444", true)
	if err != nil || !legacy.ChannelConfigured || legacy.State != domain.GuestAgentStateDisconnected {
		t.Fatalf("legacy after channel = %+v, %v", legacy, err)
	}
	if err := backend.PerformGuestAgentAction(context.Background(), "11111111-1111-4111-8111-111111111111", domain.GuestAgentActionSpec{Action: domain.GuestAgentActionSyncTime}); err != nil {
		t.Fatalf("sync-time error = %v", err)
	}
}
