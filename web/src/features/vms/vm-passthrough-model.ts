import type { HostDevice, PassthroughDevice } from "@/api/types";

export const passthroughTypeLabels: Record<string, string> = {
  pci: "PCI",
  usb: "USB",
};

export function isPassthroughHostDevice(device: HostDevice) {
  return device.type === "pci" || device.type === "usb_device";
}

export function attachableHostDevices(devices: HostDevice[], attached: PassthroughDevice[]) {
  const attachedNames = new Set(attached.map((device) => device.name));
  return devices.filter((device) => isPassthroughHostDevice(device) && !attachedNames.has(device.name));
}

export function hostDeviceLabel(device: HostDevice) {
  const type = device.type === "usb_device" ? "USB" : device.type.toUpperCase();
  const product = device.product || device.productId || device.name;
  return `${type} · ${product}${device.address ? ` · ${device.address}` : ""}`;
}

export function passthroughDeviceName(device: PassthroughDevice) {
  return device.product || device.productId || device.name;
}
