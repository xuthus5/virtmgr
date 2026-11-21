import { describe, expect, it } from "vitest";
import type { GuestAgentInfo } from "@/api/types";
import { guestAgentActionUnavailableReason, guestAgentCanAddChannel } from "@/features/guest-agent/guest-agent-model";

function connectedInfo(overrides: Partial<GuestAgentInfo> = {}): GuestAgentInfo {
  return {
    state: "connected",
    connected: true,
    channelConfigured: true,
    stale: false,
    os: {},
    interfaces: [],
    filesystems: [],
    routes: [],
    capabilities: {
      hostname: true,
      network: true,
      osInfo: true,
      fsInfo: true,
      shutdown: true,
      reboot: false,
      timeSync: true,
      trim: false,
      quiesce: true,
      fileAccess: false,
      passwordReset: false,
      sshKeys: false,
      processes: false,
      users: false,
      timezone: false,
      networkRoutes: false,
      packages: false,
    },
    probedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("guest agent action model", () => {
  it("allows only advertised capabilities", () => {
    const info = connectedInfo();
    expect(guestAgentActionUnavailableReason(info, "shutdown")).toBe("");
    expect(guestAgentActionUnavailableReason(info, "reboot")).toContain("不支持");
    expect(guestAgentActionUnavailableReason(info, "trim")).toContain("不支持");
  });

  it("requires a connected agent", () => {
    const info = connectedInfo({ state: "unresponsive", connected: false });
    expect(guestAgentActionUnavailableReason(info, "shutdown")).toContain("未连接");
  });
});

describe("guest agent channel model", () => {
  it("only permits stopped domains", () => {
    expect(guestAgentCanAddChannel("shutoff")).toBe(true);
    expect(guestAgentCanAddChannel("crashed")).toBe(true);
    expect(guestAgentCanAddChannel("running")).toBe(false);
    expect(guestAgentCanAddChannel("paused")).toBe(false);
  });
});

