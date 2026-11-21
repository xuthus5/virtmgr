import { describe, expect, it } from "vitest";
import type { StoragePool, StorageVolume, Task, VM, VMImage, VMTemplate, VirtualNetwork } from "@/api/types";
import { buildGlobalSearchItems } from "@/features/search/search-model";

const vm: VM = {
  uuid: "22222222-2222-4222-8222-222222222222",
  name: "demo-db-01",
  state: "running",
  stateReason: 0,
  persistent: true,
  autostart: true,
  vcpus: 8,
  maxVcpus: 16,
  memoryBytes: 16 * 1024 ** 3,
  maxMemoryBytes: 16 * 1024 ** 3,
  cpuTimeNanos: 0,
  firmware: "uefi",
  secureBoot: true,
  tpm: true,
  cloudInit: false,
  disks: [],
  interfaces: [{ type: "network", source: "default" }],
  graphics: [],
  passthrough: [],
  ipAddresses: ["192.168.122.102"],
  hostname: "demo-db-01",
  agentConnected: true,
  allowedActions: [],
};

const network: VirtualNetwork = {
  name: "default",
  uuid: "network-uuid",
  bridge: "virbr0",
  active: true,
  persistent: true,
  autostart: true,
  forwardMode: "nat",
  addresses: [{ family: "ipv4", address: "192.168.122.1", prefix: 24, dhcpStart: "192.168.122.2", dhcpEnd: "192.168.122.254" }],
};

const pool: StoragePool = {
  name: "iso-store",
  uuid: "pool-uuid",
  type: "dir",
  state: "active",
  autostart: true,
  persistent: true,
  targetPath: "/var/lib/libvirt/iso",
  capacityBytes: 1,
  allocationBytes: 1,
  availableBytes: 0,
  volumeCount: 2,
};

const volume: StorageVolume & { pool: string } = {
  name: "demo-db-01.qcow2", pool: "default", path: "/var/lib/libvirt/images/demo-db-01.qcow2",
  type: "file", format: "qcow2", capacityBytes: 80 * 1024 ** 3, allocationBytes: 42 * 1024 ** 3,
};

const image: VMImage = {
  id: 7, name: "ubuntu-24.04", description: "Ubuntu cloud image", pool: "default", volume: "ubuntu-24.04.qcow2",
  format: "qcow2", osFamily: "linux", osName: "Ubuntu 24.04", architecture: "x86_64",
  capacityBytes: 10 * 1024 ** 3, allocationBytes: 4 * 1024 ** 3, createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
};

const template: VMTemplate = {
  id: 9, name: "web-blueprint", description: "标准 Web 服务模板", vcpus: 4, memoryMiB: 4096, diskFormat: "qcow2",
  network: "default", firmware: "uefi", secureBoot: true, cloudInit: true, autostart: true, start: false,
  createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
};

const task: Task = {
  id: "task-100", kind: "vm.clone", targetKind: "vm", targetId: "demo-db-01", status: "failed", progress: 72,
  step: "复制磁盘", error: "disk full", actor: "admin", createdAt: "2026-01-02T00:00:00Z", updatedAt: "2026-01-02T00:01:00Z", cancellable: false,
};

describe("global search model", () => {
  it("shows quick pages when the query is empty", () => {
    const viewerItems = buildGlobalSearchItems("", "viewer", {});
    expect(viewerItems.every((item) => item.kind === "page")).toBe(true);
    expect(viewerItems.map((item) => item.title)).toContain("总览");
    expect(viewerItems.map((item) => item.title)).not.toContain("审计日志");

    const adminItems = buildGlobalSearchItems("", "admin", {});
    expect(adminItems.map((item) => item.title)).toContain("审计日志");
    expect(adminItems.map((item) => item.title)).toContain("用户管理");
    expect(adminItems.map((item) => item.title)).toContain("系统设置");
    expect(viewerItems.map((item) => item.title)).toContain("虚拟机模板");
    expect(viewerItems.map((item) => item.title)).toContain("关于");
  });

  it("matches VMs by IP, UUID and state while keeping a useful description", () => {
    for (const query of ["192.168.122.102", "22222222", "运行中"]) {
      const items = buildGlobalSearchItems(query, "viewer", { vms: [vm] });
      const result = items.find((item) => item.kind === "vm");
      expect(result?.title).toBe("demo-db-01");
      expect(result?.description).toContain("运行中");
      expect(result?.description).toContain("192.168.122.102");
      expect(result?.testId).toBe("global-search-vm-22222222-2222-4222-8222-222222222222");
    }
  });

  it("matches networks and storage pools by infrastructure fields", () => {
    const networkResult = buildGlobalSearchItems("virbr0", "operator", { networks: [network] }).find((item) => item.kind === "network");
    expect(networkResult?.title).toBe("default");
    expect(networkResult?.description).toContain("192.168.122.1/24");

    const poolResult = buildGlobalSearchItems("iso-store", "operator", { storagePools: [pool] }).find((item) => item.kind === "storage");
    expect(poolResult?.title).toBe("iso-store");
    expect(poolResult?.description).toContain("/var/lib/libvirt/iso");
  });

  it("matches volumes, images, templates and tasks", () => {
    const sources = { storageVolumes: [volume], images: [image], templates: [template], tasks: [task] };
    expect(buildGlobalSearchItems("demo-db-01.qcow2", "operator", sources).find((item) => item.kind === "volume")?.testId).toBe("global-search-volume-default-demo-db-01.qcow2");
    expect(buildGlobalSearchItems("ubuntu", "operator", sources).find((item) => item.kind === "image")?.title).toBe("ubuntu-24.04");
    expect(buildGlobalSearchItems("web-blueprint", "operator", sources).find((item) => item.kind === "template")?.description).toContain("4 vCPU");
    const taskResult = buildGlobalSearchItems("disk full", "operator", sources).find((item) => item.kind === "task");
    expect(taskResult?.title).toContain("demo-db-01");
    expect(taskResult?.description).toContain("失败");
  });

  it("returns page matches and no unrelated entity results", () => {
    const items = buildGlobalSearchItems("物理主机", "viewer", { vms: [vm], networks: [network], storagePools: [pool] });
    expect(items.filter((item) => item.kind !== "page")).toHaveLength(0);
    expect(items.map((item) => item.testId)).toContain("global-search-page-host");

    expect(buildGlobalSearchItems("definitely-missing", "admin", { vms: [vm] })).toHaveLength(0);
  });

  it("finds the about page by version and update keywords", () => {
    const items = buildGlobalSearchItems("update", "viewer", {});
    expect(items.find((item) => item.kind === "page" && item.path === "/about")?.title).toBe("关于");
  });
});
