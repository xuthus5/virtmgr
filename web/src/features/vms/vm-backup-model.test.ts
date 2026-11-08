import { describe, expect, it } from "vitest";
import { canBackupVM, defaultVMBackupName, validateVMBackupDraft } from "@/features/vms/vm-backup-model";

describe("VM backup model", () => {
  it("only enables cold backups for persistent VMs with write permission", () => {
    expect(canBackupVM("operator", false, true, "shutoff")).toBe(true);
    expect(canBackupVM("admin", false, true, "crashed")).toBe(true);
    expect(canBackupVM("operator", true, true, "shutoff")).toBe(false);
    expect(canBackupVM("viewer", false, true, "shutoff")).toBe(false);
    expect(canBackupVM("operator", false, false, "shutoff")).toBe(false);
    expect(canBackupVM("operator", false, true, "running")).toBe(false);
  });

  it("builds a bounded default name with a UTC timestamp", () => {
    expect(defaultVMBackupName("demo-legacy-01", new Date("2026-11-08T10:20:30Z"))).toBe("demo-legacy-01-backup-20261108T102030Z");
    const longName = defaultVMBackupName("a".repeat(100), new Date("2026-11-08T10:20:30Z"));
    expect(longName.length).toBeLessThanOrEqual(64);
    expect(longName.endsWith("-backup-20261108T102030Z")).toBe(true);
    expect(defaultVMBackupName(" ../../bad vm ", new Date("2026-11-08T10:20:30Z"))).toBe("bad-vm-backup-20261108T102030Z");
  });

  it("validates backup names and target pools", () => {
    expect(validateVMBackupDraft("api-backup-01", "default")).toBe("");
    expect(validateVMBackupDraft("", "default")).not.toBe("");
    expect(validateVMBackupDraft("../backup", "default")).not.toBe("");
    expect(validateVMBackupDraft("api-backup", "")).not.toBe("");
    expect(validateVMBackupDraft("api-backup", "bad/pool")).not.toBe("");
  });
});
