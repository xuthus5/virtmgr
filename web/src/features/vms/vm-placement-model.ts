import type { VMPlacement, VMPlacementInput } from "@/api/types";

export type NUMACellForm = { id: string; cpuset: string; memoryMiB: string };
export type VCPUPinForm = { vcpu: string; cpuset: string };

export type PlacementFormState = {
  hugePageSize: string;
  emulatorPin: string;
  numaCells: NUMACellForm[];
  vcpuPins: VCPUPinForm[];
};

export const emptyPlacementForm: PlacementFormState = {
  hugePageSize: "",
  emulatorPin: "",
  numaCells: [],
  vcpuPins: [],
};

export function placementFormFrom(placement?: VMPlacement): PlacementFormState {
  if (!placement) return { ...emptyPlacementForm, numaCells: [], vcpuPins: [] };
  return {
    hugePageSize: placement.hugePageSize ?? "",
    emulatorPin: placement.emulatorPin ?? "",
    numaCells: placement.numaCells.map((cell) => ({ id: String(cell.id), cpuset: cell.cpuset, memoryMiB: String(cell.memoryMiB) })),
    vcpuPins: placement.vcpuPins.map((pin) => ({ vcpu: String(pin.vcpu), cpuset: pin.cpuset })),
  };
}

export function placementHasValues(form: PlacementFormState) {
  return Boolean(form.hugePageSize.trim() || form.emulatorPin.trim() || form.numaCells.length || form.vcpuPins.length);
}

export function parseCPUSet(value: string): number[] | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const cpus = new Set<number>();
  let processed = 0;
  for (const part of trimmed.split(",")) {
    const piece = part.trim();
    const match = /^(\d+)(?:-(\d+))?$/.exec(piece);
    if (!match) return null;
    const start = Number(match[1]);
    const end = match[2] === undefined ? start : Number(match[2]);
    if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || end < start) return null;
    if (end - start + 1 > 65_536 || processed + (end - start + 1) > 65_536) return null;
    for (let cpu = start; cpu <= end; cpu += 1) cpus.add(cpu);
    processed += end - start + 1;
  }
  return [...cpus].sort((left, right) => left - right);
}

export function hugePageBytes(value: string): number | null {
  const match = /^(\d+)(KiB|MiB|GiB)$/.exec(value.trim());
  if (!match) return null;
  const size = Number(match[1]);
  if (!Number.isSafeInteger(size) || size <= 0) return null;
  const multiplier = match[2] === "KiB" ? 1024 : match[2] === "MiB" ? 1024 ** 2 : 1024 ** 3;
  return size * multiplier;
}

export function buildPlacement(form: PlacementFormState, vcpus: number, memoryMiB: number): { input?: VMPlacementInput; error?: string } {
  try {
    return buildPlacementUnchecked(form, vcpus, memoryMiB);
  } catch (cause) {
    return { error: cause instanceof Error ? cause.message : "性能拓扑配置无效。" };
  }
}

function buildPlacementUnchecked(form: PlacementFormState, vcpus: number, memoryMiB: number): { input?: VMPlacementInput; error?: string } {
  const hugePageSize = form.hugePageSize.trim();
  if (hugePageSize) {
    const bytes = hugePageBytes(hugePageSize);
    if (bytes === null) return { error: "HugePage 大小必须使用 KiB、MiB 或 GiB，例如 2MiB。" };
    if ((memoryMiB * 1024 * 1024) % bytes !== 0) return { error: "HugePage 大小必须能够整除虚拟机内存。" };
  }
  const emulatorPin = form.emulatorPin.trim();
  if (emulatorPin && !parseCPUSet(emulatorPin)) return { error: "模拟器绑定的 CPU 集合格式无效。" };

  const pinRows = form.vcpuPins.filter((pin) => pin.vcpu.trim() || pin.cpuset.trim());
  const seenVCPU = new Set<number>();
  const vcpuPins = pinRows.map((pin) => {
    if (!/^\d+$/.test(pin.vcpu.trim())) throw new Error("vCPU 绑定索引必须是整数。");
    const vcpu = Number(pin.vcpu.trim());
    if (!Number.isSafeInteger(vcpu) || vcpu < 0 || vcpu >= vcpus) throw new Error(`vCPU 绑定索引必须在 0 到 ${Math.max(0, vcpus - 1)} 之间。`);
    if (seenVCPU.has(vcpu)) throw new Error("同一个 vCPU 不能重复绑定。");
    seenVCPU.add(vcpu);
    if (!parseCPUSet(pin.cpuset)) throw new Error(`vCPU ${vcpu} 的 CPU 集合格式无效。`);
    return { vcpu, cpuset: pin.cpuset.trim() };
  });

  const cellRows = form.numaCells.filter((cell) => cell.id.trim() || cell.cpuset.trim() || cell.memoryMiB.trim());
  const seenCell = new Set<number>();
  const seenCPU = new Set<number>();
  let assignedCPUCount = 0;
  let assignedMemory = 0;
  const numaCells = cellRows.map((cell) => {
    if (!/^\d+$/.test(cell.id.trim()) || !/^\d+$/.test(cell.memoryMiB.trim())) throw new Error("NUMA cell ID 和内存必须是正整数。");
    const id = Number(cell.id.trim());
    const cellMemory = Number(cell.memoryMiB.trim());
    if (seenCell.has(id)) throw new Error("NUMA cell ID 不能重复。");
    if (cellMemory < 1) throw new Error("NUMA cell 内存必须大于 0。");
    seenCell.add(id);
    const cpus = parseCPUSet(cell.cpuset);
    if (!cpus) throw new Error(`NUMA cell ${id} 的 CPU 集合格式无效。`);
    for (const cpu of cpus) {
      if (seenCPU.has(cpu)) throw new Error("NUMA cell 的 CPU 集合不能重叠。");
      seenCPU.add(cpu);
    }
    assignedCPUCount += cpus.length;
    assignedMemory += cellMemory;
    return { id, cpuset: cell.cpuset.trim(), memoryMiB: cellMemory };
  });
  if (numaCells.length && assignedCPUCount !== vcpus) return { error: `NUMA cell 的 CPU 总数必须等于 ${vcpus} 个 vCPU。` };
  if (numaCells.length && assignedMemory !== memoryMiB) return { error: `NUMA cell 的内存总和必须等于 ${memoryMiB} MiB。` };
  return { input: { hugePageSize, emulatorPin, numaCells, vcpuPins } };
}

export function placementSummary(form: PlacementFormState) {
  const parts: string[] = [];
  if (form.hugePageSize.trim()) parts.push(`HugePages ${form.hugePageSize.trim()}`);
  if (form.numaCells.length) parts.push(`${form.numaCells.length} 个 NUMA cell`);
  if (form.vcpuPins.length) parts.push(`${form.vcpuPins.length} 条 vCPU 绑定`);
  if (form.emulatorPin.trim()) parts.push(`模拟器绑定 ${form.emulatorPin.trim()}`);
  return parts.join(" · ");
}
