import type { Role, VMConfigVersionSource } from "@/api/types";

export const vmConfigVersionSourceLabels: Record<VMConfigVersionSource, string> = {
  manual_xml: "手动编辑 XML",
  resources: "CPU / 内存调整",
  passthrough: "PCI / USB 直通",
  snapshot_revert: "快照回滚",
  external: "外部工具修改",
  create: "创建虚拟机",
  clone: "克隆虚拟机",
  backup_restore: "备份恢复",
  rollback: "XML 版本回滚",
  pre_rollback: "回滚前自动存档",
  disk_resize: "磁盘扩容",
  baseline: "初始基线",
};

export function configVersionSourceLabel(source: VMConfigVersionSource) {
  return vmConfigVersionSourceLabels[source] ?? source;
}

export function shortConfigVersionSHA(value: string) {
  if (!value) return "-";
  return value.length > 16 ? `${value.slice(0, 12)}…${value.slice(-4)}` : value;
}

export function configVersionRollbackReason(input: {
  role: Role | undefined;
  readOnly: boolean;
  isCurrent: boolean;
  protected?: boolean;
}) {
  if (input.role !== "admin") return "仅管理员可以回滚域 XML 版本";
  if (input.readOnly) return "系统当前处于只读模式";
  if (input.isCurrent) return "该版本已经是当前持久配置";
  return "";
}

export function configVersionFilename(vmName: string, versionNo: number) {
  const safeName = vmName.replace(/[^A-Za-z0-9_.-]+/g, "_").replace(/^[._-]+|[._-]+$/g, "") || "vm";
  return `${safeName}-config-v${versionNo}.xml`;
}
