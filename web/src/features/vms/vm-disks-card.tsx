import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, HardDrive, LoaderCircle, Maximize2 } from "lucide-react";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  defaultDiskCapacityGiB,
  validateDiskCapacity,
  vmDiskResizeDisabledReason,
} from "@/features/vms/vm-disk-resize-model";
import { vmQueryKey } from "@/features/vms/use-vm-query";
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { formatBytes } from "@/lib/utils";

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
          <Badge variant="outline">仅支持扩容</Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 p-0">
        {vm.disks.length ? (
          <Table>
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
    </Card>
  );
}
