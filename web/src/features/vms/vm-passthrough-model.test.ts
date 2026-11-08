import { describe, expect, it } from "vitest";
import type { HostDevice, PassthroughDevice } from "@/api/types";
import { attachableHostDevices, hostDeviceLabel } from "@/features/vms/vm-passthrough-model";

const pci: HostDevice = {
  name: "pci_0000_3b_00_0", type: "pci", product: "GA102 [GeForce RTX 3090]",
  address: "0000:3b:00.0", active: true, persistent: true, capabilities: ["pci"],
};
const usb: HostDevice = {
  name: "usb_device_046d_c52b_1_2", type: "usb_device", product: "Unifying Receiver",
  address: "bus 1 · device 2", active: true, persistent: true, capabilities: ["usb_device"],
};
const net: HostDevice = {
  name: "net_enp2s0", type: "net", address: "enp2s0", active: true, persistent: true, capabilities: ["net"],
};
const attached: PassthroughDevice[] = [{ name: pci.name, type: "pci" }];

describe("vm passthrough model", () => {
  it("only offers unattached PCI and USB devices", () => {
    expect(attachableHostDevices([pci, usb, net], attached)).toEqual([usb]);
  });

  it("formats host device options with type, product and address", () => {
    expect(hostDeviceLabel(usb)).toBe("USB · Unifying Receiver · bus 1 · device 2");
    expect(hostDeviceLabel(pci)).toContain("PCI · GA102 [GeForce RTX 3090]");
  });
});
