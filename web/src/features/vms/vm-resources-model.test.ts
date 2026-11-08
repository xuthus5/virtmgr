import { describe, expect, it } from "vitest";
import type { VM } from "@/api/types";
import {
  activeResourceLimits,
  isVMActiveResources,
  validateVMResources,
  vmResourcesDisabledReason,
} from "@/features/vms/vm-resources-model";

const vm: VM = {
  uuid: "uuid-a",
  name: "api-01",
  state: "running",
  stateReason: 0,
  persistent: true,
  autostart: false,
  vcpus: 2,
  maxVcpus: 4,
  memoryBytes: 2 * 1024 ** 3,
  maxMemoryBytes: 4 * 1024 ** 3,
  cpuTimeNanos: 0,
  firmware: "bios",
  secureBoot: false,
  cloudInit: false,
  disks: [],
  interfaces: [],
  graphics: [],
  passthrough: [],
  ipAddresses: [],
  agentConnected: false,
  allowedActions: [],
};

describe("VM resources model", () => {
  it("shows resource editing only for writable persistent domains", () => {
    expect(vmResourcesDisabledReason("admin", false, vm)).toBe("");
    expect(vmResourcesDisabledReason("operator", false, vm)).toBe("");
    expect(vmResourcesDisabledReason("viewer", false, vm)).toContain("没有虚拟机操作权限");
    expect(vmResourcesDisabledReason("admin", true, vm)).toContain("只读模式");
    expect(vmResourcesDisabledReason("admin", false, { ...vm, persistent: false })).toContain("临时域");
    expect(vmResourcesDisabledReason("admin", false, { ...vm, state: "shutting-down" })).toContain("当前状态");
  });

  it("distinguishes live and config-only states", () => {
    expect(isVMActiveResources("running")).toBe(true);
    expect(isVMActiveResources("paused")).toBe(true);
    expect(isVMActiveResources("shutoff")).toBe(false);
    expect(isVMActiveResources("crashed")).toBe(false);
  });

  it("uses the active maximum as the live adjustment limit", () => {
    expect(activeResourceLimits(vm)).toEqual({ vcpus: 4, memoryMiB: 4096 });
    expect(activeResourceLimits({ ...vm, maxVcpus: 0, maxMemoryBytes: 0 })).toEqual({ vcpus: 2, memoryMiB: 2048 });
  });

  it("validates numeric ranges and active limits", () => {
    expect(validateVMResources(vm, "4", "4096").input).toEqual({ vcpus: 4, memoryMiB: 4096 });
    expect(validateVMResources(vm, "0", "4096").error).toContain("1 到 255");
    expect(validateVMResources(vm, "2", "127").error).toContain("128");
    expect(validateVMResources(vm, "5", "4096").error).toContain("最多支持 4 个 vCPU");
    expect(validateVMResources(vm, "4", "4097").error).toContain("最多支持 4096 MiB");
    expect(validateVMResources({ ...vm, state: "shutoff" }, "16", "32768").input).toEqual({ vcpus: 16, memoryMiB: 32768 });
  });
});
