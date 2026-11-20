import type { Role, VirtualNetwork, VirtualNetworkUpdateInput } from "@/api/types";
import { splitDNSServers } from "@/features/networks/network-ui";

export type NetworkEditFormState = {
  mode: VirtualNetworkUpdateInput["mode"];
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
};

export function networkEditForm(network: VirtualNetwork): NetworkEditFormState {
  const ipv4 = network.addresses.find((address) => address.family === "" || address.family === "ipv4");
  const ipv6 = network.addresses.find((address) => address.family === "ipv6");
  return {
    mode: network.forwardMode === "bridge" ? "bridge" : network.forwardMode === "nat" ? "nat" : "isolated",
    subnet: ipv4 ? `${ipv4.address}/${ipv4.prefix}` : "",
    dhcpStart: ipv4?.dhcpStart ?? "",
    dhcpEnd: ipv4?.dhcpEnd ?? "",
    ipv6Subnet: ipv6 ? `${ipv6.address}/${ipv6.prefix}` : "",
    ipv6Mode: ipv6?.ipv6Mode === "dhcpv6" ? "dhcpv6" : "slaac",
    ipv6DhcpStart: ipv6?.dhcpStart ?? "",
    ipv6DhcpEnd: ipv6?.dhcpEnd ?? "",
    natIpv6: network.forwardMode === "nat" ? Boolean(network.natIpv6) : network.forwardMode === "isolated",
    dnsDomain: network.dnsDomain ?? "",
    dnsServers: (network.dnsServers ?? []).join(", "),
    bridge: network.bridge ?? "",
    autostart: network.autostart,
  };
}

export function networkEditDisabledReason(role: Role | undefined, readOnly: boolean, network: VirtualNetwork) {
  if (readOnly) return "系统当前处于只读模式";
  if (role !== "operator" && role !== "admin") return "当前账号没有管理网络的权限";
  if (!network.persistent) return "临时网络不支持修改配置";
  if (network.forwardMode && network.forwardMode !== "nat" && network.forwardMode !== "isolated" && network.forwardMode !== "bridge") {
    return "当前网络类型不支持修改配置";
  }
  if (network.active) return "请先停止网络再编辑";
  return "";
}

export function networkActionHint(role: Role | undefined, readOnly: boolean, network: VirtualNetwork) {
  if (readOnly) return "只读模式：网络变更已禁用，仍可查看 DHCP 租约。";
  if (role !== "operator" && role !== "admin") return "当前账号没有网络管理权限，仍可查看 DHCP 租约。";
  if (network.forwardMode && network.forwardMode !== "nat" && network.forwardMode !== "isolated" && network.forwardMode !== "bridge") {
    return "当前网络类型不支持编辑配置；仍可启动、停止或删除网络定义。";
  }
  if (network.forwardMode === "bridge") {
    if (network.active) return "外部桥接网络由宿主机网桥提供地址与 DHCP；停止后可修改网桥配置，停止可能中断使用该网络的虚拟机。";
    return "";
  }
  if (network.active) return "网络运行中：静态主机条目可实时增删；停止后可修改网络配置或删除网络。停止可能中断使用该网络的虚拟机。";
  return "";
}

export function networkUpdateInput(form: NetworkEditFormState): VirtualNetworkUpdateInput {
  if (form.mode === "bridge") {
    return {
      mode: "bridge",
      bridge: form.bridge.trim(),
      autostart: form.autostart,
    };
  }
  const ipv6Subnet = form.ipv6Subnet.trim();
  return {
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
  };
}
