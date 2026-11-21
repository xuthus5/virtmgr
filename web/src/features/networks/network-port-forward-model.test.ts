import { describe, expect, it } from "vitest";
import type { PortForward } from "@/api/types";
import {
  buildPortForwardInput,
  emptyPortForwardForm,
  portForwardForm,
} from "@/features/networks/network-port-forward-model";

describe("network port forward model", () => {
  it("maps API rules into editable form state and back", () => {
    const item: PortForward = {
      id: "pf_1",
      network: "lab-net",
      name: "https",
      family: "ipv6",
      protocol: "tcp",
      hostAddress: "2001:db8::10",
      hostPort: 8443,
      guestAddress: "fd00:122::20",
      guestPort: 443,
      enabled: true,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    };
    const form = portForwardForm(item);
    expect(form).toMatchObject({ family: "ipv6", hostPort: "8443", guestPort: "443", hostAddress: "2001:db8::10" });
    expect(buildPortForwardInput(form)).toEqual({
      input: {
        name: "https",
        family: "ipv6",
        protocol: "tcp",
        hostAddress: "2001:db8::10",
        hostPort: 8443,
        guestAddress: "fd00:122::20",
        guestPort: 443,
        enabled: true,
      },
    });
  });

  it("validates names, ports and guest address", () => {
    expect(buildPortForwardInput({ ...emptyPortForwardForm, name: "x" }).error).toContain("2 到 64");
    expect(buildPortForwardInput({ ...emptyPortForwardForm, name: "ssh", hostPort: "abc" }).error).toContain("正整数");
    expect(buildPortForwardInput({ ...emptyPortForwardForm, name: "ssh", hostPort: "70000", guestPort: "22", guestAddress: "192.168.122.20" }).error).toContain("1 到 65535");
    expect(buildPortForwardInput({ ...emptyPortForwardForm, name: "ssh", hostPort: "2222", guestPort: "22" }).error).toContain("客户机");
  });

  it("omits an empty host address so nftables listens on all host addresses", () => {
    const result = buildPortForwardInput({
      ...emptyPortForwardForm,
      name: "ssh",
      hostPort: "2222",
      guestPort: "22",
      guestAddress: "192.168.122.20",
      hostAddress: "   ",
    });
    expect(result.input?.hostAddress).toBeUndefined();
  });
});
