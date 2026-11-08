import type { Role, StoragePool, StoragePoolState, VM, VMState, VirtualNetwork } from "@/api/types";
import { formatBytes } from "@/lib/utils";

export type StaticSearchPath = "/" | "/vms" | "/templates" | "/storage" | "/networks" | "/host" | "/tasks" | "/events" | "/audit" | "/users" | "/settings";

type SearchItemBase = {
  id: string;
  group: string;
  title: string;
  description: string;
  testId: string;
};

export type GlobalSearchItem =
  | (SearchItemBase & { kind: "vm"; uuid: string })
  | (SearchItemBase & { kind: "network"; path: "/networks" })
  | (SearchItemBase & { kind: "storage"; path: "/storage" })
  | (SearchItemBase & { kind: "page"; path: StaticSearchPath });

export type SearchSources = {
  vms?: VM[];
  networks?: VirtualNetwork[];
  storagePools?: StoragePool[];
};

type SearchPage = {
  label: string;
  path: StaticSearchPath;
  description: string;
  keywords: string[];
  adminOnly?: boolean;
};

const pages: SearchPage[] = [
  { label: "总览", path: "/", description: "宿主机与虚拟机运行概览", keywords: ["overview", "dashboard", "首页", "总览"] },
  { label: "虚拟机", path: "/vms", description: "查看、搜索和管理虚拟机", keywords: ["vm", "virtual machine", "虚拟机", "域"] },
  { label: "虚拟机模板", path: "/templates", description: "复用虚拟机规格、固件与 Cloud-init 默认值", keywords: ["template", "vm template", "模板", "配置", "cloud-init"] },
  { label: "存储", path: "/storage", description: "存储池、卷和 ISO", keywords: ["storage", "pool", "volume", "iso", "存储", "磁盘", "卷"] },
  { label: "网络", path: "/networks", description: "虚拟网络、桥和 DHCP 租约", keywords: ["network", "bridge", "dhcp", "网络", "桥", "租约"] },
  { label: "物理主机", path: "/host", description: "CPU、内存、设备、存储和网络分配", keywords: ["host", "cpu", "memory", "device", "主机", "设备"] },
  { label: "任务中心", path: "/tasks", description: "长任务进度和历史结果", keywords: ["task", "job", "任务", "进度"] },
  { label: "事件", path: "/events", description: "libvirt 实时事件流", keywords: ["event", "sse", "事件", "日志"] },
  { label: "审计日志", path: "/audit", description: "管理员写操作审计记录", keywords: ["audit", "security", "审计", "安全"], adminOnly: true },
  { label: "用户管理", path: "/users", description: "本地用户、角色和启用状态", keywords: ["user", "role", "rbac", "用户", "角色"], adminOnly: true },
  { label: "系统设置", path: "/settings", description: "全局只读模式和运行安全边界", keywords: ["settings", "readonly", "security", "设置", "只读", "安全"], adminOnly: true },
];

const vmStateLabels: Record<VMState, string> = {
  running: "运行中",
  blocked: "阻塞",
  paused: "已暂停",
  "shutting-down": "关机中",
  crashed: "异常",
  suspended: "已挂起",
  shutoff: "已关机",
  unknown: "未知",
};

const poolStateLabels: Record<StoragePoolState, string> = {
  active: "活跃",
  inactive: "未激活",
  degraded: "降级",
  inaccessible: "不可访问",
  unknown: "未知",
};

function matches(query: string, fields: Array<string | undefined>) {
  if (!query) return true;
  return fields.some((field) => field?.toLocaleLowerCase().includes(query));
}

function pageTestId(path: StaticSearchPath) {
  return path === "/" ? "global-search-page-overview" : `global-search-page-${path.slice(1)}`;
}

export function buildGlobalSearchItems(query: string, role: Role | undefined, sources: SearchSources): GlobalSearchItem[] {
  const normalized = query.trim().toLocaleLowerCase();
  const result: GlobalSearchItem[] = [];

  if (normalized) {
    const vms = (sources.vms ?? [])
      .filter((vm) => matches(normalized, [
        vm.name,
        vm.uuid,
        vm.hostname,
        vmStateLabels[vm.state],
        ...(vm.ipAddresses ?? []),
        ...vm.interfaces.map((network) => network.source),
      ]))
      .sort((left, right) => left.name.localeCompare(right.name))
      .slice(0, 5)
      .map<GlobalSearchItem>((vm) => ({
        id: `vm:${vm.uuid}`,
        kind: "vm",
        group: "虚拟机",
        uuid: vm.uuid,
        title: vm.name,
        description: [vmStateLabels[vm.state], `${vm.vcpus} vCPU`, formatBytes(vm.memoryBytes), vm.ipAddresses?.join(", ")].filter(Boolean).join(" · "),
        testId: `global-search-vm-${vm.uuid}`,
      }));
    result.push(...vms);

    const networks = (sources.networks ?? [])
      .filter((network) => matches(normalized, [
        network.name,
        network.uuid,
        network.bridge,
        network.forwardMode,
        network.active ? "active 活跃" : "inactive 未激活",
        ...network.addresses.flatMap((address) => [address.address, `${address.address}/${address.prefix}`, address.dhcpStart, address.dhcpEnd]),
      ]))
      .sort((left, right) => left.name.localeCompare(right.name))
      .slice(0, 4)
      .map<GlobalSearchItem>((network) => ({
        id: `network:${network.name}`,
        kind: "network",
        group: "网络",
        path: "/networks",
        title: network.name,
        description: [network.active ? "活跃" : "未激活", network.forwardMode, network.bridge, network.addresses.map((address) => `${address.address}/${address.prefix}`).join(", ")].filter(Boolean).join(" · "),
        testId: `global-search-network-${network.name}`,
      }));
    result.push(...networks);

    const pools = (sources.storagePools ?? [])
      .filter((pool) => matches(normalized, [pool.name, pool.uuid, pool.type, pool.targetPath, poolStateLabels[pool.state], `${pool.volumeCount} 卷`]))
      .sort((left, right) => left.name.localeCompare(right.name))
      .slice(0, 4)
      .map<GlobalSearchItem>((pool) => ({
        id: `storage:${pool.name}`,
        kind: "storage",
        group: "存储池",
        path: "/storage",
        title: pool.name,
        description: [poolStateLabels[pool.state], pool.type, pool.targetPath, `${pool.volumeCount} 个卷`].filter(Boolean).join(" · "),
        testId: `global-search-storage-${pool.name}`,
      }));
    result.push(...pools);
  }

  const visiblePages = pages
    .filter((page) => !page.adminOnly || role === "admin")
    .filter((page) => matches(normalized, [page.label, page.description, ...page.keywords]))
    .slice(0, normalized ? 4 : pages.length)
    .map<GlobalSearchItem>((page) => ({
      id: `page:${page.path}`,
      kind: "page",
      group: normalized ? "页面" : "快速跳转",
      path: page.path,
      title: page.label,
      description: page.description,
      testId: pageTestId(page.path),
    }));
  result.push(...visiblePages);

  return result;
}
