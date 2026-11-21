import { describe, expect, it } from "vitest";
import { buildVMHardware, emptyHardwareForm, hardwareFormFrom, hardwareSummary } from "@/features/vms/vm-hardware-model";

describe("vm hardware model", () => {
  it("builds optional RNG and watchdog configuration", () => {
    expect(buildVMHardware(emptyHardwareForm)).toEqual({ rng: false });
    expect(buildVMHardware({ ...emptyHardwareForm, rng: true, watchdogEnabled: true })).toEqual({
      rng: true,
      watchdog: { model: "i6300esb", action: "reset" },
    });
  });

  it("restores values from a VM and formats a summary", () => {
    const form = hardwareFormFrom({ rng: true, watchdog: { model: "ib700", action: "poweroff" } });
    expect(form.watchdogEnabled).toBe(true);
    expect(hardwareSummary(buildVMHardware(form))).toBe("RNG · watchdog ib700/poweroff");
    expect(hardwareSummary({ rng: false })).toBe("未启用");
  });
});
