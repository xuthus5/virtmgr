import { describe, expect, it } from "vitest";
import type { VMExport } from "@/api/types";
import { buildVMExportFilename, serializeVMExport } from "@/features/vms/vm-export-model";

describe("VM export model", () => {
  it("builds safe download filenames from VM names", () => {
    expect(buildVMExportFilename("api-01")).toBe("api-01-config-export.json");
    expect(buildVMExportFilename(" ../../web vm ")).toBe("web_vm-config-export.json");
    expect(buildVMExportFilename("...")).toBe("vm-config-export.json");
  });

  it("serializes the export as readable JSON with a trailing newline", () => {
    const exported = {
      formatVersion: "virtmgr.vm-export/v1",
      exportedAt: "2026-11-08T00:00:00Z",
      vm: { name: "api-01" },
    } as unknown as VMExport;
    const serialized = serializeVMExport(exported);
    expect(serialized.endsWith("\n")).toBe(true);
    expect(JSON.parse(serialized)).toMatchObject({
      formatVersion: "virtmgr.vm-export/v1",
      vm: { name: "api-01" },
    });
    expect(serialized).toContain('\n  "formatVersion"');
  });
});
