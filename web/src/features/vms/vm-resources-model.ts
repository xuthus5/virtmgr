import type { Role, VM, VMResourcesInput, VMState } from "@/api/types";

export const minResourceMemoryMiB = 128;
export const maxResourceMemoryMiB = 1024 * 1024;
export const maxResourceVCPUs = 255;

export function isVMActiveResources(state: VMState) {
  return state === "running" || state === "blocked" || state === "paused" || state === "suspended";
}

export function vmResourcesDisabledReason(role: Role | undefined, readOnly: boolean, vm: VM) {
  if (readOnly) return "系统当前处于只读模式";
  if (role !== "operator" && role !== "admin") return "当前账号没有虚拟机操作权限";
  if (!vm.persistent) return "临时域不能修改 CPU 或内存配置";
  if (vm.state === "shutting-down" || vm.state === "unknown") return "当前状态不允许修改 CPU 或内存配置";
  return "";
}

export function activeResourceLimits(vm: VM) {
  return {
    vcpus: Math.max(1, vm.maxVcpus || vm.vcpus),
    memoryMiB: Math.max(1, Math.floor((vm.maxMemoryBytes || vm.memoryBytes) / (1024 * 1024))),
  };
}

export function validateVMResources(vm: VM, vcpusText: string, memoryMiBText: string): { input?: VMResourcesInput; error?: string } {
  const vcpusRaw = vcpusText.trim();
  const memoryRaw = memoryMiBText.trim();
  if (!/^\d+$/.test(vcpusRaw)) return { error: "vCPU 必须是正整数。" };
  if (!/^\d+$/.test(memoryRaw)) return { error: "内存必须是正整数（MiB）。" };
  const vcpus = Number(vcpusRaw);
  const memoryMiB = Number(memoryRaw);
  if (!Number.isSafeInteger(vcpus) || vcpus < 1 || vcpus > maxResourceVCPUs) {
    return { error: `vCPU 必须在 1 到 ${maxResourceVCPUs} 之间。` };
  }
  if (!Number.isSafeInteger(memoryMiB) || memoryMiB < minResourceMemoryMiB || memoryMiB > maxResourceMemoryMiB) {
    return { error: `内存必须在 ${minResourceMemoryMiB} 到 ${maxResourceMemoryMiB} MiB 之间。` };
  }
  if (isVMActiveResources(vm.state)) {
    const limits = activeResourceLimits(vm);
    if (vcpus > limits.vcpus) return { error: `运行中的虚拟机最多支持 ${limits.vcpus} 个 vCPU，请先关机后再扩展。` };
    if (memoryMiB > limits.memoryMiB) return { error: `运行中的虚拟机最多支持 ${limits.memoryMiB} MiB 内存，请先关机后再扩展。` };
  }
  return { input: { vcpus, memoryMiB } };
}
