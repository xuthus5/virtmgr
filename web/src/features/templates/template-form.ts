import type { VMFirmware, VMTemplate, VMTemplateInput } from "@/api/types";

export type TemplateFormState = {
  name: string;
  description: string;
  vcpus: string;
  memoryMiB: string;
  diskFormat: string;
  network: string;
  isoPath: string;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit: boolean;
  cloudInitUserData: string;
  cloudInitNetworkConfig: string;
  cloudInitVendorData: string;
  autostart: boolean;
  start: boolean;
};

export const initialTemplateForm: TemplateFormState = {
  name: "",
  description: "",
  vcpus: "2",
  memoryMiB: "2048",
  diskFormat: "qcow2",
  network: "default",
  isoPath: "",
  firmware: "bios",
  secureBoot: false,
  cloudInit: false,
  cloudInitUserData: "",
  cloudInitNetworkConfig: "",
  cloudInitVendorData: "",
  autostart: false,
  start: false,
};

export function templateToForm(template: VMTemplate): TemplateFormState {
  return {
    name: template.name,
    description: template.description,
    vcpus: String(template.vcpus),
    memoryMiB: String(template.memoryMiB),
    diskFormat: template.diskFormat,
    network: template.network,
    isoPath: template.isoPath ?? "",
    firmware: template.firmware,
    secureBoot: template.secureBoot,
    cloudInit: template.cloudInit,
    cloudInitUserData: template.cloudInitUserData ?? "",
    cloudInitNetworkConfig: template.cloudInitNetworkConfig ?? "",
    cloudInitVendorData: template.cloudInitVendorData ?? "",
    autostart: template.autostart,
    start: template.start,
  };
}

export function formToTemplateInput(form: TemplateFormState): VMTemplateInput {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    vcpus: Number(form.vcpus),
    memoryMiB: Number(form.memoryMiB),
    diskFormat: form.diskFormat,
    network: form.network.trim(),
    isoPath: form.isoPath.trim(),
    firmware: form.firmware,
    secureBoot: form.secureBoot,
    cloudInit: form.cloudInit,
    cloudInitUserData: form.cloudInit ? form.cloudInitUserData : undefined,
    cloudInitNetworkConfig: form.cloudInit ? form.cloudInitNetworkConfig : undefined,
    cloudInitVendorData: form.cloudInit ? form.cloudInitVendorData : undefined,
    autostart: form.autostart,
    start: form.start,
  };
}

export type TemplateVMCreateDefaults = {
  vcpus: number;
  memoryMiB: number;
  diskFormat: string;
  network: string;
  isoPath: string;
  firmware: VMFirmware;
  secureBoot: boolean;
  cloudInit: boolean;
  autostart: boolean;
  start: boolean;
  cloudInitUserData: string;
  cloudInitNetworkConfig: string;
  cloudInitVendorData: string;
};

export function templateVMCreateDefaults(template: VMTemplate): TemplateVMCreateDefaults {
  return {
    vcpus: template.vcpus,
    memoryMiB: template.memoryMiB,
    diskFormat: template.diskFormat,
    network: template.network,
    isoPath: template.isoPath ?? "",
    firmware: template.firmware,
    secureBoot: template.secureBoot,
    cloudInit: template.cloudInit,
    cloudInitUserData: template.cloudInit ? template.cloudInitUserData ?? "" : "",
    cloudInitNetworkConfig: template.cloudInit ? template.cloudInitNetworkConfig ?? "" : "",
    cloudInitVendorData: template.cloudInit ? template.cloudInitVendorData ?? "" : "",
    autostart: template.autostart,
    start: template.start,
  };
}
