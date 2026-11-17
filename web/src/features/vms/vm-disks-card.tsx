import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Disc3, Gauge, HardDrive, LoaderCircle, Maximize2, Plus, Unplug } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { Disk, Role, VM, VMDiskQoSInput } from "@/api/types";
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
  const [isoOpen, setIsoOpen] = useState(false);
  const [isoPool, setIsoPool] = useState("");
  const [isoVolume, setIsoVolume] = useState("");
  const [isoError, setIsoError] = useState("");
  const [detachTarget, setDetachTarget] = useState<string | null>(null);
  const [ejectTarget, setEjectTarget] = useState<Disk | null>(null);
  const [qosDisk, setQosDisk] = useState<Disk | null>(null);
  const [qosTotal, setQosTotal] = useState("");
  const [qosRead, setQosRead] = useState("");
  const [qosWrite, setQosWrite] = useState("");
  const [qosTotalIOPS, setQosTotalIOPS] = useState("");
  const [qosReadIOPS, setQosReadIOPS] = useState("");
  const [qosWriteIOPS, setQosWriteIOPS] = useState("");
  const [qosError, setQosError] = useState("");
  const poolsQuery = useStoragePoolsQuery(attachOpen);
  const activePools = (poolsQuery.data?.items ?? []).filter((item) => item.state === "active");
  const selectedPool = pool || activePools[0]?.name || "";
  const volumesQuery = useStorageVolumesQuery(selectedPool);
  const eligibleVolumes = (volumesQuery.data?.items ?? []).filter((item) => item.format === "qcow2" || item.format === "raw");
  const selectedVolume = eligibleVolumes.some((item) => item.name === volume) ? volume : eligibleVolumes[0]?.name || "";
  const rootTarget = vm.disks.find((disk) => disk.device === "disk" && !disk.readOnly && disk.source)?.target || "";
  const isoPoolsQuery = useStoragePoolsQuery(isoOpen);
  const isoActivePools = (isoPoolsQuery.data?.items ?? []).filter((item) => item.state === "active");
  const selectedISOPool = isoPool || isoActivePools[0]?.name || "";
  const isoVolumesQuery = useStorageVolumesQuery(selectedISOPool);
  const isoVolumes = (isoVolumesQuery.data?.items ?? []).filter((item) => /\.iso$/i.test(item.name) && item.path?.startsWith("/"));
  const selectedISOVolume = isoVolumes.some((item) => item.name === isoVolume) ? isoVolume : isoVolumes[0]?.name || "";
  const isoDisabledReason = readOnly
    ? "系统当前处于只读模式"
    : !role || role === "viewer"
      ? "当前账号没有虚拟机操作权限"
      : !vm.persistent
        ? "临时虚拟机不能挂载 ISO"
        : vm.state !== "shutoff" && vm.state !== "crashed"
          ? "请先关闭虚拟机再挂载 ISO"
          : "";

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

  const attachISOMutation = useMutation({
    mutationFn: () => api.attachVMISO(vm.uuid, { pool: selectedISOPool, volume: selectedISOVolume }),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
      setIsoOpen(false);
      setIsoVolume("");
      setIsoError("");
      setNotice({ tone: "success", text: `安装 ISO ${selectedISOVolume} 已挂载，并切换为光盘优先启动。` });
    },
    onError: (cause) => setIsoError(cause instanceof ApiError ? cause.message : "挂载 ISO 失败，请稍后重试。"),
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

  const ejectISOMutation = useMutation({
    mutationFn: () => api.ejectVMISO(vm.uuid),
    onSuccess: (updated) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "config-versions"] });
      setEjectTarget(null);
      setNotice({ tone: "success", text: "安装 ISO 已弹出，主磁盘已设为第一启动设备。" });
    },
    onError: (cause) => {
      setEjectTarget(null);
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "弹出 ISO 失败，请稍后重试。" });
    },
  });

  const qosMutation = useMutation({
    mutationFn: ({ target, input }: { target: string; input: VMDiskQoSInput }) => api.updateVMDiskQoS(vm.uuid, target, input),
    onSuccess: (updated, variables) => {
      queryClient.setQueryData(vmQueryKey(vm.uuid), updated);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "xml"] });
      void queryClient.invalidateQueries({ queryKey: ["vm", vm.uuid, "config-versions"] });
      setQosDisk(null);
      setNotice({ tone: "success", text: `${variables.target} 的磁盘 I/O 限制已更新。` });
    },
    onError: (cause) => setQosError(cause instanceof ApiError ? cause.message : "更新磁盘 I/O 限制失败，请稍后重试。"),
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

  function openQoS(disk: Disk) {
    setQosDisk(disk);
    setQosTotal(qosBytesToMiB(disk.qos?.totalBytesSec));
    setQosRead(qosBytesToMiB(disk.qos?.readBytesSec));
    setQosWrite(qosBytesToMiB(disk.qos?.writeBytesSec));
    setQosTotalIOPS(disk.qos?.totalIops ? String(disk.qos.totalIops) : "");
    setQosReadIOPS(disk.qos?.readIops ? String(disk.qos.readIops) : "");
    setQosWriteIOPS(disk.qos?.writeIops ? String(disk.qos.writeIops) : "");
    setQosError("");
    setNotice(null);
  }

  function submitQoS() {
    if (!qosDisk) return;
    const total = parseQoSMiB(qosTotal);
    const read = parseQoSMiB(qosRead);
    const write = parseQoSMiB(qosWrite);
    const totalIops = parseQoSIops(qosTotalIOPS);
    const readIops = parseQoSIops(qosReadIOPS);
    const writeIops = parseQoSIops(qosWriteIOPS);
    if ([total, read, write, totalIops, readIops, writeIops].some((value) => value === null)) {
      setQosError("请输入非负数值；带宽单位为 MiB/s，IOPS 必须为整数。全部留空或为 0 可清除限制。");
      return;
    }
    setQosError("");
    qosMutation.mutate({ target: qosDisk.target, input: { totalBytesSec: total ?? 0, readBytesSec: read ?? 0, writeBytesSec: write ?? 0, totalIops: totalIops ?? 0, readIops: readIops ?? 0, writeIops: writeIops ?? 0 } });
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
            <Button size="sm" variant="outline" data-testid="vm-iso-attach-open" disabled={Boolean(isoDisabledReason) || attachISOMutation.isPending} title={isoDisabledReason || "从活跃存储池挂载或替换安装 ISO"} onClick={() => { setIsoError(""); setNotice(null); setIsoOpen(true); }}>
              <Disc3 className="h-4 w-4" />挂载 ISO
            </Button>
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
                const installerISO = disk.device === "cdrom" && /\.iso$/i.test(disk.source || "") && !/-cidata\.iso$/i.test(disk.source || "");
                const ejectDisabledReason = readOnly
                  ? "系统当前处于只读模式"
                  : !role || role === "viewer"
                    ? "当前账号没有虚拟机操作权限"
                    : !vm.persistent
                      ? "临时虚拟机不能弹出 ISO"
                      : vm.state !== "shutoff" && vm.state !== "crashed"
                        ? "请先关闭虚拟机再弹出 ISO"
                        : "";
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
                      {installerISO ? (
                        <Button
                          variant="outline"
                          size="sm"
                          data-testid={`vm-iso-eject-${disk.target || index}`}
                          disabled={Boolean(ejectDisabledReason) || ejectISOMutation.isPending}
                          title={ejectDisabledReason || "移除安装 ISO，并切换为硬盘优先启动"}
                          onClick={() => { setNotice(null); setEjectTarget(disk); }}
                        >
                          {ejectISOMutation.isPending ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Disc3 className="h-3.5 w-3.5" />}弹出 ISO
                        </Button>
                      ) : disk.device === "disk" ? (
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
                            variant={disk.qos ? "outline" : "ghost"}
                            size="sm"
                            data-testid={`vm-disk-qos-${disk.target}`}
                            disabled={Boolean(disabledReason) || qosMutation.isPending}
                            title={disabledReason || `配置磁盘 ${disk.target} 的吞吐与 IOPS 限制`}
                            onClick={() => openQoS(disk)}
                          >
                            <Gauge className="h-3.5 w-3.5" />限速
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

      <Modal open={qosDisk !== null} onClose={() => { if (!qosMutation.isPending) setQosDisk(null); }} title={`磁盘 I/O 限制 · ${qosDisk?.target || ""}`} description="通过 libvirt iotune 限制磁盘总吞吐、读写吞吐和 IOPS。带宽输入单位为 MiB/s；全部清零可移除限制。" testId="vm-disk-qos-modal" footer={<><Button variant="ghost" disabled={qosMutation.isPending} onClick={() => setQosDisk(null)}>取消</Button><Button data-testid="vm-disk-qos-submit" disabled={qosMutation.isPending} onClick={submitQoS}>{qosMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Gauge className="h-4 w-4" />}{qosMutation.isPending ? "正在保存..." : "保存限制"}</Button></>}>
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2"><Label htmlFor="vm-disk-qos-total">总带宽 (MiB/s)</Label><Input id="vm-disk-qos-total" data-testid="vm-disk-qos-total" type="number" min={0} step="any" value={qosTotal} onChange={(event) => setQosTotal(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-disk-qos-read">读带宽 (MiB/s)</Label><Input id="vm-disk-qos-read" data-testid="vm-disk-qos-read" type="number" min={0} step="any" value={qosRead} onChange={(event) => setQosRead(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-disk-qos-write">写带宽 (MiB/s)</Label><Input id="vm-disk-qos-write" data-testid="vm-disk-qos-write" type="number" min={0} step="any" value={qosWrite} onChange={(event) => setQosWrite(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-disk-qos-total-iops">总 IOPS</Label><Input id="vm-disk-qos-total-iops" data-testid="vm-disk-qos-total-iops" type="number" min={0} step={1} value={qosTotalIOPS} onChange={(event) => setQosTotalIOPS(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-disk-qos-read-iops">读 IOPS</Label><Input id="vm-disk-qos-read-iops" data-testid="vm-disk-qos-read-iops" type="number" min={0} step={1} value={qosReadIOPS} onChange={(event) => setQosReadIOPS(event.target.value)} disabled={qosMutation.isPending} /></div>
            <div className="space-y-2"><Label htmlFor="vm-disk-qos-write-iops">写 IOPS</Label><Input id="vm-disk-qos-write-iops" data-testid="vm-disk-qos-write-iops" type="number" min={0} step={1} value={qosWriteIOPS} onChange={(event) => setQosWriteIOPS(event.target.value)} disabled={qosMutation.isPending} /></div>
          </div>
          {qosError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{qosError}</p> : null}
          <p className="text-xs text-muted-foreground">总限制与读写限制同时存在时，由 QEMU/libvirt 按各自维度共同约束；运行中的虚拟机将同步应用。</p>
        </div>
      </Modal>

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

      <Modal open={isoOpen} onClose={() => { if (!attachISOMutation.isPending) setIsoOpen(false); }} title="挂载安装 ISO" description="仅支持关机或崩溃状态的持久虚拟机。已有安装光盘会被替换，cloud-init 介质保持不变。" testId="vm-iso-attach-modal" footer={<><Button variant="ghost" disabled={attachISOMutation.isPending} onClick={() => setIsoOpen(false)}>取消</Button><Button data-testid="vm-iso-attach-submit" disabled={attachISOMutation.isPending || !selectedISOPool || !selectedISOVolume} onClick={() => attachISOMutation.mutate()}>{attachISOMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Disc3 className="h-4 w-4" />}{attachISOMutation.isPending ? "正在挂载..." : "确认挂载"}</Button></>}>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="vm-iso-attach-pool">存储池</Label>
            <select id="vm-iso-attach-pool" data-testid="vm-iso-attach-pool" value={selectedISOPool} onChange={(event) => { setIsoPool(event.target.value); setIsoVolume(""); }} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
              {isoActivePools.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
            </select>
            {!isoActivePools.length && !isoPoolsQuery.isLoading ? <p className="text-xs text-destructive">没有可用活跃存储池。</p> : null}
          </div>
          <div className="space-y-2">
            <Label htmlFor="vm-iso-attach-volume">ISO 存储卷</Label>
            <select id="vm-iso-attach-volume" data-testid="vm-iso-attach-volume" value={selectedISOVolume} onChange={(event) => setIsoVolume(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
              {isoVolumes.map((item) => <option key={item.name} value={item.name}>{item.name} · {formatBytes(item.capacityBytes)}</option>)}
            </select>
            {!isoVolumes.length && selectedISOPool && !isoVolumesQuery.isLoading ? <p className="text-xs text-muted-foreground">该存储池没有可用的 .iso 卷。</p> : null}
          </div>
          {isoError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{isoError}</p> : null}
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

      <AlertDialog open={ejectTarget !== null} onOpenChange={(open) => { if (!open && !ejectISOMutation.isPending) setEjectTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>弹出安装 ISO？</AlertDialogTitle>
            <AlertDialogDescription>将从 <span className="font-medium text-foreground">{vm.name}</span> 的持久配置移除 <span className="font-mono text-xs">{ejectTarget?.source}</span>，并把主磁盘调整为第一启动设备。ISO 文件与 cloud-init 介质不会被删除。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={ejectISOMutation.isPending}>取消</AlertDialogCancel>
            <Button data-testid="vm-iso-eject-submit" disabled={ejectISOMutation.isPending} onClick={() => ejectISOMutation.mutate()}>
              {ejectISOMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Disc3 className="h-4 w-4" />}确认弹出
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

function qosBytesToMiB(value?: number) {
  if (!value) return "";
  return String(Number((value / 1024 / 1024).toFixed(3)));
}

function parseQoSMiB(value: string): number | null {
  const trimmed = value.trim();
  if (!trimmed) return 0;
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed < 0 || !Number.isSafeInteger(Math.round(parsed * 1024 * 1024))) return null;
  return Math.round(parsed * 1024 * 1024);
}

function parseQoSIops(value: string): number | null {
  const trimmed = value.trim();
  if (!trimmed) return 0;
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) && parsed >= 0 ? parsed : null;
}
