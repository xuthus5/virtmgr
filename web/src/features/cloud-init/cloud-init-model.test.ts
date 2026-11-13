import { describe, expect, it } from "vitest";
import {
  cloudInitBundleValidationError,
  cloudInitByteLength,
  cloudInitCombinedByteLength,
  cloudInitDraftIssues,
  cloudInitNetworkIssues,
  cloudInitValidationError,
  defaultCloudInitUserData,
  parseCloudInitUserData,
  serializeCloudInit,
  serializeNetworkConfig,
} from "@/features/cloud-init/cloud-init-model";

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
});
