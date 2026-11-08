import type { Role, VMState } from "@/api/types";

const backupNamePattern = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/;
const targetPoolPattern = /^[A-Za-z0-9][A-Za-z0-9_.-]{1,63}$/;

export function canBackupVM(role: Role | undefined, readOnly: boolean, persistent: boolean, state: VMState) {
  return (role === "operator" || role === "admin")
    && !readOnly
    && persistent
    && (state === "shutoff" || state === "crashed");
}

export function defaultVMBackupName(vmName: string, now = new Date()) {
  const timestamp = now.toISOString().replace(/[-:]/g, "").replace(/\.\d{3}Z$/, "Z");
  const suffix = `-backup-${timestamp}`;
  const base = vmName
    .trim()
    .replace(/[^A-Za-z0-9_.-]+/g, "-")
    .replace(/^[^A-Za-z0-9]+|[^A-Za-z0-9]+$/g, "")
    .slice(0, Math.max(1, 64 - suffix.length)) || "vm";
  return `${base}${suffix}`;
}

export function validateVMBackupDraft(name: string, targetPool: string) {
  const cleanName = name.trim();
  const cleanPool = targetPool.trim();
  if (!backupNamePattern.test(cleanName)) {
    return "备份名称需以字母或数字开头，仅允许字母、数字、点、下划线和短横线，最长 64 个字符。";
  }
  if (!targetPoolPattern.test(cleanPool)) {
    return "请选择有效的目标存储池。";
  }
  return "";
}
