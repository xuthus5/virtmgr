import { describe, expect, it } from "vitest";
import type { VMTemplate } from "@/api/types";
import { formToTemplateInput, templateToForm, templateVMCreateDefaults } from "@/features/templates/template-form";

const template: VMTemplate = {
  id: 7,
  name: "secure-uefi",
  description: "UEFI baseline",
  vcpus: 4,
  memoryMiB: 4096,
  diskFormat: "qcow2",
  network: "default",
  isoPath: "/var/lib/libvirt/images/install.iso",
  firmware: "uefi",
  secureBoot: true,
  cloudInit: true,
  cloudInitUserData: "#cloud-config\n",
  autostart: true,
  start: false,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

describe("template form model", () => {
  it("round-trips editable template values", () => {
    const form = templateToForm(template);
    expect(form.vcpus).toBe("4");
    expect(form.memoryMiB).toBe("4096");
    expect(form.secureBoot).toBe(true);
    const input = formToTemplateInput(form);
    expect(input).toMatchObject({
      name: "secure-uefi", vcpus: 4, memoryMiB: 4096,
      firmware: "uefi", secureBoot: true, cloudInit: true,
    });
  });

  it("builds VM creation defaults without instance identity or disk path", () => {
    const defaults = templateVMCreateDefaults(template);
    expect(defaults).toMatchObject({
      vcpus: 4, memoryMiB: 4096, diskFormat: "qcow2", network: "default",
      firmware: "uefi", secureBoot: true, cloudInit: true, autostart: true, start: false,
    });
    expect(defaults.cloudInitUserData).toContain("#cloud-config");
    expect(defaults).not.toHaveProperty("name");
    expect(defaults).not.toHaveProperty("diskPath");
  });

  it("drops cloud-init content when the template disables it", () => {
    const input = formToTemplateInput({
      ...templateToForm(template), cloudInit: false,
    });
    expect(input.cloudInit).toBe(false);
    expect(input.cloudInitUserData).toBeUndefined();
  });
});
