import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Gauge, LoaderCircle, Network, Pencil, RefreshCw, Unplug } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { NetworkInterface, VM, VMNetworkQoSInput, VMNICModel } from "@/api/types";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useNetworksQuery } from "@/features/networks/use-networks-query";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { vmQueryKey } from "@/features/vms/use-vm-query";
import { queryKeys } from "@/lib/query-keys";

export function VMNetworkInterfacesCard({ vm, canMutate, disabledReason }: {
  vm: VM;
  canMutate: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const networksQuery = useNetworksQuery();
  const networks = (networksQuery.data?.items ?? []).filter((network) => network.active);
  const [network, setNetwork] = useState("");
  const [model, setModel] = useState<VMNICModel>("virtio");
  const [detachMAC, setDetachMAC] = useState<string | null>(null);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [qosInterface, setQosInterface] = useState<NetworkInterface | null>(null);
  const [qosInbound, setQosInbound] = useState("");
  const [qosInboundPeak, setQosInboundPeak] = useState("");
  const [qosInboundBurst, setQosInboundBurst] = useState("");
  const [qosOutbound, setQosOutbound] = useState("");
  const [qosOutboundPeak, setQosOutboundPeak] = useState("");
  const [qosOutboundBurst, setQosOutboundBurst] = useState("");
  const [qosError, setQosError] = useState("");
  const [modelInterface, setModelInterface] = useState<NetworkInterface | null>(null);
  const [modelEditValue, setModelEditValue] = useState<VMNICModel>("virtio");
  const [modelError, setModelError] = useState("");

  const selectedNetwork = network || networks[0]?.name || "";

  function invalidate(updated: VM) {
    queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
    void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
    void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
    void queryClient.invalidateQueries({ queryKey: queryKeys.vm(vm.uuid).xmlRoot });
  }

  const attachMutation = useMutation({
    mutationFn: () => api.attachVMNetwork(vm.uuid, { network: selectedNetwork, model }),
    onSuccess: (updated) => {
      invalidate(updated);
      setNotice({ tone: "success", text: `已向虚拟机附加网络 ${selectedNetwork}。` });
    },
    onError: (cause) => setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "附加网卡失败，请稍后重试。" }),
  });

  const detachMutation = useMutation({
    mutationFn: (mac: string) => api.detachVMNetwork(vm.uuid, mac),
    onSuccess: (updated, mac) => {
      invalidate(updated);
      setDetachMAC(null);
      setNotice({ tone: "success", text: `网卡 ${mac} 已移除。` });
    },
    onError: (cause) => setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "移除网卡失败，请稍后重试。" }),
  });

  const qosMutation = useMutation({
    mutationFn: ({ mac, input }: { mac: string; input: VMNetworkQoSInput }) => api.updateVMNetworkQoS(vm.uuid, mac, input),
    onSuccess: (updated, variables) => {
      invalidate(updated);
      void queryClient.invalidateQueries({ queryKey: queryKeys.vm(vm.uuid).configVersions });
      setQosInterface(null);
      setNotice({ tone: "success", text: `网卡 ${variables.mac} 的带宽限制已更新。` });
    },
    onError: (cause) => setQosError(cause instanceof ApiError ? cause.message : "更新网卡带宽限制失败，请稍后重试。"),
  });

  const modelMutation = useMutation({
    mutationFn: ({ mac, nextModel }: { mac: string; nextModel: VMNICModel }) => api.updateVMNetworkModel(vm.uuid, mac, { model: nextModel }),
    onSuccess: (updated, variables) => {
      invalidate(updated);
      void queryClient.invalidateQueries({ queryKey: queryKeys.vm(vm.uuid).configVersions });
      setModelInterface(null);
      setNotice({ tone: "success", text: `网卡 ${variables.mac} 的型号已更新为 ${variables.nextModel}。` });
    },
    onError: (cause) => setModelError(cause instanceof ApiError ? cause.message : "更新网卡型号失败，请稍后重试。"),
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedNetwork) return;
    setNotice(null);
    attachMutation.mutate();
  }

  function openQoS(networkInterface: NetworkInterface) {
    setQosInterface(networkInterface);
    setQosInbound(qosBytesToMiB(networkInterface.qos?.inboundAverageBytes));
    setQosInboundPeak(qosBytesToMiB(networkInterface.qos?.inboundPeakBytes));
    setQosInboundBurst(qosBytesToMiB(networkInterface.qos?.inboundBurstBytes));
    setQosOutbound(qosBytesToMiB(networkInterface.qos?.outboundAverageBytes));
    setQosOutboundPeak(qosBytesToMiB(networkInterface.qos?.outboundPeakBytes));
    setQosOutboundBurst(qosBytesToMiB(networkInterface.qos?.outboundBurstBytes));
    setQosError("");
    setNotice(null);
  }

  function submitQoS() {
    if (!qosInterface?.mac) return;
    const inbound = parseQosMiB(qosInbound);
    const inboundPeak = parseQosMiB(qosInboundPeak);
    const inboundBurst = parseQosMiB(qosInboundBurst);
    const outbound = parseQosMiB(qosOutbound);
    const outboundPeak = parseQosMiB(qosOutboundPeak);
    const outboundBurst = parseQosMiB(qosOutboundBurst);
    if ([inbound, inboundPeak, inboundBurst, outbound, outboundPeak, outboundBurst].some((value) => value === null)) {
      setQosError("请输入非负数值，带宽单位为 MiB/s，突发缓冲单位为 MiB。");
      return;
    }
    if (((inboundPeak ?? 0) > 0 || (inboundBurst ?? 0) > 0) && (inbound ?? 0) === 0) {
      setQosError("配置入站峰值或突发缓冲时，必须填写入站平均值。");
      return;
    }
    if (((outboundPeak ?? 0) > 0 || (outboundBurst ?? 0) > 0) && (outbound ?? 0) === 0) {
      setQosError("配置出站峰值或突发缓冲时，必须填写出站平均值。");
      return;
    }
    if ((inboundPeak ?? 0) > 0 && (inboundPeak ?? 0) < (inbound ?? 0)) {
      setQosError("入站峰值不能小于入站平均值。");
      return;
    }
    if ((outboundPeak ?? 0) > 0 && (outboundPeak ?? 0) < (outbound ?? 0)) {
      setQosError("出站峰值不能小于出站平均值。");
      return;
    }
    setQosError("");
    qosMutation.mutate({ mac: qosInterface.mac, input: {
      inboundAverageBytes: inbound ?? 0, inboundPeakBytes: inboundPeak ?? 0, inboundBurstBytes: inboundBurst ?? 0,
      outboundAverageBytes: outbound ?? 0, outboundPeakBytes: outboundPeak ?? 0, outboundBurstBytes: outboundBurst ?? 0,
    } });
  }

  function openModel(networkInterface: NetworkInterface) {
    setModelInterface(networkInterface);
    setModelEditValue((networkInterface.model as VMNICModel | undefined) ?? "virtio");
    setModelError("");
    setNotice(null);
  }

  function submitModel() {
    if (!modelInterface?.mac) return;
    setModelError("");
    modelMutation.mutate({ mac: modelInterface.mac, nextModel: modelEditValue });
  }

  const modelEditAllowed = vm.persistent && (vm.state === "shutoff" || vm.state === "crashed");

  return (
    <Card className="mt-4" data-testid="vm-network-interfaces-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><Network className="h-4 w-4 text-primary" />网络接口</CardTitle>
            <CardDescription>附加或移除虚拟网卡。运行中的虚拟机会尝试热插拔，关机虚拟机写入持久配置；最后一个网卡不可移除。</CardDescription>
          </div>
          <Badge variant="outline">{vm.interfaces.length} 个接口</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {canMutate ? (
          <form className="flex flex-col gap-3 rounded-lg border bg-muted/20 p-4 md:flex-row md:items-end" onSubmit={submit} data-testid="vm-network-attach-form">
            <div className="min-w-0 flex-1 space-y-2">
              <label htmlFor="vm-network-attach-network" className="text-sm font-medium">虚拟网络</label>
              <select id="vm-network-attach-network" data-testid="vm-network-attach-network" value={selectedNetwork} onChange={(event) => setNetwork(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
                {networks.map((item) => <option key={item.name} value={item.name}>{item.name} · {item.forwardMode || "isolated"}</option>)}
              </select>
            </div>
            <div className="space-y-2">
              <label htmlFor="vm-network-attach-model" className="text-sm font-medium">网卡型号</label>
              <select id="vm-network-attach-model" data-testid="vm-network-attach-model" value={model} onChange={(event) => setModel(event.target.value as VMNICModel)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
                <option value="virtio">virtio</option>
                <option value="e1000">e1000</option>
              </select>
            </div>
            <Button type="submit" size="sm" data-testid="vm-network-attach-submit" disabled={attachMutation.isPending || !selectedNetwork}>
              {attachMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Network className="h-4 w-4" />}
              {attachMutation.isPending ? "正在附加..." : "附加网卡"}
            </Button>
            <Button type="button" variant="ghost" size="icon-sm" aria-label="刷新虚拟网络" title="刷新虚拟网络" disabled={networksQuery.isFetching} onClick={() => void networksQuery.refetch()}>
              <RefreshCw className={networksQuery.isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
            </Button>
          </form>
        ) : <p className="rounded-md border bg-muted/25 px-3 py-2 text-xs text-muted-foreground">{disabledReason}</p>}

        {notice ? (
          <p role="status" className={notice.tone === "success" ? "flex items-center gap-2 rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success" : "flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"}>
            {notice.tone === "success" ? <CheckCircle2 className="h-3.5 w-3.5" /> : <AlertTriangle className="h-3.5 w-3.5" />}{notice.text}
          </p>
        ) : null}

        {vm.interfaces.length ? (
          <Table className="min-w-[760px]">
            <TableHeader><TableRow><TableHead>类型</TableHead><TableHead>来源</TableHead><TableHead>MAC</TableHead><TableHead>模型</TableHead><TableHead>PCI 地址</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
            <TableBody>{vm.interfaces.map((item, index) => (
              <TableRow key={`${item.mac || "nic"}-${index}`} data-testid={`vm-network-row-${item.mac || index}`}>
                <TableCell>{item.type || "-"}</TableCell>
                <TableCell className="font-mono text-xs">{item.source || "-"}</TableCell>
                <TableCell className="font-mono text-xs">{item.mac || "-"}</TableCell>
                <TableCell>{item.model || "-"}</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{item.address || "-"}</TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-1">
                    <Button type="button" variant={item.qos ? "outline" : "ghost"} size="sm" aria-label={`配置网卡 ${item.mac || index} 带宽`} data-testid={`vm-network-qos-${item.mac || index}`} disabled={!canMutate || !item.mac || qosMutation.isPending} title={!item.mac ? "网卡没有 MAC 地址" : disabledReason || "配置入站和出站带宽限制"} onClick={() => openQoS(item)}>
                      <Gauge className="h-3.5 w-3.5" />带宽
                    </Button>
                    <Button type="button" variant="ghost" size="icon-sm" aria-label={`修改网卡 ${item.mac || index} 型号`} data-testid={`vm-network-model-${item.mac || index}`} disabled={!canMutate || !modelEditAllowed || !item.mac || modelMutation.isPending} title={!modelEditAllowed ? "仅支持修改已关机或异常停止的持久虚拟机网卡型号" : !item.mac ? "网卡没有 MAC 地址" : disabledReason || "修改网卡型号"} onClick={() => openModel(item)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button type="button" variant="ghost" size="icon-sm" aria-label={`移除网卡 ${item.mac || index}`} data-testid={`vm-network-detach-${item.mac || index}`} disabled={!canMutate || !item.mac || vm.interfaces.length <= 1 || detachMutation.isPending} title={vm.interfaces.length <= 1 ? "至少保留一个网络接口" : !item.mac ? "网卡没有 MAC 地址" : disabledReason || "移除网卡"} onClick={() => setDetachMAC(item.mac || null)}>
                      <Unplug className="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}</TableBody>
          </Table>
        ) : <p className="text-sm text-muted-foreground">当前虚拟机没有网络接口。</p>}
      </CardContent>

      <AlertDialog open={detachMAC !== null} onOpenChange={(open) => { if (!open) setDetachMAC(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>移除网络接口？</AlertDialogTitle>
            <AlertDialogDescription>将从 <span className="font-medium text-foreground">{vm.name}</span> 移除 MAC 为 <span className="font-mono text-foreground">{detachMAC}</span> 的网卡。运行中的虚拟机将尝试热拔除。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button variant="destructive" disabled={detachMutation.isPending} onClick={() => { if (detachMAC) detachMutation.mutate(detachMAC); }}>
              {detachMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Unplug className="h-4 w-4" />}确认移除
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Modal open={qosInterface !== null} onClose={() => { if (!qosMutation.isPending) setQosInterface(null); }} title={`网卡带宽限制 · ${qosInterface?.mac || ""}`} description="通过 libvirt interface bandwidth 限制入站与出站速率。平均值和峰值单位为 MiB/s，burst 为突发缓冲 MiB；全部清零可移除限制。" testId="vm-network-qos-modal" footer={<><Button variant="ghost" disabled={qosMutation.isPending} onClick={() => setQosInterface(null)}>取消</Button><Button data-testid="vm-network-qos-submit" disabled={qosMutation.isPending} onClick={submitQoS}>{qosMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Gauge className="h-4 w-4" />}{qosMutation.isPending ? "正在保存..." : "保存限制"}</Button></>}>
        <div className="space-y-4">
          <section className="space-y-3"><p className="text-sm font-medium">入站</p><div className="grid gap-3 sm:grid-cols-3">
            <div className="space-y-2"><Label htmlFor="vm-network-qos-in">平均 (MiB/s)</Label><Input id="vm-network-qos-in" data-testid="vm-network-qos-in" type="number" min={0} step="any" value={qosInbound} onChange={(event) => setQosInbound(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-network-qos-in-peak">峰值 (MiB/s)</Label><Input id="vm-network-qos-in-peak" data-testid="vm-network-qos-in-peak" type="number" min={0} step="any" value={qosInboundPeak} onChange={(event) => setQosInboundPeak(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-network-qos-in-burst">突发缓冲 (MiB)</Label><Input id="vm-network-qos-in-burst" data-testid="vm-network-qos-in-burst" type="number" min={0} step="any" value={qosInboundBurst} onChange={(event) => setQosInboundBurst(event.target.value)} disabled={qosMutation.isPending} /></div>
          </div></section>
          <section className="space-y-3 border-t pt-4"><p className="text-sm font-medium">出站</p><div className="grid gap-3 sm:grid-cols-3">
            <div className="space-y-2"><Label htmlFor="vm-network-qos-out">平均 (MiB/s)</Label><Input id="vm-network-qos-out" data-testid="vm-network-qos-out" type="number" min={0} step="any" value={qosOutbound} onChange={(event) => setQosOutbound(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-network-qos-out-peak">峰值 (MiB/s)</Label><Input id="vm-network-qos-out-peak" data-testid="vm-network-qos-out-peak" type="number" min={0} step="any" value={qosOutboundPeak} onChange={(event) => setQosOutboundPeak(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-network-qos-out-burst">突发缓冲 (MiB)</Label><Input id="vm-network-qos-out-burst" data-testid="vm-network-qos-out-burst" type="number" min={0} step="any" value={qosOutboundBurst} onChange={(event) => setQosOutboundBurst(event.target.value)} disabled={qosMutation.isPending} /></div>
          </div></section>
          {qosError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{qosError}</p> : null}
        </div>
      </Modal>

      <Modal open={modelInterface !== null} onClose={() => { if (!modelMutation.isPending) setModelInterface(null); }} title={`修改网卡型号 · ${modelInterface?.mac || ""}`} description="网卡型号会写入虚拟机持久配置，重启后生效。当前仅支持 virtio 与 e1000。" testId="vm-network-model-modal" footer={<><Button variant="ghost" disabled={modelMutation.isPending} onClick={() => setModelInterface(null)}>取消</Button><Button data-testid="vm-network-model-submit" disabled={modelMutation.isPending || !modelInterface?.mac} onClick={submitModel}>{modelMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Pencil className="h-4 w-4" />}{modelMutation.isPending ? "正在保存..." : "保存型号"}</Button></>}>
        <div className="space-y-4">
          <div className="rounded-md border bg-muted/20 px-3 py-2 text-sm"><span className="text-muted-foreground">当前型号：</span><span className="font-mono">{modelInterface?.model || "virtio"}</span></div>
          <div className="space-y-2">
            <Label htmlFor="vm-network-model-value">新网卡型号</Label>
            <select id="vm-network-model-value" data-testid="vm-network-model-value" value={modelEditValue} onChange={(event) => setModelEditValue(event.target.value as VMNICModel)} disabled={modelMutation.isPending} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
              <option value="virtio">virtio（推荐，性能更好）</option>
              <option value="e1000">e1000（兼容旧操作系统）</option>
            </select>
          </div>
          {modelError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{modelError}</p> : null}
        </div>
      </Modal>
    </Card>
  );
}

function qosBytesToMiB(value?: number) {
  if (!value) return "";
  return String(Number((value / 1024 / 1024).toFixed(3)));
}

function parseQosMiB(value: string): number | null {
  const trimmed = value.trim();
  if (!trimmed) return 0;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed < 0 || !Number.isSafeInteger(Math.round(parsed * 1024 * 1024))) return null;
  return Math.round(parsed * 1024 * 1024);
}
