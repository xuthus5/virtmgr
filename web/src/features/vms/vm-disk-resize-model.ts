import type { Disk, Role, VM, VMDiskResizeInput } from "@/api/types";

const bytesPerGiB = 1024 ** 3;

export const minDiskCapacityBytes = 1024 ** 2;
export const maxDiskCapacityBytes = 1024 ** 5;

export function vmDiskResizeDisabledReason(role: Role | undefined, readOnly: boolean, vm: VM, disk: Disk) {
  if (disk.device !== "disk") return "只支持数据磁盘扩容";
  if (readOnly) return "系统当前处于只读模式";
  if (role !== "operator" && role !== "admin") return "当前账号没有虚拟机操作权限";
  if (!vm.persistent) return "临时域不能扩容磁盘";
  if (vm.state === "shutting-down" || vm.state === "unknown") return "当前状态不允许扩容磁盘";
  if (disk.readOnly) return "只读磁盘不能扩容";
  if (!disk.source) return "磁盘没有可识别的数据源";
  if (!Number.isFinite(disk.capacityBytes) || disk.capacityBytes <= 0) return "无法读取该磁盘的当前容量，暂时不能安全扩容";
  return "";
}

export function defaultDiskCapacityGiB(disk: Disk) {
  return Math.max(Math.ceil(disk.capacityBytes / bytesPerGiB) + 10, 1);
}

export function validateDiskCapacity(disk: Disk, capacityGiBText: string): { input?: VMDiskResizeInput; error?: string } {
  const raw = capacityGiBText.trim();
  if (!/^\d+(?:\.\d{1,3})?$/.test(raw)) return { error: "新容量必须是大于 0 的数字（GiB）。" };
  const capacityGiB = Number(raw);
  const capacityBytes = Math.round(capacityGiB * bytesPerGiB);
  if (!Number.isSafeInteger(capacityBytes) || capacityBytes < minDiskCapacityBytes) {
    return { error: "新容量至少为 1 MiB。" };
  }
  if (capacityBytes <= disk.capacityBytes) {
    return { error: "只支持扩容，新容量必须大于当前容量。" };
  }
  if (capacityBytes > maxDiskCapacityBytes) {
    return { error: "单块磁盘容量不能超过 1 PiB。" };
  }
  return { input: { capacityBytes } };
}
