import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Cpu, LoaderCircle, MemoryStick, Save } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VM } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import {
  activeResourceLimits,
  isVMActiveResources,
  validateVMResources,
} from "@/features/vms/vm-resources-model";
import { vmQueryKey } from "@/features/vms/use-vm-query";
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

  useEffect(() => {
    setVcpus(String(vm.vcpus));
    setMemoryMiB(String(Math.round(vm.memoryBytes / (1024 * 1024))));
    setFormError("");
  }, [vm.uuid, vm.vcpus, vm.memoryBytes]);

  const mutation = useMutation({
    mutationFn: (input: { vcpus: number; memoryMiB: number }) => api.updateVMResources(vm.uuid, input),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "metrics"] });
      setNotice({ tone: "success", text: `CPU 与内存已更新为 ${updated.vcpus} vCPU / ${formatBytes(updated.memoryBytes)}。` });
    },
    onError: (error) => {
      setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "更新 CPU 与内存失败，请稍后重试。" });
    },
  });

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
          <Badge variant="outline" data-testid="vm-resources-max">上限 {limits.vcpus} vCPU / {formatBytes(limits.memoryMiB * 1024 * 1024)}</Badge>
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
    </Card>
  );
}
