import type { Role, VirtualNetwork, VirtualNetworkUpdateInput } from "@/api/types";

export type NetworkEditFormState = {
  mode: VirtualNetworkUpdateInput["mode"];
  subnet: string;
  dhcpStart: string;
  dhcpEnd: string;
  autostart: boolean;
};

export function networkEditForm(network: VirtualNetwork): NetworkEditFormState {
  const ipv4 = network.addresses.find((address) => address.family === "" || address.family === "ipv4");
  return {
    mode: network.forwardMode === "nat" ? "nat" : "isolated",
    subnet: ipv4 ? `${ipv4.address}/${ipv4.prefix}` : "",
    dhcpStart: ipv4?.dhcpStart ?? "",
    dhcpEnd: ipv4?.dhcpEnd ?? "",
    autostart: network.autostart,
  };
}

export function networkEditDisabledReason(role: Role | undefined, readOnly: boolean, network: VirtualNetwork) {
  if (readOnly) return "系统当前处于只读模式";
  if (role !== "operator" && role !== "admin") return "当前账号没有管理网络的权限";
  if (!network.persistent) return "临时网络不支持修改配置";
  if (network.forwardMode && network.forwardMode !== "nat" && network.forwardMode !== "isolated") {
    return "当前网络类型不支持修改配置";
  }
  if (network.active) return "请先停止网络再编辑";
  return "";
}

export function networkActionHint(role: Role | undefined, readOnly: boolean, network: VirtualNetwork) {
  if (readOnly) return "只读模式：网络变更已禁用，仍可查看 DHCP 租约。";
  if (role !== "operator" && role !== "admin") return "当前账号没有网络管理权限，仍可查看 DHCP 租约。";
  if (network.forwardMode && network.forwardMode !== "nat" && network.forwardMode !== "isolated") {
    return "当前网络类型不支持编辑配置；仍可启动、停止或删除网络定义。";
  }
  if (network.active) return "网络运行中：静态主机条目可实时增删；停止后可修改网络配置或删除网络。停止可能中断使用该网络的虚拟机。";
  return "";
}

export function networkUpdateInput(form: NetworkEditFormState): VirtualNetworkUpdateInput {
  return {
    mode: form.mode,
    subnet: form.subnet.trim(),
    dhcpStart: form.dhcpStart.trim() || undefined,
    dhcpEnd: form.dhcpEnd.trim() || undefined,
    autostart: form.autostart,
  };
}
