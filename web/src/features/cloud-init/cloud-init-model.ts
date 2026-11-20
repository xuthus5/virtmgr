import { parseDocument, stringify } from "yaml";

export const maxCloudInitBytes = 64 << 10;

export type CloudInitNetworkMode = "disabled" | "dhcp" | "static";
export type CloudInitIPv4Mode = "off" | "dhcp";
export type CloudInitIPv6Mode = "off" | "dhcpv6" | "slaac";

export type CloudInitNetworkDraft = {
  mode: CloudInitNetworkMode;
  interface: string;
  ipv4Address: string;
  ipv4Gateway: string;
  ipv6Address: string;
  ipv6Gateway: string;
  dnsServers: string;
  ipv4Mode?: CloudInitIPv4Mode;
  ipv6Mode?: CloudInitIPv6Mode;
  mtu?: string;
};

export type CloudInitNetworkSourceAddress = {
  family: string;
  address: string;
  prefix: number;
  ipv6Mode?: string;
  dhcpEnabled?: boolean;
  dhcpStart?: string;
};

export type CloudInitNetworkSource = {
  name: string;
  addresses: CloudInitNetworkSourceAddress[];
  dnsServers?: string[];
  mtu?: number;
};

export type CloudInitNetworkParseResult = {
  valid: boolean;
  syntaxValid: boolean;
  visualizable: boolean;
  error: string;
  draft: CloudInitNetworkDraft;
};

export type CloudInitDraft = {
  packageUpdate: boolean;
  packageUpgrade: boolean;
  packageRebootIfRequired: boolean;
  timezone: string;
  locale: string;
  packages: string;
  runcmd: string;
  createUser: boolean;
  username: string;
  groups: string;
  shell: string;
  sshAuthorizedKeys: string;
};

export type CloudInitParseResult = {
  cloudConfig: boolean;
  syntaxValid: boolean;
  visualizable: boolean;
  error: string;
  unsupported: string[];
  draft: CloudInitDraft;
};

export const emptyCloudInitDraft: CloudInitDraft = {
  packageUpdate: false,
  packageUpgrade: false,
  packageRebootIfRequired: false,
  timezone: "",
  locale: "",
  packages: "",
  runcmd: "",
  createUser: false,
  username: "",
  groups: "sudo",
  shell: "/bin/bash",
  sshAuthorizedKeys: "",
};

export function defaultCloudInitDraft(): CloudInitDraft {
  return { ...emptyCloudInitDraft, packageUpdate: true };
}

export function defaultCloudInitUserData(): string {
  return serializeCloudInit(defaultCloudInitDraft());
}

export const emptyCloudInitNetworkDraft: CloudInitNetworkDraft = {
  mode: "disabled",
  interface: "ens3",
  ipv4Address: "",
  ipv4Gateway: "",
  ipv6Address: "",
  ipv6Gateway: "",
  dnsServers: "",
  ipv4Mode: "dhcp",
  ipv6Mode: "dhcpv6",
  mtu: "",
};

export function serializeNetworkConfig(draft: CloudInitNetworkDraft): string {
  if (draft.mode === "disabled") return "";
  const name = draft.interface.trim() || "ens3";
  const ethernet: Record<string, unknown> = {};
  if (draft.mode === "dhcp") {
    const ipv4Mode = draft.ipv4Mode ?? "dhcp";
    const ipv6Mode = draft.ipv6Mode ?? "dhcpv6";
    ethernet.dhcp4 = ipv4Mode === "dhcp";
    ethernet.dhcp6 = ipv6Mode === "dhcpv6";
    if (ipv6Mode === "slaac") ethernet["accept-ra"] = true;
  } else {
    ethernet.dhcp4 = false;
    ethernet.dhcp6 = false;
    const addresses = [draft.ipv4Address.trim(), draft.ipv6Address.trim()].filter(Boolean);
    if (addresses.length) ethernet.addresses = addresses;
    const routes: Array<Record<string, unknown>> = [];
    if (draft.ipv4Gateway.trim()) routes.push({ to: "default", via: draft.ipv4Gateway.trim() });
    if (draft.ipv6Gateway.trim()) routes.push({ to: "default", via: draft.ipv6Gateway.trim() });
    if (routes.length) ethernet.routes = routes;
  }
  const mtu = Number(draft.mtu?.trim());
  if (draft.mtu?.trim() && Number.isInteger(mtu)) ethernet.mtu = mtu;
  const nameservers = draft.dnsServers.split(/[\r\n,]+/).map((value) => value.trim()).filter(Boolean);
  if (nameservers.length) ethernet.nameservers = { addresses: nameservers };
  return `version: 2\n${stringify({ ethernets: { [name]: ethernet } }, { lineWidth: 0 })}`;
}

function lines(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function commaValues(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function packagesFrom(value: string): string[] {
  return value
    .split(/[\r\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function serializeCloudInit(draft: CloudInitDraft): string {
  const config: Record<string, unknown> = {};
  if (draft.packageUpdate) config.package_update = true;
  if (draft.packageUpgrade) config.package_upgrade = true;
  if (draft.packageRebootIfRequired) config.package_reboot_if_required = true;
  if (draft.timezone.trim()) config.timezone = draft.timezone.trim();
  if (draft.locale.trim()) config.locale = draft.locale.trim();
  const packages = packagesFrom(draft.packages);
  if (packages.length) config.packages = packages;

  if (draft.createUser && draft.username.trim()) {
    const user: Record<string, unknown> = {
      name: draft.username.trim(),
      lock_passwd: true,
    };
    const groups = commaValues(draft.groups);
    if (groups.length) user.groups = groups;
    if (draft.shell.trim()) user.shell = draft.shell.trim();
    user.sudo = "ALL=(ALL) NOPASSWD:ALL";
    const keys = lines(draft.sshAuthorizedKeys);
    if (keys.length) user.ssh_authorized_keys = keys;
    config.users = [user];
  }

  const runcmd = lines(draft.runcmd);
  if (runcmd.length) config.runcmd = runcmd;
  if (!Object.keys(config).length) return "#cloud-config\n";
  return `#cloud-config\n${stringify(config, { lineWidth: 0 })}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function stringList(value: unknown, unsupported: string[], name: string): string[] {
  if (!Array.isArray(value)) {
    if (value !== undefined) unsupported.push(name);
    return [];
  }
  const values: string[] = [];
  for (const item of value) {
    if (typeof item === "string") values.push(item);
    else unsupported.push(name);
  }
  return values;
}

const knownTopLevel = new Set([
  "package_update",
  "package_upgrade",
  "package_reboot_if_required",
  "timezone",
  "locale",
  "packages",
  "users",
  "runcmd",
]);

const knownUserFields = new Set([
  "name",
  "groups",
  "shell",
  "sudo",
  "lock_passwd",
  "ssh_authorized_keys",
]);

export function parseCloudInitUserData(value: string): CloudInitParseResult {
  const text = value.trim();
  const result: CloudInitParseResult = {
    cloudConfig: false,
    syntaxValid: true,
    visualizable: false,
    error: "",
    unsupported: [],
    draft: { ...emptyCloudInitDraft },
  };
  if (!text) {
    result.error = "尚未填写 user-data。";
    return result;
  }
  if (!text.startsWith("#cloud-config")) {
    result.error = "当前内容不是 #cloud-config 格式，可使用 YAML 源码模式继续编辑脚本或高级配置。";
    return result;
  }
  result.cloudConfig = true;
  const document = parseDocument(text, { prettyErrors: true });
  if (document.errors.length) {
    result.syntaxValid = false;
    result.error = document.errors[0]?.message ?? "YAML 语法错误。";
    return result;
  }

  let data: unknown;
  try {
    data = document.toJS();
  } catch (cause) {
    result.syntaxValid = false;
    result.error = cause instanceof Error ? cause.message : "无法解析 YAML。";
    return result;
  }
  if (data == null) data = {};
  if (!isRecord(data)) {
    result.error = "cloud-config 顶层必须是 YAML 映射。";
    return result;
  }

  const draft: CloudInitDraft = {
    ...emptyCloudInitDraft,
    packageUpdate: data.package_update === true,
    packageUpgrade: data.package_upgrade === true,
    packageRebootIfRequired: data.package_reboot_if_required === true,
    timezone: stringValue(data.timezone),
    locale: stringValue(data.locale),
    packages: stringList(data.packages, result.unsupported, "packages 中的复杂条目").join("\n"),
    runcmd: stringList(data.runcmd, result.unsupported, "runcmd 中的复杂命令").join("\n"),
  };

  for (const key of Object.keys(data)) {
    if (!knownTopLevel.has(key)) result.unsupported.push(key);
  }

  if (data.users !== undefined) {
    if (!Array.isArray(data.users)) {
      result.unsupported.push("users 不是列表");
    } else if (data.users.length) {
      if (data.users.length > 1) result.unsupported.push("多个用户");
      const user = data.users[0];
      if (isRecord(user)) {
        draft.createUser = true;
        draft.username = stringValue(user.name);
        if (Array.isArray(user.groups)) draft.groups = stringList(user.groups, result.unsupported, "用户 groups 中的复杂条目").join(", ");
        else draft.groups = stringValue(user.groups);
        draft.shell = stringValue(user.shell);
        draft.sshAuthorizedKeys = stringList(user.ssh_authorized_keys, result.unsupported, "SSH 公钥中的复杂条目").join("\n");
        for (const key of Object.keys(user)) {
          if (!knownUserFields.has(key)) result.unsupported.push(`users.${key}`);
        }
      } else {
        result.unsupported.push("users 中的字符串用户");
      }
    }
  }

  result.draft = draft;
  result.visualizable = true;
  return result;
}

export function cloudInitDraftIssues(draft: CloudInitDraft): string[] {
  const issues: string[] = [];
  if (draft.createUser && !draft.username.trim()) issues.push("启用登录用户后必须填写用户名。");
  if (draft.createUser && draft.username.trim() && !/^[a-z_][a-z0-9_-]*$/i.test(draft.username.trim())) {
    issues.push("用户名只能包含字母、数字、下划线和短横线，且不能以数字开头。");
  }
  return issues;
}

export function cloudInitByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function cloudInitCombinedByteLength(userData: string, networkConfig = "", vendorData = ""): number {
  return cloudInitByteLength(userData) + cloudInitByteLength(networkConfig) + cloudInitByteLength(vendorData);
}

export function cloudInitValidationError(value: string): string {
  if (!value.trim()) return "启用 Cloud-init 后必须填写用户数据。";
  if (cloudInitByteLength(value) > maxCloudInitBytes) return "Cloud-init 用户数据不能超过 64 KiB。";
  const parsed = parseCloudInitUserData(value);
  if (!parsed.cloudConfig) return "";
  if (!parsed.syntaxValid) return `Cloud-init YAML 语法错误：${parsed.error}`;
  const issues = cloudInitDraftIssues(parsed.draft);
  return issues[0] ?? "";
}

type ParsedIP = { family: 4 | 6; value: bigint; bits: number };
type ParsedCIDR = ParsedIP & { prefix: number };

function parseIPv4(value: string): bigint | null {
  const octets = value.split(".");
  if (octets.length !== 4) return null;
  let result = 0n;
  for (const octet of octets) {
    if (!/^\d{1,3}$/.test(octet)) return null;
    const number = Number(octet);
    if (number > 255) return null;
    result = (result << 8n) | BigInt(number);
  }
  return result;
}

function parseIPv6(value: string): bigint | null {
  let text = value.toLowerCase();
  if (!text.includes(":") || text.includes("%")) return null;
  if (text.includes(".")) {
    const separator = text.lastIndexOf(":");
    const ipv4 = parseIPv4(text.slice(separator + 1));
    if (ipv4 === null) return null;
    text = `${text.slice(0, separator + 1)}${(ipv4 >> 16n).toString(16)}:${(ipv4 & 0xffffn).toString(16)}`;
  }
  const compressed = text.split("::");
  if (compressed.length > 2) return null;
  const left = compressed[0] ? compressed[0].split(":") : [];
  const right = compressed.length === 2 && compressed[1] ? compressed[1].split(":") : [];
  if (compressed.length === 1 && left.length !== 8) return null;
  if (compressed.length === 2 && left.length + right.length >= 8) return null;
  const groups = [...left, ...Array<string>(8 - left.length - right.length).fill("0"), ...right];
  if (groups.length !== 8 || groups.some((group) => !/^[0-9a-f]{1,4}$/.test(group))) return null;
  return groups.reduce((result, group) => (result << 16n) | BigInt(Number.parseInt(group, 16)), 0n);
}

function parseIP(value: string, family?: 4 | 6): ParsedIP | null {
  const text = value.trim();
  if (family !== 6) {
    const ipv4 = parseIPv4(text);
    if (ipv4 !== null) return { family: 4, value: ipv4, bits: 32 };
    if (family === 4) return null;
  }
  const ipv6 = parseIPv6(text);
  return ipv6 === null ? null : { family: 6, value: ipv6, bits: 128 };
}

function parseCIDR(value: string, family?: 4 | 6): ParsedCIDR | null {
  const [address, prefixText, extra] = value.trim().split("/");
  if (!address || !prefixText || extra !== undefined || !/^\d{1,3}$/.test(prefixText)) return null;
  const parsed = parseIP(address, family);
  const prefix = Number(prefixText);
  if (!parsed || !Number.isInteger(prefix) || prefix < 0 || prefix > parsed.bits) return null;
  return { ...parsed, prefix };
}

function prefixContains(network: ParsedIP, prefix: number, candidate: ParsedIP): boolean {
  if (network.family !== candidate.family) return false;
  const shift = BigInt(network.bits - prefix);
  const mask = ((1n << BigInt(network.bits)) - 1n) ^ ((1n << shift) - 1n);
  return (network.value & mask) === (candidate.value & mask);
}

function dnsIssues(value: string): string[] {
  return value
    .split(/[\r\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean)
    .filter((item) => parseIP(item) === null)
    .map((item) => `DNS 地址格式无效：${item}`);
}

function mtuIssues(draft: CloudInitNetworkDraft): string[] {
  const text = draft.mtu?.trim() ?? "";
  if (!text) return [];
  const mtu = Number(text);
  if (!Number.isInteger(mtu) || mtu < 576 || mtu > 9000) return ["MTU 必须是 576 至 9000 之间的整数。"];
  const hasIPv6 = draft.mode === "static" ? Boolean(draft.ipv6Address.trim()) : (draft.ipv6Mode ?? "dhcpv6") !== "off";
  if (hasIPv6 && mtu < 1280) return ["启用 IPv6 时 MTU 不能小于 1280。"];
  return [];
}

function networkFamilyAddress(source: CloudInitNetworkSource | undefined, family: "ipv4" | "ipv6"): CloudInitNetworkSourceAddress | undefined {
  return source?.addresses.find((address) => address.family === family);
}

function staticFamilyIssues(
  addressText: string,
  gatewayText: string,
  family: 4 | 6,
  sourceAddress: CloudInitNetworkSourceAddress | undefined,
): string[] {
  const label = family === 4 ? "IPv4" : "IPv6";
  const issues: string[] = [];
  const address = addressText ? parseCIDR(addressText, family) : null;
  if (addressText && !address) {
    issues.push(`${label} 地址必须使用 CIDR 格式，例如 ${family === 4 ? "192.0.2.10/24" : "2001:db8::10/64"}。`);
  }
  const gateway = gatewayText ? parseIP(gatewayText, family) : null;
  if (gatewayText && !gateway) issues.push(`${label} 网关格式无效。`);
  if (!sourceAddress) return issues;

  const sourceGateway = parseIP(sourceAddress.address, family);
  if (!sourceGateway) return issues;
  if (address && address.prefix !== sourceAddress.prefix) {
    issues.push(`${label} 前缀应为 /${sourceAddress.prefix}，与虚拟网络 ${sourceAddress.address}/${sourceAddress.prefix} 保持一致。`);
  }
  if (address && !prefixContains(sourceGateway, sourceAddress.prefix, address)) {
    issues.push(`${label} 地址不在虚拟网络 ${sourceAddress.address}/${sourceAddress.prefix} 内。`);
  }
  if (gateway && !prefixContains(sourceGateway, sourceAddress.prefix, gateway)) {
    issues.push(`${label} 网关不在虚拟网络 ${sourceAddress.address}/${sourceAddress.prefix} 内。`);
  }
  if (gateway && gateway.value !== sourceGateway.value) {
    issues.push(`${label} 网关应为虚拟网络网关 ${sourceAddress.address}。`);
  }
  return issues;
}

export function cloudInitNetworkIssues(draft: CloudInitNetworkDraft, source?: CloudInitNetworkSource): string[] {
  const issues = mtuIssues(draft);
  const ipv4 = draft.ipv4Address.trim();
  const ipv6 = draft.ipv6Address.trim();
  const sourceIPv4 = networkFamilyAddress(source, "ipv4");
  const sourceIPv6 = networkFamilyAddress(source, "ipv6");
  if (draft.mode === "dhcp") {
    const ipv4Mode = draft.ipv4Mode ?? "dhcp";
    const ipv6Mode = draft.ipv6Mode ?? "dhcpv6";
    if (ipv4Mode === "off" && ipv6Mode === "off") issues.push("自动配置至少需要启用 IPv4 DHCP、IPv6 DHCPv6 或 IPv6 SLAAC 之一。");
    if (ipv4Mode === "dhcp" && source?.addresses.length && (!sourceIPv4 || (!sourceIPv4.dhcpEnabled && !sourceIPv4.dhcpStart))) {
      issues.push("当前虚拟网络没有启用 IPv4 DHCP。");
    }
    if (ipv6Mode !== "off" && source?.addresses.length && !sourceIPv6) issues.push("当前虚拟网络没有配置 IPv6。");
    if (ipv6Mode === "dhcpv6" && sourceIPv6?.ipv6Mode === "slaac") issues.push("当前虚拟网络仅启用 IPv6 SLAAC，请将 IPv6 自动配置切换为 SLAAC。");
    issues.push(...dnsIssues(draft.dnsServers));
    return issues;
  }
  if (draft.mode !== "static") return issues;
  if (!ipv4 && !ipv6) issues.push("静态网络至少需要填写一个 IPv4 或 IPv6 地址，格式为地址/前缀。");
  issues.push(...staticFamilyIssues(ipv4, draft.ipv4Gateway.trim(), 4, sourceIPv4));
  issues.push(...staticFamilyIssues(ipv6, draft.ipv6Gateway.trim(), 6, sourceIPv6));
  if (source?.addresses.length && ipv4 && !sourceIPv4) issues.push("当前虚拟网络没有配置 IPv4 子网。");
  if (source?.addresses.length && ipv6 && !sourceIPv6) issues.push("当前虚拟网络没有配置 IPv6 子网。");
  issues.push(...dnsIssues(draft.dnsServers));
  return issues;
}

export function cloudInitNetworkDraftFromNetwork(source: CloudInitNetworkSource): CloudInitNetworkDraft {
  const ipv4 = networkFamilyAddress(source, "ipv4");
  const ipv6 = networkFamilyAddress(source, "ipv6");
  const hasDHCP = source.addresses.some((address) => address.dhcpEnabled || Boolean(address.dhcpStart));
  const hasSLAAC = source.addresses.some((address) => address.ipv6Mode === "slaac");
  const mode: CloudInitNetworkMode = hasDHCP || hasSLAAC || source.addresses.length === 0 ? "dhcp" : "static";
  const ipv4Mode: CloudInitIPv4Mode = ipv4
    ? ipv4.dhcpEnabled || ipv4.dhcpStart ? "dhcp" : "off"
    : source.addresses.length === 0 ? "dhcp" : "off";
  let ipv6Mode: CloudInitIPv6Mode = "off";
  if (ipv6?.ipv6Mode === "slaac") ipv6Mode = "slaac";
  else if (ipv6 && (ipv6.dhcpEnabled || ipv6.dhcpStart || ipv6.ipv6Mode === "dhcpv6")) ipv6Mode = "dhcpv6";
  else if (!ipv6 && source.addresses.length === 0) ipv6Mode = "dhcpv6";
  const staticGateways = mode === "static" ? [ipv4?.address, ipv6?.address].filter((value): value is string => Boolean(value)) : [];
  return {
    ...emptyCloudInitNetworkDraft,
    mode,
    ipv4Mode,
    ipv6Mode,
    ipv4Gateway: mode === "static" ? ipv4?.address ?? "" : "",
    ipv6Gateway: mode === "static" ? ipv6?.address ?? "" : "",
    dnsServers: staticGateways.join(", "),
    mtu: source.mtu ? String(source.mtu) : "",
  };
}

export function parseNetworkConfig(value: string): CloudInitNetworkParseResult {
  const result: CloudInitNetworkParseResult = {
    valid: false,
    syntaxValid: true,
    visualizable: false,
    error: "",
    draft: { ...emptyCloudInitNetworkDraft },
  };
  const text = value.trim();
  if (!text) {
    result.valid = true;
    return result;
  }
  const document = parseDocument(text, { prettyErrors: true });
  if (document.errors.length) {
    result.syntaxValid = false;
    result.error = document.errors[0]?.message ?? "YAML 语法错误。";
    return result;
  }
  let data: unknown;
  try {
    data = document.toJS();
  } catch (cause) {
    result.syntaxValid = false;
    result.error = cause instanceof Error ? cause.message : "无法解析 YAML。";
    return result;
  }
  if (!isRecord(data) || data.version !== 2 || !isRecord(data.ethernets)) {
    result.error = "network-config 必须是 Netplan v2，并包含 ethernets 映射。";
    return result;
  }
  const interfaces = Object.entries(data.ethernets);
  if (interfaces.length !== 1) {
    result.valid = true;
    result.error = "可视化编辑器只支持单个以太网接口；当前内容保留在 YAML 中。";
    return result;
  }
  const [name, valueForInterface] = interfaces[0]!;
  if (!isRecord(valueForInterface)) {
    result.error = "以太网接口配置必须是 YAML 映射。";
    return result;
  }
  const draft: CloudInitNetworkDraft = { ...emptyCloudInitNetworkDraft, interface: name };
  const known = new Set(["dhcp4", "dhcp6", "accept-ra", "addresses", "routes", "nameservers", "mtu"]);
  const unsupported = Object.keys(valueForInterface).filter((key) => !known.has(key));
  if (unsupported.length) {
    result.valid = true;
    result.error = `network-config 包含高级字段（${unsupported.join("、")}），请在 YAML 中继续编辑。`;
    return result;
  }
  draft.ipv4Mode = valueForInterface.dhcp4 === true ? "dhcp" : "off";
  if (valueForInterface.dhcp6 === true) draft.ipv6Mode = "dhcpv6";
  else if (valueForInterface["accept-ra"] === true) draft.ipv6Mode = "slaac";
  else draft.ipv6Mode = "off";
  if (Array.isArray(valueForInterface.addresses)) {
    for (const address of valueForInterface.addresses) {
      if (typeof address !== "string") continue;
      const parsed = parseCIDR(address);
      if (!parsed) continue;
      if (parsed.family === 4 && !draft.ipv4Address) draft.ipv4Address = address;
      if (parsed.family === 6 && !draft.ipv6Address) draft.ipv6Address = address;
    }
  }
  if (Array.isArray(valueForInterface.routes)) {
    for (const route of valueForInterface.routes) {
      if (!isRecord(route) || (route.to !== "default" && route.to !== "0.0.0.0/0" && route.to !== "::/0") || typeof route.via !== "string") continue;
      const gateway = parseIP(route.via);
      if (gateway?.family === 4 && !draft.ipv4Gateway) draft.ipv4Gateway = route.via;
      if (gateway?.family === 6 && !draft.ipv6Gateway) draft.ipv6Gateway = route.via;
    }
  }
  if (isRecord(valueForInterface.nameservers) && Array.isArray(valueForInterface.nameservers.addresses)) {
    draft.dnsServers = valueForInterface.nameservers.addresses.filter((item): item is string => typeof item === "string").join(", ");
  }
  if (typeof valueForInterface.mtu === "number") draft.mtu = String(valueForInterface.mtu);
  draft.mode = valueForInterface.dhcp4 === true || valueForInterface.dhcp6 === true || valueForInterface["accept-ra"] === true
    ? "dhcp"
    : draft.ipv4Address || draft.ipv6Address || draft.ipv4Gateway || draft.ipv6Gateway
      ? "static"
      : "disabled";
  result.draft = draft;
  result.valid = true;
  result.visualizable = draft.mode !== "disabled";
  return result;
}

export function cloudInitNetworkConfigIssues(value: string, source?: CloudInitNetworkSource): string[] {
  if (!value.trim()) return [];
  const parsed = parseNetworkConfig(value);
  if (!parsed.syntaxValid) return [`network-config YAML 语法错误：${parsed.error}`];
  if (!parsed.valid) return [parsed.error];
  if (!parsed.visualizable) return [];
  return cloudInitNetworkIssues(parsed.draft, source);
}

export function cloudInitBundleValidationError(userData: string, networkConfig = "", vendorData = "", source?: CloudInitNetworkSource): string {
  const userDataError = cloudInitValidationError(userData);
  if (userDataError) return userDataError;
  if (cloudInitCombinedByteLength(userData, networkConfig, vendorData) > maxCloudInitBytes) {
    return "Cloud-init user-data、network-config 与 vendor-data 总计不能超过 64 KiB。";
  }
  const networkError = cloudInitNetworkConfigIssues(networkConfig, source)[0];
  if (networkError) return networkError;
  return "";
}
