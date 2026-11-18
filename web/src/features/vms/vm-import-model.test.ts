import { describe, expect, it } from "vitest";
import { buildVMImportInput, importDiskCapacityError, importFilenameError, importVolumeFormat, isSupportedImportVolume, vmImportTargetDiskName } from "@/features/vms/vm-import-model";

describe("vm import model", () => {
  it("accepts matching qcow2, raw and ISO volumes", () => {
    expect(importVolumeFormat({ name: "base.qcow2", format: "qcow2" })).toBe("qcow2");
    expect(importVolumeFormat({ name: "base.raw", format: "raw" })).toBe("raw");
    expect(importVolumeFormat({ name: "install.iso", format: "raw" })).toBe("iso");
    expect(isSupportedImportVolume({ name: "install.iso", format: "raw" })).toBe(true);
    expect(isSupportedImportVolume({ name: "install.iso", format: "qcow2" })).toBe(false);
    expect(isSupportedImportVolume({ name: "base.qcow2", format: "raw" })).toBe(false);
    expect(isSupportedImportVolume({ name: "base.vmdk", format: "qcow2" })).toBe(false);
  });

  it("validates selected filenames and derives the copied disk name", () => {
    expect(importFilenameError("")).toContain("请选择");
    expect(importFilenameError("disk.vmdk")).toContain("仅支持");
    expect(importFilenameError("disk.QCOW2")).toBe("");
    expect(importFilenameError("install.ISO")).toBe("");
    expect(vmImportTargetDiskName(" web-01 ", "qcow2")).toBe("web-01-disk.qcow2");
    expect(vmImportTargetDiskName(" web-01 ", "iso")).toBe("web-01-disk.qcow2");
  });

  it("validates ISO system disk capacity", () => {
    expect(importDiskCapacityError("")).toContain("有效");
    expect(importDiskCapacityError("0")).toContain("有效");
    expect(importDiskCapacityError("0.5")).toContain("1 到 16384");
    expect(importDiskCapacityError("16385")).toContain("1 到 16384");
    expect(importDiskCapacityError("20")).toBe("");
  });

  it("maps form values to the API payload", () => {
    expect(buildVMImportInput({
      name: " web-import ", architecture: "x86_64", machine: "q35", sourcePool: "default", sourceVolume: "base.qcow2", targetPool: "fast",
      vcpus: "4", memoryMiB: "4096", diskCapacityGiB: "30", diskBus: "virtio", network: "default", nicModel: "e1000",
      firmware: "uefi", secureBoot: true, tpm: true, guestAgent: true, autostart: true, start: false,
    })).toEqual({
      name: "web-import", architecture: "x86_64", machine: "q35", sourcePool: "default", sourceVolume: "base.qcow2", targetPool: "fast",
      vcpus: 4, memoryMiB: 4096, diskCapacityBytes: 30 * 1024 ** 3, diskBus: "virtio", network: "default", nicModel: "e1000",
      firmware: "uefi", secureBoot: true, tpm: true, guestAgent: true, autostart: true, start: false,
    });
  });
});
