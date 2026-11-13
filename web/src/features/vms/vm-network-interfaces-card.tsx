import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, LoaderCircle, Network, RefreshCw, Unplug } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VM, VMNICModel } from "@/api/types";
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
import { useNetworksQuery } from "@/features/networks/use-networks-query";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { vmQueryKey } from "@/features/vms/use-vm-query";

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

  const selectedNetwork = network || networks[0]?.name || "";

  function invalidate(updated: VM) {
    queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
    void queryClient.invalidateQueries({ queryKey: ["vms"] });
    void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
    void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
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

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedNetwork) return;
    setNotice(null);
    attachMutation.mutate();
  }

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
                  <Button type="button" variant="ghost" size="icon-sm" aria-label={`移除网卡 ${item.mac || index}`} data-testid={`vm-network-detach-${item.mac || index}`} disabled={!canMutate || !item.mac || vm.interfaces.length <= 1 || detachMutation.isPending} title={vm.interfaces.length <= 1 ? "至少保留一个网络接口" : !item.mac ? "网卡没有 MAC 地址" : disabledReason || "移除网卡"} onClick={() => setDetachMAC(item.mac || null)}>
                    <Unplug className="h-4 w-4" />
                  </Button>
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
    </Card>
  );
}
