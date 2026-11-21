import type { VM, VMHardwareInput, VMWatchdogAction, VMWatchdogModel } from "@/api/types";

export type HardwareFormState = {
  rng: boolean;
  watchdogEnabled: boolean;
  watchdogModel: VMWatchdogModel;
  watchdogAction: VMWatchdogAction;
};

export const emptyHardwareForm: HardwareFormState = {
  rng: false,
  watchdogEnabled: false,
  watchdogModel: "i6300esb",
  watchdogAction: "reset",
};

export function hardwareFormFrom(vm: Pick<VM, "rng" | "watchdog">): HardwareFormState {
  return {
    rng: Boolean(vm.rng),
    watchdogEnabled: Boolean(vm.watchdog),
    watchdogModel: vm.watchdog?.model ?? "i6300esb",
    watchdogAction: vm.watchdog?.action ?? "reset",
  };
}

export function buildVMHardware(form: HardwareFormState): VMHardwareInput {
  return {
    rng: form.rng,
    ...(form.watchdogEnabled ? { watchdog: { model: form.watchdogModel, action: form.watchdogAction } } : {}),
  };
}

export function hardwareSummary(hardware: VMHardwareInput) {
  const parts: string[] = [];
  if (hardware.rng) parts.push("RNG");
  if (hardware.watchdog) parts.push(`watchdog ${hardware.watchdog.model}/${hardware.watchdog.action}`);
  return parts.length ? parts.join(" · ") : "未启用";
}
