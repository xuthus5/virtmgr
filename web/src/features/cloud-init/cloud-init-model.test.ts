import { describe, expect, it } from "vitest";
import {
  cloudInitBundleValidationError,
  cloudInitByteLength,
  cloudInitCombinedByteLength,
  cloudInitDraftIssues,
  cloudInitNetworkConfigIssues,
  cloudInitNetworkDraftFromNetwork,
  cloudInitNetworkIssues,
  cloudInitValidationError,
  defaultCloudInitUserData,
  parseCloudInitUserData,
  parseNetworkConfig,
  serializeCloudInit,
  serializeNetworkConfig,
} from "@/features/cloud-init/cloud-init-model";

const dualStackNetwork = {
  name: "default",
  mtu: 1500,
  dnsServers: ["2001:4860:4860::8888"],
  addresses: [
    { family: "ipv4", address: "192.168.122.1", prefix: 24, dhcpEnabled: true, dhcpStart: "192.168.122.2" },
    { family: "ipv6", address: "fd00:122::1", prefix: 64, dhcpEnabled: true, dhcpStart: "fd00:122::2", ipv6Mode: "dhcpv6" },
  ],
};

describe("cloud-init model", () => {
  it("serializes the visual fields into standard cloud-config YAML", () => {
    const yaml = serializeCloudInit({
      packageUpdate: true,
      packageUpgrade: false,
      packageRebootIfRequired: true,
      timezone: "Asia/Shanghai",
      locale: "zh_CN.UTF-8",
      packages: "nginx, curl\ntmux",
      runcmd: "systemctl enable --now nginx\necho ready",
      createUser: true,
      username: "deploy",
      groups: "sudo, docker",
      shell: "/bin/bash",
      sshAuthorizedKeys: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA test@example",
    });
    expect(yaml).toContain("#cloud-config");
    expect(yaml).toContain("package_update: true");
    expect(yaml).toContain("package_reboot_if_required: true");
    expect(yaml).toContain("packages:\n  - nginx\n  - curl\n  - tmux");
    expect(yaml).toContain("name: deploy");
    expect(yaml).toContain("groups:\n      - sudo\n      - docker");
    expect(yaml).toContain("ssh_authorized_keys:\n      - ssh-ed25519");
    expect(yaml).toContain("runcmd:\n  - systemctl enable --now nginx\n  - echo ready");
  });

  it("round-trips supported fields and preserves defaults", () => {
    const source = serializeCloudInit({
      packageUpdate: true,
      packageUpgrade: true,
      packageRebootIfRequired: false,
      timezone: "UTC",
      locale: "",
      packages: "qemu-guest-agent",
      runcmd: "systemctl enable qemu-guest-agent",
      createUser: true,
      username: "ops",
      groups: "sudo",
      shell: "/bin/bash",
      sshAuthorizedKeys: "ssh-rsa AAAAB3Nza test@example",
    });
    const parsed = parseCloudInitUserData(source);
    expect(parsed.visualizable).toBe(true);
    expect(parsed.unsupported).toEqual([]);
    expect(parsed.draft).toMatchObject({
      packageUpdate: true,
      packageUpgrade: true,
      packageRebootIfRequired: false,
      timezone: "UTC",
      packages: "qemu-guest-agent",
      runcmd: "systemctl enable qemu-guest-agent",
      createUser: true,
      username: "ops",
      shell: "/bin/bash",
    });
  });

  it("reports advanced fields without rejecting the YAML", () => {
    const parsed = parseCloudInitUserData(`#cloud-config
package_update: true
write_files:
  - path: /etc/motd
    content: hello
users:
  - name: deploy
    ssh_authorized_keys:
      - ssh-ed25519 AAAAC3 test@example
    shell: /bin/bash
  - name: second
runcmd:
  - [sh, -c, "echo complex"]
`);
    expect(parsed.visualizable).toBe(true);
    expect(parsed.syntaxValid).toBe(true);
    expect(parsed.unsupported).toEqual(expect.arrayContaining(["write_files", "多个用户", "runcmd 中的复杂命令"]));
  });

  it("distinguishes custom user-data, syntax errors, and empty input", () => {
    expect(parseCloudInitUserData("#!/bin/sh\necho hello")).toMatchObject({ cloudConfig: false, visualizable: false });
    expect(parseCloudInitUserData("#cloud-config\npackages: [")).toMatchObject({ cloudConfig: true, syntaxValid: false, visualizable: false });
    expect(parseCloudInitUserData("")).toMatchObject({ cloudConfig: false, visualizable: false });
  });

  it("validates user names and byte limits", () => {
    expect(cloudInitDraftIssues({ ...parseCloudInitUserData(defaultCloudInitUserData()).draft, createUser: true, username: "9bad" })).toHaveLength(1);
    expect(cloudInitByteLength("你好")).toBe(6);
    expect(cloudInitValidationError("")).toContain("必须填写");
    expect(cloudInitValidationError("#cloud-config\npackages: [")).toContain("语法错误");
    expect(cloudInitValidationError("#!/bin/sh\necho hello")).toBe("");
  });

  it("generates netplan v2 DHCP and static IPv4/IPv6 network config", () => {
    expect(serializeNetworkConfig({ mode: "disabled", interface: "ens3", ipv4Address: "", ipv4Gateway: "", ipv6Address: "", ipv6Gateway: "", dnsServers: "" })).toBe("");
    const dhcp = serializeNetworkConfig({ mode: "dhcp", interface: "ens3", ipv4Address: "", ipv4Gateway: "", ipv6Address: "", ipv6Gateway: "", dnsServers: "" });
    expect(dhcp).toContain("version: 2");
    expect(dhcp).toContain("dhcp4: true");

    const staticConfig = serializeNetworkConfig({
      mode: "static",
      interface: "ens3",
      ipv4Address: "192.0.2.10/24",
      ipv4Gateway: "192.0.2.1",
      ipv6Address: "2001:db8::10/64",
      ipv6Gateway: "2001:db8::1",
      dnsServers: "1.1.1.1, 2606:4700:4700::1111",
    });
    expect(staticConfig).toContain("addresses:\n      - 192.0.2.10/24\n      - 2001:db8::10/64");
    expect(staticConfig).toContain("via: 192.0.2.1");
    expect(staticConfig).toContain("via: 2001:db8::1");
    expect(staticConfig).toContain("- 1.1.1.1\n        - 2606:4700:4700::1111");
  });

  it("validates static networking and the combined cloud-init size", () => {
    expect(cloudInitNetworkIssues({ mode: "static", interface: "ens3", ipv4Address: "", ipv4Gateway: "", ipv6Address: "", ipv6Gateway: "", dnsServers: "" })).toHaveLength(1);
    expect(cloudInitNetworkIssues({ mode: "static", interface: "ens3", ipv4Address: "192.0.2.10", ipv4Gateway: "bad", ipv6Address: "", ipv6Gateway: "", dnsServers: "" }).length).toBeGreaterThan(0);
    expect(cloudInitNetworkIssues({ mode: "static", interface: "ens3", ipv4Address: "192.0.2.10/24", ipv4Gateway: "192.0.2.1", ipv6Address: "2001:db8::10/64", ipv6Gateway: "2001:db8::1", dnsServers: "" })).toEqual([]);
    expect(cloudInitCombinedByteLength("abc", "de", "f")).toBe(6);
    expect(cloudInitBundleValidationError("#cloud-config\n", "x".repeat(65 << 10))).toContain("总计");
  });

  it("generates DHCPv6-only and SLAAC IPv6 configurations", () => {
    const base = { mode: "dhcp" as const, interface: "ens3", ipv4Address: "", ipv4Gateway: "", ipv6Address: "", ipv6Gateway: "", dnsServers: "", ipv4Mode: "off" as const };
    const dhcpv6 = serializeNetworkConfig({ ...base, ipv6Mode: "dhcpv6" });
    expect(dhcpv6).toContain("dhcp4: false");
    expect(dhcpv6).toContain("dhcp6: true");

    const slaac = serializeNetworkConfig({ ...base, ipv6Mode: "slaac" });
    expect(slaac).toContain("dhcp4: false");
    expect(slaac).toContain("dhcp6: false");
    expect(slaac).toContain("accept-ra: true");
    expect(slaac).not.toContain("ipv6-address");
  });

  it("derives DHCPv6, MTU and gateway settings from the selected virtual network", () => {
    const draft = cloudInitNetworkDraftFromNetwork(dualStackNetwork);
    expect(draft).toMatchObject({ mode: "dhcp", ipv4Mode: "dhcp", ipv6Mode: "dhcpv6", mtu: "1500" });
    const config = serializeNetworkConfig(draft);
    expect(config).toContain("dhcp4: true");
    expect(config).toContain("dhcp6: true");
    expect(config).toContain("mtu: 1500");

    const slaac = cloudInitNetworkDraftFromNetwork({
      name: "slaac-net",
      addresses: [{ family: "ipv6", address: "fd00:130::1", prefix: 64, ipv6Mode: "slaac" }],
      mtu: 1280,
    });
    expect(slaac).toMatchObject({ mode: "dhcp", ipv4Mode: "off", ipv6Mode: "slaac", mtu: "1280" });
    expect(serializeNetworkConfig(slaac)).toContain("accept-ra: true");
  });

  it("parses existing netplan v2 back into the visual network draft", () => {
    const source = serializeNetworkConfig({
      mode: "static",
      interface: "enp1s0",
      ipv4Address: "192.168.122.10/24",
      ipv4Gateway: "192.168.122.1",
      ipv6Address: "fd00:122::10/64",
      ipv6Gateway: "fd00:122::1",
      dnsServers: "192.168.122.1, fd00:122::1",
      mtu: "1500",
    });
    const parsed = parseNetworkConfig(source);
    expect(parsed).toMatchObject({ valid: true, syntaxValid: true, visualizable: true });
    expect(parsed.draft).toMatchObject({
      mode: "static",
      interface: "enp1s0",
      ipv4Address: "192.168.122.10/24",
      ipv4Gateway: "192.168.122.1",
      ipv6Address: "fd00:122::10/64",
      ipv6Gateway: "fd00:122::1",
      dnsServers: "192.168.122.1, fd00:122::1",
      mtu: "1500",
    });
  });

  it("validates IPv6 subnets, gateways, DNS and netplan structure", () => {
    const valid = {
      mode: "static" as const,
      interface: "ens3",
      ipv4Address: "192.168.122.10/24",
      ipv4Gateway: "192.168.122.1",
      ipv6Address: "fd00:122::10/64",
      ipv6Gateway: "fd00:122::1",
      dnsServers: "192.168.122.1, fd00:122::1",
    };
    expect(cloudInitNetworkIssues(valid, dualStackNetwork)).toEqual([]);
    expect(cloudInitNetworkIssues({ ...valid, ipv6Address: "fd00:999::10/64" }, dualStackNetwork)).toEqual(expect.arrayContaining([expect.stringContaining("不在虚拟网络")]));
    expect(cloudInitNetworkIssues({ ...valid, ipv6Gateway: "fd00:122::2" }, dualStackNetwork)).toEqual(expect.arrayContaining([expect.stringContaining("网关应为")]));
    expect(cloudInitNetworkIssues({ ...valid, dnsServers: "not-an-ip" }, dualStackNetwork)).toEqual(expect.arrayContaining([expect.stringContaining("DNS 地址格式无效")]));
    expect(cloudInitNetworkIssues({ ...valid, mtu: "1200" }, dualStackNetwork)).toEqual(expect.arrayContaining([expect.stringContaining("MTU 不能小于 1280")]));
    expect(cloudInitNetworkConfigIssues("version: 1\nethernets: {}", dualStackNetwork)).toContain("network-config 必须是 Netplan v2，并包含 ethernets 映射。");
    expect(cloudInitNetworkConfigIssues("#cloud-config\npackages: [", dualStackNetwork)[0]).toContain("network-config YAML 语法错误");
  });

  it("covers IPv4-only, IPv6-only and dual-stack Cloud-init environments", () => {
    const ipv4Only = cloudInitNetworkDraftFromNetwork({
      name: "ipv4-only",
      addresses: [{ family: "ipv4", address: "10.20.0.1", prefix: 24, dhcpEnabled: true, dhcpStart: "10.20.0.2" }],
    });
    expect(ipv4Only).toMatchObject({ ipv4Mode: "dhcp", ipv6Mode: "off" });
    expect(serializeNetworkConfig(ipv4Only)).toContain("dhcp4: true");
    expect(serializeNetworkConfig(ipv4Only)).toContain("dhcp6: false");

    const ipv6Only = cloudInitNetworkDraftFromNetwork({
      name: "ipv6-only",
      addresses: [{ family: "ipv6", address: "fd00:60::1", prefix: 64, dhcpEnabled: true, dhcpStart: "fd00:60::2", ipv6Mode: "dhcpv6" }],
    });
    expect(ipv6Only).toMatchObject({ ipv4Mode: "off", ipv6Mode: "dhcpv6" });
    expect(serializeNetworkConfig(ipv6Only)).toContain("dhcp4: false");
    expect(serializeNetworkConfig(ipv6Only)).toContain("dhcp6: true");

    const dualStack = cloudInitNetworkDraftFromNetwork(dualStackNetwork);
    expect(dualStack).toMatchObject({ ipv4Mode: "dhcp", ipv6Mode: "dhcpv6" });
    expect(serializeNetworkConfig(dualStack)).toContain("dhcp4: true");
    expect(serializeNetworkConfig(dualStack)).toContain("dhcp6: true");
  });
});
