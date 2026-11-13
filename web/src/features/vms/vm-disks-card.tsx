import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, HardDrive, LoaderCircle, Maximize2, Plus, Unplug } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { Disk, Role, VM } from "@/api/types";
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
import {
  defaultDiskCapacityGiB,
  validateDiskCapacity,
  vmDiskResizeDisabledReason,
} from "@/features/vms/vm-disk-resize-model";
import { vmQueryKey } from "@/features/vms/use-vm-query";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { formatBytes } from "@/lib/utils";
import { useStoragePoolsQuery, useStorageVolumesQuery } from "@/features/storage/use-storage-query";

export function VMDisksCard({ vm, role, readOnly }: {
  vm: VM;
  role: Role | undefined;
  readOnly: boolean;
}) {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState<Disk | null>(null);
  const [capacityGiB, setCapacityGiB] = useState("");
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [attachOpen, setAttachOpen] = useState(false);
  const [pool, setPool] = useState("");
  const [volume, setVolume] = useState("");
  const [bus, setBus] = useState("virtio");
  const [target, setTarget] = useState("");
  const [attachError, setAttachError] = useState("");
  const [detachTarget, setDetachTarget] = useState<string | null>(null);
  const poolsQuery = useStoragePoolsQuery(attachOpen);
  const activePools = (poolsQuery.data?.items ?? []).filter((item) => item.state === "active");
  const selectedPool = pool || activePools[0]?.name || "";
  const volumesQuery = useStorageVolumesQuery(selectedPool);
  const eligibleVolumes = (volumesQuery.data?.items ?? []).filter((item) => item.format === "qcow2" || item.format === "raw");
  const selectedVolume = eligibleVolumes.some((item) => item.name === volume) ? volume : eligibleVolumes[0]?.name || "";
  const rootTarget = vm.disks.find((disk) => disk.device === "disk" && !disk.readOnly && disk.source)?.target || "";

  const mutation = useMutation({
    mutationFn: (input: { target: string; capacityBytes: number }) => api.resizeVMDisk(vm.uuid, input.target, { capacityBytes: input.capacityBytes }),
    onSuccess: (updated, input) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
      setSelected(null);
      setFormError("");
      setNotice({ tone: "success", text: `${input.target} 已扩容至 ${formatBytes(input.capacityBytes)}。请在客户机内继续扩展分区和文件系统。` });
    },
    onError: (error) => {
      setFormError(error instanceof ApiError ? error.message : "磁盘扩容失败，请稍后重试。");
    },
  });

  const attachMutation = useMutation({
    mutationFn: () => api.attachVMDisk(vm.uuid, { pool: selectedPool, volume: selectedVolume, bus: bus as "virtio" | "sata" | "scsi", ...(target.trim() ? { target: target.trim() } : {}) }),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
      setAttachOpen(false);
      setVolume("");
      setTarget("");
      setAttachError("");
      setNotice({ tone: "success", text: `已从 ${selectedPool}/${selectedVolume} 附加磁盘。` });
    },
    onError: (cause) => setAttachError(cause instanceof ApiError ? cause.message : "附加磁盘失败，请稍后重试。"),
  });

  const detachMutation = useMutation({
    mutationFn: (diskTarget: string) => api.detachVMDisk(vm.uuid, diskTarget),
    onSuccess: (updated, diskTarget) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
      setDetachTarget(null);
      setNotice({ tone: "success", text: `磁盘 ${diskTarget} 已移除。` });
    },
    onError: (cause) => setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "移除磁盘失败，请稍后重试。" }),
  });

  function openResize(disk: Disk) {
    setSelected(disk);
    setCapacityGiB(String(defaultDiskCapacityGiB(disk)));
    setFormError("");
    setNotice(null);
  }

  function submitResize() {
    if (!selected) return;
    const result = validateDiskCapacity(selected, capacityGiB);
    if (!result.input) {
      setFormError(result.error ?? "请检查输入。");
      return;
    }
    setFormError("");
    mutation.mutate({ target: selected.target, capacityBytes: result.input.capacityBytes });
  }

  return (
    <Card className="mt-4" data-testid="vm-disks-card">
      <CardHeader>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="flex items-center gap-2"><HardDrive className="h-4 w-4 text-primary" />磁盘设备</CardTitle>
            <CardDescription>磁盘目标、总线、格式、容量和 libvirt 数据源。</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant="outline">支持热插拔</Badge>
            <Button size="sm" variant="outline" data-testid="vm-disk-attach-open" disabled={!role || role === "viewer" || readOnly} title={readOnly ? "系统当前处于只读模式" : role === "viewer" ? "当前账号没有磁盘操作权限" : "附加存储卷"} onClick={() => { setAttachError(""); setNotice(null); setAttachOpen(true); }}>
              <Plus className="h-4 w-4" />附加磁盘
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 p-0">
        {vm.disks.length ? (
          <Table className="min-w-[1080px]">
            <TableHeader>
              <TableRow>
                <TableHead>目标</TableHead>
                <TableHead>设备</TableHead>
                <TableHead>总线</TableHead>
                <TableHead>格式</TableHead>
                <TableHead>容量</TableHead>
                <TableHead>已分配</TableHead>
                <TableHead className="min-w-[22rem]">数据源</TableHead>
                <TableHead>访问</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {vm.disks.map((disk, index) => {
                const disabledReason = vmDiskResizeDisabledReason(role, readOnly, vm, disk);
                return (
                  <TableRow key={`${disk.target}-${index}`} data-testid={`vm-disk-row-${disk.target || index}`}>
                    <TableCell className="font-mono text-xs">{disk.target || "-"}</TableCell>
                    <TableCell>{disk.device || "-"}</TableCell>
                    <TableCell className="font-mono text-xs">{disk.bus || "-"}</TableCell>
                    <TableCell>{disk.format || "-"}</TableCell>
                    <TableCell className="whitespace-nowrap font-medium tabular-nums" data-testid={`vm-disk-capacity-${disk.target || index}`}>
                      {disk.capacityBytes > 0 ? formatBytes(disk.capacityBytes) : "-"}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-muted-foreground tabular-nums">
                      {disk.allocationBytes > 0 ? formatBytes(disk.allocationBytes) : "-"}
                    </TableCell>
                    <TableCell>
                      <span
                        data-testid="vm-disk-source"
                        title={disk.source || "-"}
                        className="block w-[20rem] max-w-[28rem] truncate whitespace-nowrap font-mono text-xs text-muted-foreground"
                      >
                        {disk.source || "-"}
                      </span>
                    </TableCell>
                    <TableCell><Badge variant={disk.readOnly ? "secondary" : "outline"}>{disk.readOnly ? "只读" : "读写"}</Badge></TableCell>
                    <TableCell className="text-right">
                      {disk.device === "disk" ? (
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="outline"
                            size="sm"
                            data-testid={`vm-disk-resize-${disk.target}`}
                            disabled={Boolean(disabledReason) || mutation.isPending}
                            title={disabledReason || `扩容磁盘 ${disk.target}`}
                            onClick={() => openResize(disk)}
                          >
                            <Maximize2 className="h-3.5 w-3.5" />扩容
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            data-testid={`vm-disk-detach-${disk.target}`}
                            disabled={Boolean(disabledReason) || detachMutation.isPending || disk.target === rootTarget}
                            title={disk.target === rootTarget ? "不能移除主系统磁盘" : disabledReason || "移除磁盘"}
                            aria-label={`移除磁盘 ${disk.target}`}
                            onClick={() => { setNotice(null); setDetachTarget(disk.target); }}
                          >
                            <Unplug className="h-4 w-4" />
                          </Button>
                        </div>
                      ) : <span className="text-muted-foreground">-</span>}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : <p className="p-5 text-sm text-muted-foreground">当前虚拟机没有磁盘设备。</p>}

        <div className="mx-4 mb-4 flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning" data-testid="vm-disk-resize-hint">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <p>扩容会调整宿主机侧 qcow2/raw 卷；运行中的虚拟机会同步扩展块设备。之后仍须在客户机内扩展分区和文件系统（Linux 使用 growpart 与 resize2fs/xfs_growfs，Windows 使用磁盘管理）。不支持缩容。</p>
        </div>

        {notice ? (
          <p
            className={notice.tone === "success" ? "mx-4 mb-4 rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success" : "mx-4 mb-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"}
            role="status"
            data-testid="vm-disk-resize-notice"
          >
            <span className="inline-flex items-center gap-2">
              {notice.tone === "success" ? <CheckCircle2 className="h-3.5 w-3.5" /> : <AlertTriangle className="h-3.5 w-3.5" />}
              {notice.text}
            </span>
          </p>
        ) : null}
      </CardContent>

      <AlertDialog open={selected !== null} onOpenChange={(open) => { if (!open && !mutation.isPending) { setSelected(null); setFormError(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>扩容磁盘 {selected?.target}？</AlertDialogTitle>
            <AlertDialogDescription>
              libvirt 存储卷将从 {selected ? formatBytes(selected.capacityBytes) : "-"} 扩容。本操作只增大容量，不能撤销。
              {vm.state === "running" || vm.state === "paused" || vm.state === "blocked" || vm.state === "suspended" ? " 虚拟机运行中会同步扩展块设备。" : " 虚拟机当前未运行，将在下次启动时使用新容量。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-2">
            <Label htmlFor="vm-disk-resize-capacity">新容量 (GiB)</Label>
            <Input
              id="vm-disk-resize-capacity"
              data-testid="vm-disk-resize-input"
              type="number"
              min={0.001}
              step={0.001}
              value={capacityGiB}
              onChange={(event) => { setCapacityGiB(event.target.value); setFormError(""); }}
              disabled={mutation.isPending}
              autoFocus
            />
            {selected ? <p className="text-xs text-muted-foreground">当前容量 {formatBytes(selected.capacityBytes)}，已分配 {formatBytes(selected.allocationBytes)}。qcow2 扩容后仍按需分配宿主机空间。</p> : null}
          </div>
          {formError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert" data-testid="vm-disk-resize-error">{formError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={mutation.isPending}>取消</AlertDialogCancel>
            <Button data-testid="vm-disk-resize-submit" disabled={mutation.isPending || !capacityGiB.trim()} onClick={submitResize}>
              {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Maximize2 className="h-4 w-4" />}
              {mutation.isPending ? "正在扩容..." : "确认扩容"}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Modal open={attachOpen} onClose={() => { if (!attachMutation.isPending) setAttachOpen(false); }} title="附加存储卷" description="从活跃存储池选择 qcow2/raw 卷。运行中的虚拟机使用热插拔，关机虚拟机写入持久配置。" testId="vm-disk-attach-modal" footer={<><Button variant="ghost" disabled={attachMutation.isPending} onClick={() => setAttachOpen(false)}>取消</Button><Button data-testid="vm-disk-attach-submit" disabled={attachMutation.isPending || !selectedPool || !selectedVolume} onClick={() => attachMutation.mutate()}>{attachMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}{attachMutation.isPending ? "正在附加..." : "确认附加"}</Button></>}>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="vm-disk-attach-pool">存储池</Label>
            <select id="vm-disk-attach-pool" data-testid="vm-disk-attach-pool" value={selectedPool} onChange={(event) => setPool(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
              {activePools.map((item) => <option key={item.name} value={item.name}>{item.name} · {formatBytes(item.availableBytes)} 可用</option>)}
            </select>
            {!activePools.length && !poolsQuery.isLoading ? <p className="text-xs text-destructive">没有可用活跃存储池。</p> : null}
          </div>
          <div className="space-y-2">
            <Label htmlFor="vm-disk-attach-volume">存储卷</Label>
            <select id="vm-disk-attach-volume" data-testid="vm-disk-attach-volume" value={selectedVolume} onChange={(event) => setVolume(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
              {eligibleVolumes.map((item) => <option key={item.name} value={item.name}>{item.name} · {item.format} · {formatBytes(item.capacityBytes)}</option>)}
            </select>
            {!eligibleVolumes.length && selectedPool && !volumesQuery.isLoading ? <p className="text-xs text-muted-foreground">该存储池没有 qcow2/raw 卷。</p> : null}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="vm-disk-attach-bus">磁盘总线</Label><select id="vm-disk-attach-bus" data-testid="vm-disk-attach-bus" value={bus} onChange={(event) => setBus(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm"><option value="virtio">virtio</option><option value="sata">sata</option><option value="scsi">scsi</option></select></div>
            <div className="space-y-2"><Label htmlFor="vm-disk-attach-target">目标设备（可选）</Label><Input id="vm-disk-attach-target" data-testid="vm-disk-attach-target" value={target} onChange={(event) => setTarget(event.target.value)} placeholder="留空自动分配，例如 vdb" /></div>
          </div>
          {attachError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{attachError}</p> : null}
        </div>
      </Modal>

      <AlertDialog open={detachTarget !== null} onOpenChange={(open) => { if (!open && !detachMutation.isPending) setDetachTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>移除磁盘 {detachTarget}？</AlertDialogTitle>
            <AlertDialogDescription>将从 <span className="font-medium text-foreground">{vm.name}</span> 移除该磁盘设备。存储卷本身不会被删除；运行中的虚拟机将尝试热拔除。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={detachMutation.isPending}>取消</AlertDialogCancel>
            <Button variant="destructive" data-testid="vm-disk-detach-submit" disabled={detachMutation.isPending} onClick={() => { if (detachTarget) detachMutation.mutate(detachTarget); }}>
              {detachMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Unplug className="h-4 w-4" />}确认移除
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
