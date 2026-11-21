import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { PlacementFormState } from "@/features/vms/vm-placement-model";

export function VMPlacementFields({ form, onChange, vcpus, memoryMiB, disabled, prefix }: {
  form: PlacementFormState;
  onChange: (form: PlacementFormState) => void;
  vcpus: number;
  memoryMiB: number;
  disabled: boolean;
  prefix: string;
}) {
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor={`${prefix}-hugepages`}>HugePages 页大小</Label>
        <Input id={`${prefix}-hugepages`} data-testid={`${prefix}-hugepages`} list={`${prefix}-hugepage-options`} value={form.hugePageSize} onChange={(event) => onChange({ ...form, hugePageSize: event.target.value })} placeholder="留空使用普通内存" disabled={disabled} />
        <datalist id={`${prefix}-hugepage-options`}><option value="2MiB" /><option value="1GiB" /></datalist>
        <p className="text-[11px] text-muted-foreground">大小必须整除 {memoryMiB} MiB 内存；宿主机需预保留相应 HugePages。</p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${prefix}-emulator-pin`}>模拟器线程绑定</Label>
        <Input id={`${prefix}-emulator-pin`} data-testid={`${prefix}-emulator-pin`} value={form.emulatorPin} onChange={(event) => onChange({ ...form, emulatorPin: event.target.value })} placeholder="例如 0-1" disabled={disabled} className="font-mono" />
        <p className="text-[11px] text-muted-foreground">使用宿主机 CPU 列表格式，例如 0-1,4。</p>
      </div>
    </div>

    <section className="space-y-2 rounded-md border bg-muted/10 p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div><p className="text-sm font-medium">NUMA 拓扑</p><p className="mt-0.5 text-[11px] text-muted-foreground">CPU 集合不能重叠，CPU 总数必须等于 {vcpus}，内存总和必须等于 {memoryMiB} MiB。</p></div>
        <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => onChange({ ...form, numaCells: [...form.numaCells, { id: String(form.numaCells.length), cpuset: "", memoryMiB: "" }] })}><Plus className="h-3.5 w-3.5" />添加 cell</Button>
      </div>
      {form.numaCells.length ? <div className="space-y-2">{form.numaCells.map((cell, index) => <div key={`${prefix}-numa-${index}`} className="grid gap-2 rounded border bg-card p-2 sm:grid-cols-[5rem_1fr_8rem_auto]">
        <div className="space-y-1"><Label htmlFor={`${prefix}-numa-id-${index}`} className="text-[11px]">Cell ID</Label><Input id={`${prefix}-numa-id-${index}`} data-testid={`${prefix}-numa-id-${index}`} type="number" min={0} value={cell.id} onChange={(event) => onChange({ ...form, numaCells: form.numaCells.map((item, itemIndex) => itemIndex === index ? { ...item, id: event.target.value } : item) })} disabled={disabled} /></div>
        <div className="space-y-1"><Label htmlFor={`${prefix}-numa-cpus-${index}`} className="text-[11px]">CPU 集合</Label><Input id={`${prefix}-numa-cpus-${index}`} data-testid={`${prefix}-numa-cpus-${index}`} value={cell.cpuset} onChange={(event) => onChange({ ...form, numaCells: form.numaCells.map((item, itemIndex) => itemIndex === index ? { ...item, cpuset: event.target.value } : item) })} placeholder="0-1" disabled={disabled} className="font-mono" /></div>
        <div className="space-y-1"><Label htmlFor={`${prefix}-numa-memory-${index}`} className="text-[11px]">内存 (MiB)</Label><Input id={`${prefix}-numa-memory-${index}`} data-testid={`${prefix}-numa-memory-${index}`} type="number" min={1} value={cell.memoryMiB} onChange={(event) => onChange({ ...form, numaCells: form.numaCells.map((item, itemIndex) => itemIndex === index ? { ...item, memoryMiB: event.target.value } : item) })} disabled={disabled} /></div>
        <div className="flex items-end"><Button type="button" variant="ghost" size="icon-sm" aria-label="删除 NUMA cell" disabled={disabled} onClick={() => onChange({ ...form, numaCells: form.numaCells.filter((_, itemIndex) => itemIndex !== index) })}><Trash2 className="h-3.5 w-3.5" /></Button></div>
      </div>)}</div> : <p className="text-[11px] text-muted-foreground">未配置显式 NUMA 拓扑，由 libvirt 自动放置。</p>}
    </section>

    <section className="space-y-2 rounded-md border bg-muted/10 p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div><p className="text-sm font-medium">vCPU 绑定</p><p className="mt-0.5 text-[11px] text-muted-foreground">索引从 0 开始，同一 vCPU 只能绑定一次；留空由宿主调度器自动放置。</p></div>
        <Button type="button" variant="outline" size="sm" disabled={disabled} onClick={() => onChange({ ...form, vcpuPins: [...form.vcpuPins, { vcpu: String(form.vcpuPins.length), cpuset: "" }] })}><Plus className="h-3.5 w-3.5" />添加绑定</Button>
      </div>
      {form.vcpuPins.length ? <div className="space-y-2">{form.vcpuPins.map((pin, index) => <div key={`${prefix}-pin-${index}`} className="grid gap-2 rounded border bg-card p-2 sm:grid-cols-[6rem_1fr_auto]">
        <div className="space-y-1"><Label htmlFor={`${prefix}-pin-vcpu-${index}`} className="text-[11px]">vCPU 索引</Label><Input id={`${prefix}-pin-vcpu-${index}`} data-testid={`${prefix}-pin-vcpu-${index}`} type="number" min={0} max={Math.max(0, vcpus - 1)} value={pin.vcpu} onChange={(event) => onChange({ ...form, vcpuPins: form.vcpuPins.map((item, itemIndex) => itemIndex === index ? { ...item, vcpu: event.target.value } : item) })} disabled={disabled} /></div>
        <div className="space-y-1"><Label htmlFor={`${prefix}-pin-cpus-${index}`} className="text-[11px]">宿主机 CPU 集合</Label><Input id={`${prefix}-pin-cpus-${index}`} data-testid={`${prefix}-pin-cpus-${index}`} value={pin.cpuset} onChange={(event) => onChange({ ...form, vcpuPins: form.vcpuPins.map((item, itemIndex) => itemIndex === index ? { ...item, cpuset: event.target.value } : item) })} placeholder="0" disabled={disabled} className="font-mono" /></div>
        <div className="flex items-end"><Button type="button" variant="ghost" size="icon-sm" aria-label="删除 vCPU 绑定" disabled={disabled} onClick={() => onChange({ ...form, vcpuPins: form.vcpuPins.filter((_, itemIndex) => itemIndex !== index) })}><Trash2 className="h-3.5 w-3.5" /></Button></div>
      </div>)}</div> : <p className="text-[11px] text-muted-foreground">未配置 vCPU 亲和性绑定。</p>}
    </section>
  </div>;
}
