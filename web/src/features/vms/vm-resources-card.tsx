import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Cpu, LoaderCircle, MemoryStick, Save, Settings2 } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VMBootDevice, VM, VMCPUUpdateInput } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import {
  activeResourceLimits,
  isVMActiveResources,
  validateVMResources,
} from "@/features/vms/vm-resources-model";
import { vmQueryKey } from "@/features/vms/use-vm-query";
import { queryKeys } from "@/lib/query-keys";
import { formatBytes } from "@/lib/utils";

export function VMResourcesCard({ vm, canMutate, disabledReason }: {
  vm: VM;
  canMutate: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const currentMemoryMiB = Math.round(vm.memoryBytes / (1024 * 1024));
  const limits = activeResourceLimits(vm);
  const live = isVMActiveResources(vm.state);
  const [vcpus, setVcpus] = useState(String(vm.vcpus));
  const [memoryMiB, setMemoryMiB] = useState(String(currentMemoryMiB));
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [bootFirst, setBootFirst] = useState<VMBootDevice | "">("");
  const [bootSecond, setBootSecond] = useState<VMBootDevice | "">("");
  const [bootThird, setBootThird] = useState<VMBootDevice | "">("");
  const [cpuModel, setCpuModel] = useState("");
  const [cpuSockets, setCpuSockets] = useState("1");
  const [cpuCores, setCpuCores] = useState("1");
  const [cpuThreads, setCpuThreads] = useState("1");
  const [advancedError, setAdvancedError] = useState("");

  const bootDevices = availableBootDevices(vm);
  const cpuEditable = vm.state === "shutoff" || vm.state === "crashed";

  useEffect(() => {
    setVcpus(String(vm.vcpus));
    setMemoryMiB(String(Math.round(vm.memoryBytes / (1024 * 1024))));
    setFormError("");
    resetAdvancedForm(vm);
  }, [vm.uuid, vm.vcpus, vm.memoryBytes]);

  const mutation = useMutation({
    mutationFn: (input: { vcpus: number; memoryMiB: number }) => api.updateVMResources(vm.uuid, input),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: queryKeys.vm(vm.uuid).xmlRoot });
      void queryClient.invalidateQueries({ queryKey: queryKeys.vm(vm.uuid).metrics });
      setNotice({ tone: "success", text: `CPU 与内存已更新为 ${updated.vcpus} vCPU / ${formatBytes(updated.memoryBytes)}。` });
    },
    onError: (error) => {
      setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "更新 CPU 与内存失败，请稍后重试。" });
    },
  });

  const bootMutation = useMutation({
    mutationFn: (devices: VMBootDevice[]) => api.updateVMBoot(vm.uuid, { devices }),
    onSuccess: (updated) => {
      applyUpdatedVM(updated);
      setNotice({ tone: "success", text: `启动顺序已更新为 ${(updated.bootDevices ?? []).map(bootDeviceLabel).join(" → ")}。` });
    },
    onError: (error) => setAdvancedError(error instanceof ApiError ? error.message : "更新启动顺序失败，请稍后重试。"),
  });

  const cpuMutation = useMutation({
    mutationFn: (input: VMCPUUpdateInput) => api.updateVMCPU(vm.uuid, input),
    onSuccess: (updated) => {
      applyUpdatedVM(updated);
      setNotice({ tone: "success", text: `CPU 型号与拓扑已更新，共 ${(updated.cpuSockets ?? 0) * (updated.cpuCores ?? 0) * (updated.cpuThreads ?? 0)} 个 vCPU。` });
    },
    onError: (error) => setAdvancedError(error instanceof ApiError ? error.message : "更新 CPU 型号与拓扑失败，请稍后重试。"),
  });

  function applyUpdatedVM(updated: VM) {
    queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
    void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
    void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
    void queryClient.invalidateQueries({ queryKey: queryKeys.vm(vm.uuid).xmlRoot });
    void queryClient.invalidateQueries({ queryKey: queryKeys.vm(vm.uuid).configVersions });
    resetAdvancedForm(updated);
  }

  function resetAdvancedForm(target: VM) {
    const devices = target.bootDevices ?? [];
    setBootFirst(devices[0] ?? "");
    setBootSecond(devices[1] ?? "");
    setBootThird(devices[2] ?? "");
    setCpuModel(target.cpuModel || "host-model");
    setCpuSockets(String(target.cpuSockets || 1));
    setCpuCores(String(target.cpuCores || target.vcpus || 1));
    setCpuThreads(String(target.cpuThreads || 1));
    setAdvancedError("");
  }

  function openAdvanced() {
    resetAdvancedForm(vm);
    setNotice(null);
    setAdvancedOpen(true);
  }

  function submitBoot() {
    const devices = [bootFirst, bootSecond, bootThird].filter((value): value is VMBootDevice => Boolean(value));
    if (!devices.length) return setAdvancedError("至少选择一个启动设备。");
    if (new Set(devices).size !== devices.length) return setAdvancedError("启动设备不能重复。");
    setAdvancedError("");
    bootMutation.mutate(devices);
  }

  function submitCPU() {
    const sockets = Number(cpuSockets);
    const cores = Number(cpuCores);
    const threads = Number(cpuThreads);
    if (!cpuModel.trim() || !/^[A-Za-z0-9][A-Za-z0-9_.+-]{0,63}$/.test(cpuModel.trim())) return setAdvancedError("CPU 型号格式无效。");
    if (![sockets, cores, threads].every((value) => Number.isSafeInteger(value) && value > 0)) return setAdvancedError("CPU 插槽、核心和线程数必须为正整数。");
    if (sockets * cores * threads !== vm.vcpus) return setAdvancedError(`CPU 拓扑乘积必须等于当前 ${vm.vcpus} 个 vCPU。`);
    setAdvancedError("");
    cpuMutation.mutate({ model: cpuModel.trim(), sockets, cores, threads });
  }

  function submit() {
    setNotice(null);
    const result = validateVMResources(vm, vcpus, memoryMiB);
    if (!result.input) {
      setFormError(result.error ?? "请检查输入。");
      return;
    }
    setFormError("");
    mutation.mutate(result.input);
  }

  return (
    <Card className="mt-4" data-testid="vm-resources-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><Cpu className="h-4 w-4 text-primary" />CPU 与内存</CardTitle>
            <CardDescription>
              {live ? "运行中的虚拟机可在当前最大值内热调整；扩大最大值需要先关机。" : "关机或异常停止的虚拟机会更新持久配置，并在下次启动时生效。"}
            </CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" data-testid="vm-resources-max">上限 {limits.vcpus} vCPU / {formatBytes(limits.memoryMiB * 1024 * 1024)}</Badge>
            <Button type="button" variant="outline" size="sm" data-testid="vm-advanced-config-open" disabled={!canMutate} title={!canMutate ? disabledReason : "修改启动顺序、CPU 型号和拓扑"} onClick={openAdvanced}>
              <Settings2 className="h-4 w-4" />启动与 CPU
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="rounded-lg border bg-muted/20 px-3 py-2">
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><Cpu className="h-3.5 w-3.5" />当前 vCPU</p>
            <p className="mt-1 text-lg font-semibold tabular-nums" data-testid="vm-resources-current-vcpus">{vm.vcpus}</p>
          </div>
          <div className="rounded-lg border bg-muted/20 px-3 py-2">
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground"><MemoryStick className="h-3.5 w-3.5" />当前内存</p>
            <p className="mt-1 text-lg font-semibold tabular-nums" data-testid="vm-resources-current-memory">{formatBytes(vm.memoryBytes)}</p>
          </div>
        </div>

        {canMutate ? (
          <form
            className="space-y-3 rounded-lg border bg-muted/10 p-4"
            data-testid="vm-resources-form"
            onSubmit={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="vm-resources-vcpus">vCPU 数量</Label>
                <Input
                  id="vm-resources-vcpus"
                  data-testid="vm-resources-vcpus"
                  type="number"
                  min={1}
                  max={255}
                  step={1}
                  value={vcpus}
                  onChange={(event) => { setVcpus(event.target.value); setFormError(""); }}
                  disabled={mutation.isPending}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-resources-memory">内存 (MiB)</Label>
                <Input
                  id="vm-resources-memory"
                  data-testid="vm-resources-memory"
                  type="number"
                  min={128}
                  max={1024 * 1024}
                  step={1}
                  value={memoryMiB}
                  onChange={(event) => { setMemoryMiB(event.target.value); setFormError(""); }}
                  disabled={mutation.isPending}
                />
              </div>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-xs text-muted-foreground">
                {live
                  ? `运行中最多可热调整至 ${limits.vcpus} vCPU / ${formatBytes(limits.memoryMiB * 1024 * 1024)}。`
                  : "关机状态可以同时扩展持久最大值，重启后使用新配置。"}
              </p>
              <Button type="submit" size="sm" data-testid="vm-resources-save" disabled={mutation.isPending || !vcpus.trim() || !memoryMiB.trim()}>
                {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                {mutation.isPending ? "正在更新..." : "保存资源"}
              </Button>
            </div>
            {formError ? <p className="text-xs text-destructive" role="alert" data-testid="vm-resources-error">{formError}</p> : null}
          </form>
        ) : (
          <p className="rounded-md border bg-muted/25 px-3 py-2 text-xs text-muted-foreground" data-testid="vm-resources-readonly">{disabledReason}</p>
        )}

        {notice ? (
          <p
            className={notice.tone === "success" ? "rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success" : "rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"}
            role="status"
            data-testid="vm-resources-notice"
          >
            <span className="inline-flex items-center gap-2">
              {notice.tone === "success" ? <CheckCircle2 className="h-3.5 w-3.5" /> : <AlertTriangle className="h-3.5 w-3.5" />}
              {notice.text}
            </span>
          </p>
        ) : null}
      </CardContent>

      <Modal
        open={advancedOpen}
        onClose={() => { if (!bootMutation.isPending && !cpuMutation.isPending) setAdvancedOpen(false); }}
        title="启动与 CPU 配置"
        description="启动顺序可随时更新并在下次启动生效；CPU 型号与拓扑只允许在关机或异常停止状态下修改。"
        testId="vm-advanced-config-modal"
      >
        <div className="space-y-5">
          <section className="space-y-3">
            <div><p className="text-sm font-medium">启动顺序</p><p className="mt-0.5 text-xs text-muted-foreground">仅列出当前虚拟机实际存在的设备；留空表示不参与启动顺序。</p></div>
            <div className="grid gap-3 sm:grid-cols-3">
              {[
                { label: "第一启动设备", value: bootFirst, set: setBootFirst },
                { label: "第二启动设备", value: bootSecond, set: setBootSecond },
                { label: "第三启动设备", value: bootThird, set: setBootThird },
              ].map((item, index) => (
                <div key={item.label} className="space-y-2">
                  <Label>{item.label}</Label>
                  <select value={item.value} onChange={(event) => item.set(event.target.value as VMBootDevice | "")} disabled={bootMutation.isPending} data-testid={`vm-boot-device-${index}`} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
                    <option value="">不使用</option>
                    {bootDevices.map((device) => <option key={device} value={device}>{bootDeviceLabel(device)}</option>)}
                  </select>
                </div>
              ))}
            </div>
            {!bootDevices.length ? <p className="text-xs text-muted-foreground">当前没有可配置的启动设备。</p> : null}
            <div className="flex justify-end"><Button type="button" size="sm" data-testid="vm-boot-save" disabled={bootMutation.isPending || !bootDevices.length || !vm.persistent} onClick={submitBoot}>{bootMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}{bootMutation.isPending ? "正在保存..." : "保存启动顺序"}</Button></div>
          </section>

          <section className="space-y-3 border-t pt-5">
            <div className="flex flex-wrap items-center justify-between gap-2"><div><p className="text-sm font-medium">CPU 型号与拓扑</p><p className="mt-0.5 text-xs text-muted-foreground">CPU 拓扑乘积必须等于当前 {vm.vcpus} 个 vCPU。</p></div><Badge variant={cpuEditable ? "outline" : "warning"}>{cpuEditable ? "可修改" : "需先关机"}</Badge></div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-2 sm:col-span-2"><Label htmlFor="vm-cpu-model">CPU 型号</Label><Input id="vm-cpu-model" data-testid="vm-cpu-model" list="vm-cpu-model-options" value={cpuModel} onChange={(event) => setCpuModel(event.target.value)} disabled={!cpuEditable || cpuMutation.isPending} placeholder="host-model" /><datalist id="vm-cpu-model-options"><option value="host-model" /><option value="host-passthrough" /><option value="Skylake-Client" /><option value="Cascadelake-Server" /><option value="EPYC" /></datalist></div>
              {[{ label: "插槽", value: cpuSockets, set: setCpuSockets, testId: "vm-cpu-sockets" }, { label: "每插槽核心", value: cpuCores, set: setCpuCores, testId: "vm-cpu-cores" }, { label: "每核心线程", value: cpuThreads, set: setCpuThreads, testId: "vm-cpu-threads" }].map((field) => (
                <div key={field.label} className="space-y-2"><Label htmlFor={field.testId}>{field.label}</Label><Input id={field.testId} data-testid={field.testId} type="number" min={1} step={1} value={field.value} onChange={(event) => field.set(event.target.value)} disabled={!cpuEditable || cpuMutation.isPending} /></div>
              ))}
            </div>
            {!cpuEditable ? <p className="text-xs text-muted-foreground">当前状态不支持修改 CPU 型号与拓扑，请先正常关机。</p> : null}
            <div className="flex justify-end"><Button type="button" size="sm" data-testid="vm-cpu-save" disabled={!cpuEditable || cpuMutation.isPending} onClick={submitCPU}>{cpuMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}{cpuMutation.isPending ? "正在保存..." : "保存 CPU 配置"}</Button></div>
          </section>

          {advancedError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{advancedError}</p> : null}
        </div>
      </Modal>
    </Card>
  );
}

function availableBootDevices(vm: VM): VMBootDevice[] {
  const devices: VMBootDevice[] = [];
  if (vm.disks.some((disk) => disk.device === "disk" && !disk.readOnly)) devices.push("hd");
  if (vm.disks.some((disk) => disk.device === "cdrom")) devices.push("cdrom");
  if (vm.interfaces.length) devices.push("network");
  return devices;
}

function bootDeviceLabel(device: VMBootDevice) {
  if (device === "hd") return "硬盘";
  if (device === "cdrom") return "光盘";
  return "网络";
}
