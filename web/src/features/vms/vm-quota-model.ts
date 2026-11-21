import type { VMResourceQuota } from "@/api/types";

export type QuotaFormState = {
  cpuShares: string;
  cpuQuota: string;
  cpuPeriod: string;
  memoryHardLimitMiB: string;
  memorySoftLimitMiB: string;
};

export const emptyQuotaForm: QuotaFormState = {
  cpuShares: "",
  cpuQuota: "",
  cpuPeriod: "",
  memoryHardLimitMiB: "",
  memorySoftLimitMiB: "",
};

export function quotaFormFrom(quota?: VMResourceQuota): QuotaFormState {
  if (!quota) return { ...emptyQuotaForm };
  return {
    cpuShares: quota.cpuShares ? String(quota.cpuShares) : "",
    cpuQuota: quota.cpuQuota ? String(quota.cpuQuota) : "",
    cpuPeriod: quota.cpuPeriod ? String(quota.cpuPeriod) : "",
    memoryHardLimitMiB: quota.memoryHardLimitMiB ? String(quota.memoryHardLimitMiB) : "",
    memorySoftLimitMiB: quota.memorySoftLimitMiB ? String(quota.memorySoftLimitMiB) : "",
  };
}

export function quotaHasValues(form: QuotaFormState) {
  return Object.values(form).some((value) => value.trim() !== "");
}

export function validateVMQuota(form: QuotaFormState, memoryMiB: number): { input?: VMResourceQuota; error?: string } {
  const values = Object.fromEntries(Object.entries(form).map(([key, value]) => [key, value.trim()])) as QuotaFormState;
  for (const [name, value] of Object.entries(values)) {
    if (value && (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value)))) {
      return { error: `${quotaFieldLabel(name)}必须是非负安全整数。` };
    }
  }
  const cpuShares = numberValue(values.cpuShares);
  const cpuQuota = numberValue(values.cpuQuota);
  let cpuPeriod = numberValue(values.cpuPeriod);
  const hard = numberValue(values.memoryHardLimitMiB);
  const soft = numberValue(values.memorySoftLimitMiB);
  if (cpuShares !== 0 && (cpuShares < 2 || cpuShares > 262_144)) return { error: "CPU shares 必须在 2 到 262144 之间。" };
  if (cpuQuota === 0 && cpuPeriod !== 0) return { error: "设置 CPU period 时必须同时设置 CPU quota。" };
  if (cpuQuota > 0 && cpuPeriod === 0) cpuPeriod = 100_000;
  if (cpuPeriod !== 0 && (cpuPeriod < 1_000 || cpuPeriod > 1_000_000)) return { error: "CPU period 必须在 1000 到 1000000 微秒之间。" };
  if (hard > 0 && hard < memoryMiB) return { error: `内存硬限制不能低于当前 ${memoryMiB} MiB。` };
  if (hard > 0 && soft > hard) return { error: "内存软限制不能超过硬限制。" };
  return {
    input: {
      cpuShares,
      cpuQuota,
      cpuPeriod,
      memoryHardLimitMiB: hard,
      memorySoftLimitMiB: soft,
    },
  };
}

export function quotaSummary(quota?: VMResourceQuota) {
  if (!quota) return "未设置";
  const parts: string[] = [];
  if (quota.cpuShares) parts.push(`shares ${quota.cpuShares}`);
  if (quota.cpuQuota) parts.push(`${quota.cpuQuota}/${quota.cpuPeriod || 100_000} µs`);
  if (quota.memoryHardLimitMiB) parts.push(`硬限制 ${quota.memoryHardLimitMiB} MiB`);
  if (quota.memorySoftLimitMiB) parts.push(`软限制 ${quota.memorySoftLimitMiB} MiB`);
  return parts.length ? parts.join(" · ") : "未设置";
}

function numberValue(value: string) {
  return Number(value || "0");
}

function quotaFieldLabel(name: string) {
  switch (name) {
    case "cpuShares": return "CPU shares ";
    case "cpuQuota": return "CPU quota ";
    case "cpuPeriod": return "CPU period ";
    case "memoryHardLimitMiB": return "内存硬限制 ";
    case "memorySoftLimitMiB": return "内存软限制 ";
    default: return "资源配额 ";
  }
}
