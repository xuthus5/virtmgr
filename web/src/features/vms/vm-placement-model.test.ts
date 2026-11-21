import { describe, expect, it } from "vitest";
import {
  buildPlacement,
  hugePageBytes,
  parseCPUSet,
  placementFormFrom,
} from "@/features/vms/vm-placement-model";

describe("VM placement model", () => {
  it("parses and expands CPU sets", () => {
    expect(parseCPUSet("0-2,5,8-9")).toEqual([0, 1, 2, 5, 8, 9]);
    expect(parseCPUSet(" 3-1 ")).toBeNull();
    expect(parseCPUSet("0-65536")).toBeNull();
  });

  it("parses HugePage sizes and rejects overflow or unsupported units", () => {
    expect(hugePageBytes("2MiB")).toBe(2 * 1024 ** 2);
    expect(hugePageBytes("1GiB")).toBe(1024 ** 3);
    expect(hugePageBytes("2MB")).toBeNull();
    expect(hugePageBytes("0MiB")).toBeNull();
  });

  it("builds a valid NUMA and pinning specification", () => {
    const result = buildPlacement({
      hugePageSize: "2MiB",
      emulatorPin: "8",
      numaCells: [
        { id: "0", cpuset: "0-1", memoryMiB: "1024" },
        { id: "1", cpuset: "2-3", memoryMiB: "1024" },
      ],
      vcpuPins: [{ vcpu: "0", cpuset: "0" }, { vcpu: "1", cpuset: "1" }],
    }, 4, 2048);
    expect(result.error).toBeUndefined();
    expect(result.input).toEqual({
      hugePageSize: "2MiB",
      emulatorPin: "8",
      numaCells: [
        { id: 0, cpuset: "0-1", memoryMiB: 1024 },
        { id: 1, cpuset: "2-3", memoryMiB: 1024 },
      ],
      vcpuPins: [{ vcpu: 0, cpuset: "0" }, { vcpu: 1, cpuset: "1" }],
    });
  });

  it("rejects topology mismatches and duplicate or out-of-range pins", () => {
    const form = placementFormFrom(undefined);
    expect(buildPlacement({ ...form, numaCells: [{ id: "0", cpuset: "0-3", memoryMiB: "512" }] }, 4, 2048).error).toContain("内存总和");
    expect(buildPlacement({ ...form, numaCells: [{ id: "0", cpuset: "0-1", memoryMiB: "1024" }, { id: "1", cpuset: "1-2", memoryMiB: "1024" }] }, 4, 2048).error).toContain("重叠");
    expect(buildPlacement({ ...form, numaCells: [{ id: "0", cpuset: "0-1", memoryMiB: "2048" }] }, 4, 2048).error).toContain("CPU 总数");
    expect(buildPlacement({ ...form, vcpuPins: [{ vcpu: "4", cpuset: "0" }] }, 4, 2048).error).toContain("0 到 3");
    expect(buildPlacement({ ...form, vcpuPins: [{ vcpu: "0", cpuset: "0" }, { vcpu: "0", cpuset: "1" }] }, 4, 2048).error).toContain("重复");
  });
});
