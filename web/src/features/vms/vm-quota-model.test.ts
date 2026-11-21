import { describe, expect, it } from "vitest";
import { emptyQuotaForm, quotaFormFrom, quotaSummary, validateVMQuota } from "@/features/vms/vm-quota-model";

describe("vm quota model", () => {
  it("accepts an empty quota as a clear operation", () => {
    const result = validateVMQuota(emptyQuotaForm, 2048);
    expect(result.error).toBeUndefined();
    expect(result.input).toEqual({
      cpuShares: 0, cpuQuota: 0, cpuPeriod: 0,
      memoryHardLimitMiB: 0, memorySoftLimitMiB: 0,
    });
  });

  it("defaults the CPU period and validates memory relationships", () => {
    const result = validateVMQuota({
      ...emptyQuotaForm,
      cpuShares: "1024",
      cpuQuota: "50000",
      memoryHardLimitMiB: "4096",
      memorySoftLimitMiB: "3072",
    }, 2048);
    expect(result.input?.cpuPeriod).toBe(100000);
    expect(result.error).toBeUndefined();
    expect(validateVMQuota({ ...emptyQuotaForm, memoryHardLimitMiB: "1024" }, 2048).error).toContain("不能低于");
    expect(validateVMQuota({ ...emptyQuotaForm, cpuPeriod: "100000" }, 2048).error).toContain("CPU quota");
    expect(validateVMQuota({ ...emptyQuotaForm, memoryHardLimitMiB: "4096", memorySoftLimitMiB: "8192" }, 2048).error).toContain("不能超过硬限制");
    expect(validateVMQuota({ ...emptyQuotaForm, cpuQuota: "9007199254740992" }, 2048).error).toContain("安全整数");
  });

  it("formats existing quota values", () => {
    const form = quotaFormFrom({
      cpuShares: 1024, cpuQuota: 50000, cpuPeriod: 100000,
      memoryHardLimitMiB: 4096, memorySoftLimitMiB: 0,
    });
    expect(form.memoryHardLimitMiB).toBe("4096");
    expect(quotaSummary({ cpuShares: 1024, cpuQuota: 50000, cpuPeriod: 100000, memoryHardLimitMiB: 4096, memorySoftLimitMiB: 0 })).toContain("硬限制 4096 MiB");
  });
});
