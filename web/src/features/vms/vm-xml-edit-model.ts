import type { Role, VMXMLFormat } from "@/api/types";

export const maxVMXMLBytes = 512 * 1024;

export function canEditVMConfigXML(role: Role | undefined, readOnly: boolean, persistent: boolean, format: VMXMLFormat) {
  return role === "admin" && !readOnly && persistent && format === "config";
}

export function validateVMXMLDraft(xmlText: string) {
  if (!xmlText.trim()) return "域 XML 不能为空。";
  if (new TextEncoder().encode(xmlText).length > maxVMXMLBytes) return "域 XML 不能超过 512 KiB。";
  return "";
}
