import type { SnapshotPolicy, SnapshotPolicyInput } from "@/api/types";

export const snapshotPolicyNamePattern = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$/;
export const minSnapshotPolicyInterval = 5;
export const maxSnapshotPolicyInterval = 60 * 24 * 7;
export const minSnapshotPolicyRetention = 1;
export const maxSnapshotPolicyRetention = 50;

export type SnapshotPolicyFormValues = {
  name: string;
  intervalMinutes: string;
  retention: string;
  enabled: boolean;
};

export function initialSnapshotPolicyForm(): SnapshotPolicyFormValues {
  return { name: "", intervalMinutes: "60", retention: "3", enabled: true };
}

export function snapshotPolicyFormFromPolicy(policy: SnapshotPolicy): SnapshotPolicyFormValues {
  return {
    name: policy.name,
    intervalMinutes: String(policy.intervalMinutes),
    retention: String(policy.retention),
    enabled: policy.enabled,
  };
}

export function snapshotPolicyInputFromForm(values: SnapshotPolicyFormValues): { input?: SnapshotPolicyInput; error: string } {
  const name = values.name.trim();
  if (!snapshotPolicyNamePattern.test(name)) {
    return {
      error: "策略名称需以字母或数字开头，仅允许字母、数字、点、下划线和短横线，最长 32 个字符。",
    };
  }
  const intervalMinutes = Number(values.intervalMinutes);
  if (!Number.isInteger(intervalMinutes) || intervalMinutes < minSnapshotPolicyInterval || intervalMinutes > maxSnapshotPolicyInterval) {
    return { error: `间隔必须是 ${minSnapshotPolicyInterval} 到 ${maxSnapshotPolicyInterval} 分钟之间的整数。` };
  }
  const retention = Number(values.retention);
  if (!Number.isInteger(retention) || retention < minSnapshotPolicyRetention || retention > maxSnapshotPolicyRetention) {
    return { error: `保留数量必须是 ${minSnapshotPolicyRetention} 到 ${maxSnapshotPolicyRetention} 之间的整数。` };
  }
  return { input: { name, intervalMinutes, retention, enabled: values.enabled }, error: "" };
}

export function formatSnapshotPolicyTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}
