import type { StorageVolume, VMDiskBus, VMFirmware, VMImportInput, VMNICModel } from "@/api/types";

export type VMImportFormat = "qcow2" | "raw" | "iso";

const gibibyte = 1024 ** 3;

export function importVolumeFormat(volume: Pick<StorageVolume, "name" | "format">): VMImportFormat | null {
  const format = (volume.format ?? "").toLowerCase();
  const extension = volume.name.toLowerCase().match(/\.(qcow2|raw|iso)$/)?.[1];
  if (extension === "iso" && (format === "raw" || format === "iso")) return "iso";
  if ((format === "qcow2" || format === "raw") && extension === format) return format;
  return null;
}

export function isSupportedImportVolume(volume: Pick<StorageVolume, "name" | "format">) {
  return importVolumeFormat(volume) !== null;
}

export function importFilenameError(filename: string) {
  if (!filename.trim()) return "请选择 QCOW2、RAW 或 ISO 文件";
  return /\.(qcow2|raw|iso)$/i.test(filename) ? "" : "仅支持 .qcow2、.raw 和 .iso 文件";
}

export function vmImportTargetDiskName(vmName: string, format: VMImportFormat) {
  const diskFormat = format === "iso" ? "qcow2" : format;
  return `${vmName.trim() || "<name>"}-disk.${diskFormat}`;
}

export function importDiskCapacityError(value: string) {
  const gib = Number(value);
  if (!Number.isFinite(gib) || gib <= 0) return "请输入有效的系统盘容量";
  if (gib < 1 || gib > 16384) return "系统盘容量需在 1 到 16384 GiB 之间";
  return "";
}

export type VMImportFormValues = {
  name: string;
  architecture: string;
  machine: string;
  sourcePool: string;
  sourceVolume: string;
  targetPool: string;
  vcpus: string;
  memoryMiB: string;
  diskCapacityGiB: string;
  diskBus: VMDiskBus;
  network: string;
  nicModel: VMNICModel;
  firmware: VMFirmware;
  secureBoot: boolean;
  tpm: boolean;
  guestAgent: boolean;
  autostart: boolean;
  start: boolean;
};

export function buildVMImportInput(values: VMImportFormValues): VMImportInput {
  const capacityGiB = Number(values.diskCapacityGiB);
  return {
    name: values.name.trim(),
    ...(values.architecture ? { architecture: values.architecture } : {}),
    ...(values.machine ? { machine: values.machine } : {}),
    sourcePool: values.sourcePool,
    sourceVolume: values.sourceVolume,
    targetPool: values.targetPool,
    vcpus: Number(values.vcpus),
    memoryMiB: Number(values.memoryMiB),
    diskCapacityBytes: Number.isFinite(capacityGiB) ? Math.round(capacityGiB * gibibyte) : 0,
    diskBus: values.diskBus,
    network: values.network,
    nicModel: values.nicModel,
    firmware: values.firmware,
    secureBoot: values.secureBoot,
    tpm: values.tpm,
    guestAgent: values.guestAgent,
    autostart: values.autostart,
    start: values.start,
  };
}
