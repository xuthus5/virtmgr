import { describe, expect, it } from "vitest";
import { canEditVMConfigXML, maxVMXMLBytes, validateVMXMLDraft } from "@/features/vms/vm-xml-edit-model";

describe("VM XML edit model", () => {
  it("allows only admins to edit persistent config XML in writable mode", () => {
    expect(canEditVMConfigXML("admin", false, true, "config")).toBe(true);
    expect(canEditVMConfigXML("operator", false, true, "config")).toBe(false);
    expect(canEditVMConfigXML("admin", true, true, "config")).toBe(false);
    expect(canEditVMConfigXML("admin", false, false, "config")).toBe(false);
    expect(canEditVMConfigXML("admin", false, true, "live")).toBe(false);
  });

  it("validates empty and oversized drafts", () => {
    expect(validateVMXMLDraft(" ")).toContain("不能为空");
    expect(validateVMXMLDraft("<domain/>")).toBe("");
    expect(validateVMXMLDraft("x".repeat(maxVMXMLBytes + 1))).toContain("512 KiB");
  });
});
