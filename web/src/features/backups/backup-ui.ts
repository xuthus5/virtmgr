import { ApiError } from "@/api/client";
import type { BackupStatus, Role } from "@/api/types";

export type BackupTab = "backups" | "schedules" | "repositories";
export type BackupNotice = { tone: "success" | "error"; text: string };

export const backupPageSize = 25;

export const backupStatusMeta: Record<BackupStatus, { label: string; tone: "success" | "warning" | "destructive" | "secondary" | "default" }> = {
  running: { label: "备份中", tone: "warning" },
  completed: { label: "已完成", tone: "success" },
  failed: { label: "失败", tone: "destructive" },
  interrupted: { label: "已中断", tone: "secondary" },
  deleting: { label: "删除中", tone: "warning" },
  delete_failed: { label: "删除失败", tone: "destructive" },
};

export const verifyMeta = {
  unknown: { label: "未校验", tone: "secondary" as const },
  verified: { label: "校验通过", tone: "success" as const },
  failed: { label: "校验失败", tone: "destructive" as const },
};

const dateTime = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false,
});

export function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : dateTime.format(date);
}

export function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size >= 100 || index === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

export function errorText(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

export function backupName(vmName: string) {
  const stamp = new Date().toISOString().replace(/\D/g, "").slice(0, 14);
  return `${vmName}-backup-${stamp}`.slice(0, 64);
}

export function actionReason(role: Role | undefined, readOnly: boolean, action: "create" | "verify" | "restore" | "delete" | "admin") {
  if (readOnly && action !== "verify") return "系统当前处于只读模式";
  if (!role) return "正在读取账号权限";
  if (action === "admin" && role !== "admin") return "仅管理员可执行此操作";
  if (action === "restore" && role !== "admin") return "仅管理员可将备份恢复为新虚拟机";
  if (action === "delete" && role !== "admin") return "仅管理员可删除备份";
  if ((action === "create" || action === "verify") && role === "viewer") return "Viewer 账号只能查看备份";
  return "";
}
