import { describe, expect, it } from "vitest";
import { createNetworkInput, hostIdentity, hostIsIPv6, initialCreateForm, initialHostForm, leaseIsExpired, leaseReservationIssue } from "@/features/networks/network-ui";

describe("network ui model", () => {
  it("builds IPv6-only DHCPv6 input without requiring IPv4", () => {
    expect(createNetworkInput({
      ...initialCreateForm,
      name: " ipv6-only ",
      mode: "isolated",
      subnet: "",
      ipv6Subnet: " fd00:60::/64 ",
      ipv6Mode: "dhcpv6",
      ipv6DhcpStart: " fd00:60::10 ",
      ipv6DhcpEnd: " fd00:60::20 ",
      dnsDomain: " lab.example.com ",
      dnsServers: "1.1.1.1, 2606:4700:4700::1111",
    })).toEqual({
      name: "ipv6-only",
      mode: "isolated",
      subnet: "",
      dhcpStart: undefined,
      dhcpEnd: undefined,
      ipv6Subnet: "fd00:60::/64",
      ipv6Mode: "dhcpv6",
      ipv6DhcpStart: "fd00:60::10",
      ipv6DhcpEnd: "fd00:60::20",
      natIpv6: undefined,
      dnsDomain: "lab.example.com",
      dnsServers: ["1.1.1.1", "2606:4700:4700::1111"],
      autostart: false,
      start: false,
    });
  });

  it("omits DHCPv6 fields and NAT66 for SLAAC or isolated networks", () => {
    const slaac = createNetworkInput({
      ...initialCreateForm,
      name: "slaac-net",
      mode: "nat",
      subnet: "10.60.0.0/24",
      ipv6Subnet: "fd00:61::/64",
      ipv6Mode: "slaac",
      ipv6DhcpStart: "fd00:61::10",
      natIpv6: false,
    });
    expect(slaac.ipv6Mode).toBe("slaac");
    expect(slaac.ipv6DhcpStart).toBeUndefined();
    expect(slaac.natIpv6).toBe(false);
  });

  it("builds a bridge input without managed addressing", () => {
    expect(createNetworkInput({ ...initialCreateForm, name: "br-net", mode: "bridge", bridge: " br0 ", start: true })).toEqual({
      name: "br-net",
      mode: "bridge",
      bridge: "br0",
      autostart: false,
      start: true,
    });
  });

  it("uses DUID, MAC, IP or hostname as a stable host key", () => {
    expect(hostIdentity({ ...initialHostForm, id: "00:03:00:01:52:54:00:12:34:56", ip: "fd00::50" })).toBe("00:03:00:01:52:54:00:12:34:56");
    expect(hostIdentity({ ...initialHostForm, mac: "52:54:00:12:34:56", ip: "10.0.0.50" })).toBe("52:54:00:12:34:56");
    expect(hostIdentity({ ...initialHostForm, ip: "fd00::50" })).toBe("fd00::50");
    expect(hostIsIPv6({ ip: "fd00::50" })).toBe(true);
    expect(hostIsIPv6({ ip: "10.0.0.50" })).toBe(false);
  });

  it("requires a DHCPv6 identity or hostname before reserving a lease", () => {
    expect(leaseReservationIssue({ family: "ipv6", mac: "52:54:00:12:34:56", ipAddress: "fd00::50", prefix: 64, clientId: "00:03:00:01:aa" })).toBe("");
    expect(leaseReservationIssue({ family: "ipv6", mac: "52:54:00:12:34:56", ipAddress: "fd00::50", prefix: 64, hostname: "db-v6" })).toBe("");
    expect(leaseReservationIssue({ family: "ipv6", mac: "52:54:00:12:34:56", ipAddress: "fd00::50", prefix: 64 })).toContain("DUID");
    expect(leaseReservationIssue({ family: "ipv4", mac: "", ipAddress: "10.0.0.50", prefix: 24 })).toContain("MAC");
  });

  it("marks leases at or past their expiry time", () => {
    const now = Date.parse("2026-11-20T10:00:00Z");
    expect(leaseIsExpired({ family: "ipv6", mac: "", ipAddress: "fd00::50", prefix: 64, expiresAt: "2026-11-20T09:59:59Z" }, now)).toBe(true);
    expect(leaseIsExpired({ family: "ipv6", mac: "", ipAddress: "fd00::50", prefix: 64, expiresAt: "2026-11-20T10:00:01Z" }, now)).toBe(false);
    expect(leaseIsExpired({ family: "ipv6", mac: "", ipAddress: "fd00::50", prefix: 64 }, now)).toBe(false);
  });
});
