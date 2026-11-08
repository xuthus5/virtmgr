import { describe, expect, it } from "vitest";
import type { Disk, VM } from "@/api/types";
import {
  defaultDiskCapacityGiB,
  validateDiskCapacity,
  vmDiskResizeDisabledReason,
} from "@/features/vms/vm-disk-resize-model";

const disk: Disk = {
  device: "disk",
  target: "vda",
  bus: "virtio",
  format: "qcow2",
  source: "/var/lib/libvirt/images/demo.qcow2",
  readOnly: false,
  capacityBytes: 80 * 1024 ** 3,
  allocationBytes: 42 * 1024 ** 3,
};

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
  disks: [disk],
  interfaces: [],
  graphics: [],
  passthrough: [],
  ipAddresses: [],
  agentConnected: false,
  allowedActions: [],
};

describe("VM disk resize model", () => {
  it("only enables resizing for writable managed data disks", () => {
    expect(vmDiskResizeDisabledReason("operator", false, vm, disk)).toBe("");
    expect(vmDiskResizeDisabledReason("viewer", false, vm, disk)).toContain("没有虚拟机操作权限");
    expect(vmDiskResizeDisabledReason("admin", true, vm, disk)).toContain("只读模式");
    expect(vmDiskResizeDisabledReason("admin", false, { ...vm, persistent: false }, disk)).toContain("临时域");
    expect(vmDiskResizeDisabledReason("admin", false, { ...vm, state: "shutting-down" }, disk)).toContain("当前状态");
    expect(vmDiskResizeDisabledReason("admin", false, vm, { ...disk, device: "cdrom" })).toContain("数据磁盘");
    expect(vmDiskResizeDisabledReason("admin", false, vm, { ...disk, capacityBytes: 0 })).toContain("当前容量");
  });

  it("suggests a larger default and validates expansion", () => {
    expect(defaultDiskCapacityGiB(disk)).toBe(90);
    expect(validateDiskCapacity(disk, "100").input).toEqual({ capacityBytes: 100 * 1024 ** 3 });
    expect(validateDiskCapacity(disk, "80").error).toContain("必须大于当前容量");
    expect(validateDiskCapacity(disk, "79.5").error).toContain("必须大于当前容量");
    expect(validateDiskCapacity(disk, "0").error).toContain("至少");
    expect(validateDiskCapacity(disk, "100.1234").error).toContain("数字");
    expect(validateDiskCapacity(disk, "1048577").error).toContain("1 PiB");
  });
});
