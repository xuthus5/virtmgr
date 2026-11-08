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
      autostart: false,
    });
  });

  it("requires the network to be stopped and supported", () => {
    expect(networkEditDisabledReason("admin", false, inactiveNetwork)).toBe("");
    expect(networkEditDisabledReason("admin", false, { ...inactiveNetwork, active: true })).toContain("停止网络");
    expect(networkEditDisabledReason("admin", false, { ...inactiveNetwork, forwardMode: "bridge" })).toContain("不支持");
    expect(networkEditDisabledReason("admin", false, { ...inactiveNetwork, active: true, forwardMode: "bridge" })).toContain("不支持");
    expect(networkEditDisabledReason("viewer", false, inactiveNetwork)).toContain("权限");
    expect(networkEditDisabledReason("admin", true, inactiveNetwork)).toContain("只读");
  });

  it("normalizes optional DHCP values", () => {
    expect(networkUpdateInput({ mode: "isolated", subnet: " 10.40.0.0/24 ", dhcpStart: " ", dhcpEnd: "", autostart: true })).toEqual({
      mode: "isolated",
      subnet: "10.40.0.0/24",
      dhcpStart: undefined,
      dhcpEnd: undefined,
      autostart: true,
    });
  });

  it("explains unavailable actions in the row", () => {
    expect(networkActionHint("admin", false, inactiveNetwork)).toBe("");
    expect(networkActionHint("admin", false, { ...inactiveNetwork, active: true })).toContain("停止后");
    expect(networkActionHint("admin", false, { ...inactiveNetwork, forwardMode: "bridge" })).toContain("不支持编辑");
    expect(networkActionHint("viewer", false, inactiveNetwork)).toContain("没有网络管理权限");
    expect(networkActionHint("admin", true, inactiveNetwork)).toContain("只读模式");
  });
});
