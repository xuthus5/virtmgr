import { ApiError } from "@/api/client";
import type { NetworkAddress, NetworkDHCPHost, VirtualNetwork, VirtualNetworkCreateInput } from "@/api/types";

export type CreateFormState = {
  name: string;
  mode: VirtualNetworkCreateInput["mode"];
  subnet: string;
  dhcpStart: string;
  dhcpEnd: string;
  bridge: string;
  autostart: boolean;
  start: boolean;
};

export type HostFormState = {
  mac: string;
  ip: string;
  name: string;
};

export const initialCreateForm: CreateFormState = {
  name: "",
  mode: "nat",
  subnet: "10.0.0.0/24",
  dhcpStart: "",
  dhcpEnd: "",
  bridge: "",
  autostart: false,
  start: false,
};

export const initialHostForm: HostFormState = { mac: "", ip: "", name: "" };

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

const leaseTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
});

export function leaseExpiry(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : leaseTimeFormatter.format(date);
}
