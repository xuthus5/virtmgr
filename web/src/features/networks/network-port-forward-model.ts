import type { PortForward, PortForwardInput } from "@/api/types";

export type PortForwardFormState = {
  name: string;
  family: "ipv4" | "ipv6";
  protocol: "tcp" | "udp";
  hostAddress: string;
  hostPort: string;
  guestAddress: string;
  guestPort: string;
  enabled: boolean;
};

export const emptyPortForwardForm: PortForwardFormState = {
  name: "",
  family: "ipv4",
  protocol: "tcp",
  hostAddress: "",
  hostPort: "",
  guestAddress: "",
  guestPort: "",
  enabled: true,
};

export function portForwardForm(item: PortForward): PortForwardFormState {
  return {
    name: item.name,
    family: item.family,
    protocol: item.protocol,
    hostAddress: item.hostAddress ?? "",
    hostPort: String(item.hostPort),
    guestAddress: item.guestAddress,
    guestPort: String(item.guestPort),
    enabled: item.enabled,
  };
}

export function buildPortForwardInput(form: PortForwardFormState): { input?: PortForwardInput; error?: string } {
  const name = form.name.trim();
  if (name.length < 2 || name.length > 64) return { error: "规则名称必须为 2 到 64 个字符。" };
  const hostRaw = form.hostPort.trim();
  const guestRaw = form.guestPort.trim();
  if (!/^\d+$/.test(hostRaw) || !/^\d+$/.test(guestRaw)) return { error: "宿主机端口和客户机端口必须是正整数。" };
  const hostPort = Number(hostRaw);
  const guestPort = Number(guestRaw);
  if (hostPort < 1 || hostPort > 65535 || guestPort < 1 || guestPort > 65535) return { error: "端口必须在 1 到 65535 之间。" };
  const guestAddress = form.guestAddress.trim();
  if (!guestAddress) return { error: "请填写客户机 IPv4 或 IPv6 地址。" };
  const hostAddress = form.hostAddress.trim();
  return {
    input: {
      name,
      family: form.family,
      protocol: form.protocol,
      hostAddress: hostAddress || undefined,
      hostPort,
      guestAddress,
      guestPort,
      enabled: form.enabled,
    },
  };
}
