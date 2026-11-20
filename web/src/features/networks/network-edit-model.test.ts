import { describe, expect, it } from "vitest";
import type { VirtualNetwork } from "@/api/types";
import { networkActionHint, networkEditDisabledReason, networkEditForm, networkUpdateInput } from "@/features/networks/network-edit-model";

const inactiveNetwork: VirtualNetwork = {
  name: "lab-net",
  active: false,
  persistent: true,
  autostart: false,
  forwardMode: "nat",
  addresses: [{ family: "ipv4", address: "10.20.30.1", prefix: 24, dhcpStart: "10.20.30.10", dhcpEnd: "10.20.30.200" }],
};

describe("network edit model", () => {
  it("maps an inactive network into an editable form", () => {
    expect(networkEditForm(inactiveNetwork)).toEqual({
      mode: "nat",
      subnet: "10.20.30.1/24",
      dhcpStart: "10.20.30.10",
      dhcpEnd: "10.20.30.200",
      ipv6Subnet: "",
      ipv6Mode: "slaac",
      ipv6DhcpStart: "",
      ipv6DhcpEnd: "",
      natIpv6: false,
      dnsDomain: "",
      dnsServers: "",
      bridge: "",
      mtu: "",
      autostart: false,
    });
  });

  it("maps an external bridge network without IPv4 fields", () => {
    expect(networkEditForm({
      name: "br-prod",
      active: false,
      persistent: true,
      autostart: true,
      forwardMode: "bridge",
      bridge: "br0",
      addresses: [],
    })).toEqual({
      mode: "bridge",
      subnet: "",
      dhcpStart: "",
      dhcpEnd: "",
      ipv6Subnet: "",
      ipv6Mode: "slaac",
      ipv6DhcpStart: "",
      ipv6DhcpEnd: "",
      natIpv6: false,
      dnsDomain: "",
      dnsServers: "",
      bridge: "br0",
      mtu: "",
      autostart: true,
    });
  });

  it("maps IPv6-only DUID metadata and NAT66 state", () => {
    expect(networkEditForm({
      name: "ipv6-only",
      active: false,
      persistent: true,
      autostart: false,
      forwardMode: "nat",
      natIpv6: true,
      dnsDomain: "lab.example.com",
      dnsServers: ["1.1.1.1", "2606:4700:4700::1111"],
      addresses: [{ family: "ipv6", address: "fd00:70::1", prefix: 64, ipv6Mode: "dhcpv6", dhcpStart: "fd00:70::2", dhcpEnd: "fd00:70::ff" }],
    })).toMatchObject({
      subnet: "",
      ipv6Subnet: "fd00:70::1/64",
      ipv6Mode: "dhcpv6",
      ipv6DhcpStart: "fd00:70::2",
      ipv6DhcpEnd: "fd00:70::ff",
      natIpv6: true,
      dnsDomain: "lab.example.com",
      dnsServers: "1.1.1.1, 2606:4700:4700::1111",
    });
  });

  it("defaults NAT66 when an isolated dual-stack network is switched to NAT", () => {
    expect(networkEditForm({
      name: "isolated-dual",
      active: false,
      persistent: true,
      autostart: false,
      forwardMode: "isolated",
      natIpv6: false,
      addresses: [{ family: "ipv6", address: "fd00:71::1", prefix: 64, ipv6Mode: "dhcpv6", dhcpStart: "fd00:71::2", dhcpEnd: "fd00:71::ff" }],
    }).natIpv6).toBe(true);
  });

  it("requires the network to be stopped and supported", () => {
    expect(networkEditDisabledReason("admin", false, inactiveNetwork)).toBe("");
    expect(networkEditDisabledReason("admin", false, { ...inactiveNetwork, active: true })).toContain("停止网络");
    expect(networkEditDisabledReason("admin", false, { ...inactiveNetwork, forwardMode: "bridge", bridge: "br0", addresses: [] })).toBe("");
    expect(networkEditDisabledReason("admin", false, { ...inactiveNetwork, forwardMode: "route" })).toContain("不支持");
    expect(networkEditDisabledReason("viewer", false, inactiveNetwork)).toContain("权限");
    expect(networkEditDisabledReason("admin", true, inactiveNetwork)).toContain("只读");
  });

  it("normalizes optional DHCP values", () => {
    expect(networkUpdateInput({ mode: "isolated", subnet: " 10.40.0.0/24 ", dhcpStart: " ", dhcpEnd: "", ipv6Subnet: " fd00:40::/64 ", ipv6Mode: "dhcpv6", ipv6DhcpStart: "", ipv6DhcpEnd: " ", natIpv6: true, dnsDomain: " lab.example.com ", dnsServers: "1.1.1.1, 2606:4700:4700::1111", bridge: "", mtu: " 1500 ", autostart: true })).toEqual({
      mode: "isolated",
      subnet: "10.40.0.0/24",
      dhcpStart: undefined,
      dhcpEnd: undefined,
      ipv6Subnet: "fd00:40::/64",
      ipv6Mode: "dhcpv6",
      ipv6DhcpStart: undefined,
      ipv6DhcpEnd: undefined,
      natIpv6: undefined,
      dnsDomain: "lab.example.com",
      dnsServers: ["1.1.1.1", "2606:4700:4700::1111"],
      mtu: 1500,
      autostart: true,
    });
    expect(networkUpdateInput({ mode: "bridge", subnet: "", dhcpStart: "", dhcpEnd: "", ipv6Subnet: "", ipv6Mode: "dhcpv6", ipv6DhcpStart: "", ipv6DhcpEnd: "", natIpv6: true, dnsDomain: "", dnsServers: "", bridge: " br1 ", mtu: "", autostart: true })).toEqual({
      mode: "bridge",
      bridge: "br1",
      autostart: true,
    });
  });

  it("explains unavailable actions in the row", () => {
    expect(networkActionHint("admin", false, inactiveNetwork)).toBe("");
    expect(networkActionHint("admin", false, { ...inactiveNetwork, active: true })).toContain("停止后");
    expect(networkActionHint("admin", false, { ...inactiveNetwork, active: true, forwardMode: "bridge", bridge: "br0", addresses: [] })).toContain("宿主机网桥");
    expect(networkActionHint("admin", false, { ...inactiveNetwork, forwardMode: "route" })).toContain("不支持编辑");
    expect(networkActionHint("viewer", false, inactiveNetwork)).toContain("没有网络管理权限");
    expect(networkActionHint("admin", true, inactiveNetwork)).toContain("只读模式");
  });
});
