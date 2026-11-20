import { ApiError } from "@/api/client";
import type { DHCPLease, NetworkAddress, NetworkDHCPHost, VirtualNetwork, VirtualNetworkCreateInput } from "@/api/types";

export type CreateFormState = {
  name: string;
  mode: VirtualNetworkCreateInput["mode"];
  subnet: string;
  dhcpStart: string;
  dhcpEnd: string;
  ipv6Subnet: string;
  ipv6Mode: "slaac" | "dhcpv6";
  ipv6DhcpStart: string;
  ipv6DhcpEnd: string;
  natIpv6: boolean;
  dnsDomain: string;
  dnsServers: string;
  bridge: string;
  autostart: boolean;
  start: boolean;
};

export type HostFormState = {
  mac: string;
  id: string;
  ip: string;
  name: string;
};

export const initialCreateForm: CreateFormState = {
  name: "",
  mode: "nat",
  subnet: "10.0.0.0/24",
  dhcpStart: "",
  dhcpEnd: "",
  ipv6Subnet: "",
  ipv6Mode: "dhcpv6",
  ipv6DhcpStart: "",
  ipv6DhcpEnd: "",
  natIpv6: true,
  dnsDomain: "",
  dnsServers: "",
  bridge: "",
  autostart: false,
  start: false,
};

export const initialHostForm: HostFormState = { mac: "", id: "", ip: "", name: "" };

export function splitDNSServers(value: string): string[] {
  return value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean);
}

export function hostIdentity(host: NetworkDHCPHost): string {
  return host.id || host.mac || host.ip || host.name || "";
}

export function hostIsIPv6(host: Pick<NetworkDHCPHost, "ip">): boolean {
  return host.ip.includes(":");
}

export function createNetworkInput(form: CreateFormState): VirtualNetworkCreateInput {
  if (form.mode === "bridge") {
    return { name: form.name.trim(), mode: "bridge", bridge: form.bridge.trim(), autostart: form.autostart, start: form.start };
  }
  const ipv6Subnet = form.ipv6Subnet.trim();
  return {
    name: form.name.trim(),
    mode: form.mode,
    subnet: form.subnet.trim(),
    dhcpStart: form.dhcpStart.trim() || undefined,
    dhcpEnd: form.dhcpEnd.trim() || undefined,
    ipv6Subnet: ipv6Subnet || undefined,
    ipv6Mode: ipv6Subnet ? form.ipv6Mode : undefined,
    ipv6DhcpStart: ipv6Subnet && form.ipv6Mode === "dhcpv6" ? form.ipv6DhcpStart.trim() || undefined : undefined,
    ipv6DhcpEnd: ipv6Subnet && form.ipv6Mode === "dhcpv6" ? form.ipv6DhcpEnd.trim() || undefined : undefined,
    natIpv6: form.mode === "nat" && Boolean(ipv6Subnet) ? form.natIpv6 : undefined,
    dnsDomain: form.dnsDomain.trim() || undefined,
    dnsServers: splitDNSServers(form.dnsServers),
    autostart: form.autostart,
    start: form.start,
  };
}

export function addressLabel(address: NetworkAddress) {
  return `${address.address}/${address.prefix}`;
}

export function actionError(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function staticHosts(network?: VirtualNetwork): NetworkDHCPHost[] {
  if (!network) return [];
  return network.addresses
    .flatMap((address) => address.hosts ?? [])
    .sort((left, right) => left.ip.localeCompare(right.ip, undefined, { numeric: true }));
}

export function staticHostCount(network: VirtualNetwork) {
  return network.addresses.reduce((total, address) => total + (address.hosts?.length ?? 0), 0);
}

export function networkHasIPv6(network?: VirtualNetwork): boolean {
  return Boolean(network?.addresses.some((address) => address.family === "ipv6"));
}

export function networkHasIPv4(network?: VirtualNetwork): boolean {
  return Boolean(network?.addresses.some((address) => !address.family || address.family === "ipv4"));
}

const leaseTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
});

export function leaseExpiry(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : leaseTimeFormatter.format(date);
}

export function leaseIsExpired(lease: DHCPLease, now = Date.now()) {
  if (!lease.expiresAt) return false;
  const expiresAt = new Date(lease.expiresAt).getTime();
  return !Number.isNaN(expiresAt) && expiresAt <= now;
}

export function leaseReservationIssue(lease: DHCPLease): string {
  const ipv6 = lease.family === "ipv6" || lease.ipAddress.includes(":");
  if (ipv6) {
    if (!lease.clientId?.trim() && !lease.hostname?.trim()) return "该租约缺少 DUID/Client ID 和主机名，无法生成 DHCPv6 静态保留。";
    return "";
  }
  if (!lease.mac?.trim()) return "该租约缺少 MAC 地址，无法生成 IPv4 静态保留。";
  return "";
}
