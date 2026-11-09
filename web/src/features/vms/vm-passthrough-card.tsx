import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, CircuitBoard, LoaderCircle, RefreshCw, Unplug } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { PassthroughDevice, VM } from "@/api/types";
import { ErrorState } from "@/components/feedback/states";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { hostDevicesQueryKey, useHostDevicesQuery } from "@/features/host/use-host-devices-query";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { vmQueryKey } from "@/features/vms/use-vm-query";
import {
  attachableHostDevices,
  hostDeviceLabel,
  passthroughDeviceName,
  passthroughTypeLabels,
} from "@/features/vms/vm-passthrough-model";

export function VMPassthroughCard({ vm, canMutate, disabledReason }: {
  vm: VM;
  canMutate: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const devicesQuery = useHostDevicesQuery();
  const [selected, setSelected] = useState("");
  const [detachTarget, setDetachTarget] = useState<PassthroughDevice | null>(null);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const attached = vm.passthrough ?? [];
  const available = attachableHostDevices(devicesQuery.data?.items ?? [], attached);

  function invalidateAfterMutation(updated: VM) {
    queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
    void queryClient.invalidateQueries({ queryKey: ["vms"] });
    void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
    void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
    void queryClient.invalidateQueries({ queryKey: hostDevicesQueryKey });
  }

  const attachMutation = useMutation({
    mutationFn: (device: string) => api.attachVMDevice(vm.uuid, { device }),
    onSuccess: (updated, device) => {
      invalidateAfterMutation(updated);
      setSelected("");
      setNotice({ tone: "success", text: `设备 ${device} 已直通到虚拟机。` });
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "直通设备失败，请稍后重试。" });
    },
  });

  const detachMutation = useMutation({
    mutationFn: (device: string) => api.detachVMDevice(vm.uuid, device),
    onSuccess: (updated, device) => {
      invalidateAfterMutation(updated);
      setDetachTarget(null);
      setNotice({ tone: "success", text: `设备 ${device} 已从虚拟机移除。` });
    },
    onError: (cause) => {
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "移除直通设备失败，请稍后重试。" });
    },
  });

  return (
    <Card className="mt-4" data-testid="vm-passthrough-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><CircuitBoard className="h-4 w-4 text-primary" />PCI / USB 直通</CardTitle>
            <CardDescription>将宿主机 PCI 或 USB 设备分配给虚拟机。运行中的虚拟机使用 libvirt 热插拔，关机虚拟机写入持久配置。</CardDescription>
          </div>
          <Badge variant="outline" data-testid="vm-passthrough-total">{attached.length} 个设备</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {canMutate ? (
          <form
            className="flex flex-col gap-3 rounded-lg border bg-muted/20 p-4 md:flex-row md:items-end"
            data-testid="vm-passthrough-form"
            onSubmit={(event) => {
              event.preventDefault();
              if (!selected) return;
              setNotice(null);
              attachMutation.mutate(selected);
            }}
          >
            <div className="min-w-0 flex-1 space-y-2">
              <label htmlFor="vm-passthrough-device" className="text-xs font-medium text-muted-foreground">可直通主机设备</label>
              <select
                id="vm-passthrough-device"
                data-testid="vm-passthrough-select"
                value={selected}
                onChange={(event) => setSelected(event.target.value)}
                disabled={attachMutation.isPending || devicesQuery.isLoading || available.length === 0}
                className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="">{devicesQuery.isLoading ? "正在读取主机设备..." : available.length ? "选择 PCI / USB 设备" : "没有可直通设备"}</option>
                {available.map((device) => <option key={device.name} value={device.name}>{hostDeviceLabel(device)}</option>)}
              </select>
            </div>
            <Button type="submit" size="sm" data-testid="vm-passthrough-attach" disabled={!selected || attachMutation.isPending} title={!selected ? "请先选择可直通的主机设备" : attachMutation.isPending ? "正在直通设备" : "将所选设备直通到虚拟机"}>
              {attachMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <CircuitBoard className="h-4 w-4" />}
              {attachMutation.isPending ? "正在直通..." : "直通设备"}
            </Button>
            <Button type="button" variant="ghost" size="icon-sm" aria-label="刷新主机设备" title={devicesQuery.isFetching ? "正在刷新主机设备" : "刷新主机设备列表"} data-testid="vm-passthrough-refresh" disabled={devicesQuery.isFetching} onClick={() => void devicesQuery.refetch()}>
              <RefreshCw className={devicesQuery.isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
            </Button>
          </form>
        ) : (
          <p className="rounded-md border bg-muted/25 px-3 py-2 text-xs text-muted-foreground" data-testid="vm-passthrough-readonly">{disabledReason}</p>
        )}

        {devicesQuery.isError ? (
          <ErrorState message={devicesQuery.error instanceof Error ? devicesQuery.error.message : "无法读取主机设备"} onRetry={() => void devicesQuery.refetch()} />
        ) : null}

        {notice ? (
          <p className={notice.tone === "success" ? "rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success" : "rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"} role="status" data-testid="vm-passthrough-notice">
            <span className="inline-flex items-center gap-2">{notice.tone === "success" ? <CheckCircle2 className="h-3.5 w-3.5" /> : <AlertTriangle className="h-3.5 w-3.5" />}{notice.text}</span>
          </p>
        ) : null}

        {attached.length ? (
          <div className="rounded-lg border">
            <Table className="min-w-[880px]">
              <TableHeader><TableRow><TableHead>类型</TableHead><TableHead>设备</TableHead><TableHead>厂商 / 产品 ID</TableHead><TableHead>地址</TableHead><TableHead>驱动</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {attached.map((device) => (
                  <TableRow key={device.name} data-testid={`vm-passthrough-row-${device.name}`}>
                    <TableCell><Badge variant="outline">{passthroughTypeLabels[device.type] ?? device.type}</Badge></TableCell>
                    <TableCell>
                      <p className="font-medium">{passthroughDeviceName(device)}</p>
                      <p className="font-mono text-[11px] text-muted-foreground">{device.name}</p>
                    </TableCell>
                    <TableCell className="text-xs">
                      <p>{device.vendor || device.vendorId || "-"}</p>
                      <p className="font-mono text-muted-foreground">{[device.vendorId, device.productId].filter(Boolean).join(" / ") || "-"}</p>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{device.address || "-"}</TableCell>
                    <TableCell className="font-mono text-xs">{device.driver || "-"}</TableCell>
                    <TableCell>
                      <Button type="button" variant="ghost" size="icon-sm" data-testid={`vm-passthrough-detach-${device.name}`} aria-label={`移除直通设备 ${device.name}`} disabled={!canMutate || detachMutation.isPending} title={canMutate ? "从虚拟机移除该设备" : disabledReason} onClick={() => { setNotice(null); setDetachTarget(device); }}>
                        <Unplug className="h-4 w-4" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ) : <p className="text-sm text-muted-foreground" data-testid="vm-passthrough-empty">当前虚拟机还没有直通 PCI / USB 设备。</p>}
      </CardContent>

      <AlertDialog open={detachTarget !== null} onOpenChange={(open) => { if (!open) setDetachTarget(null); }}>
        <AlertDialogContent data-testid="vm-passthrough-detach-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>移除直通设备？</AlertDialogTitle>
            <AlertDialogDescription>
              将从 <span className="font-medium text-foreground">{vm.name}</span> 移除设备 <span className="font-mono text-foreground">{detachTarget?.name}</span>。运行中的虚拟机将尝试热拔除该设备。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button variant="destructive" data-testid="vm-passthrough-confirm-detach" disabled={detachMutation.isPending} onClick={() => { if (detachTarget) detachMutation.mutate(detachTarget.name); }}>
              {detachMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Unplug className="h-4 w-4" />}
              确认移除
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
