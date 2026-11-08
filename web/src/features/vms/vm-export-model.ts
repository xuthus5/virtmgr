import type { VMExport } from "@/api/types";

export function buildVMExportFilename(name: string) {
  const sanitized = name
    .trim()
    .replace(/[^A-Za-z0-9._-]+/g, "_")
    .replace(/^[._-]+|[._-]+$/g, "");
  return `${sanitized || "vm"}-config-export.json`;
}

export function serializeVMExport(exported: VMExport) {
  return `${JSON.stringify(exported, null, 2)}\n`;
}

export function downloadVMExport(exported: VMExport) {
  const blob = new Blob([serializeVMExport(exported)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = buildVMExportFilename(exported.vm.name);
  link.rel = "noopener";
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}
