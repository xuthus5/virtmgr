import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Camera, CheckCircle2, LoaderCircle, Plus, RotateCcw, Trash2 } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { VMSnapshotCreateInput } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { StatusBadge } from "@/components/feedback/status-badge";
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
import { overviewQueryKey } from "@/features/overview/use-overview-query";
import { useVMSnapshotsQuery, vmQueryKey, vmSnapshotsQueryKey } from "@/features/vms/use-vm-query";

const snapshotTimeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

function formatSnapshotTime(seconds: number) {
  if (!seconds) return "-";
  const date = new Date(seconds * 1000);
  return Number.isNaN(date.getTime()) ? "-" : snapshotTimeFormatter.format(date);
}

function initialForm(): VMSnapshotCreateInput {
  return { name: "", description: "" };
}

export function VMSnapshotsCard({ uuid, canMutate, disabledReason }: {
  uuid: string;
  canMutate: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const snapshotsQuery = useVMSnapshotsQuery(uuid);
  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<VMSnapshotCreateInput>(initialForm);
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [revertTarget, setRevertTarget] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [actionError, setActionError] = useState("");

  function invalidateSnapshots() {
    void queryClient.invalidateQueries({ queryKey: vmSnapshotsQueryKey(uuid) });
  }

  const createMutation = useMutation({
    mutationFn: (input: VMSnapshotCreateInput) => api.createVMSnapshot(uuid, input),
    onSuccess: (snapshot) => {
      setNotice({ tone: "success", text: `快照 ${snapshot.name} 已创建。` });
      setFormError("");
      setCreateOpen(false);
      setForm(initialForm());
      invalidateSnapshots();
    },
    onError: (cause) => {
      setFormError(cause instanceof ApiError ? cause.message : "创建快照失败，请稍后重试。");
    },
  });

  const revertMutation = useMutation({
    mutationFn: (name: string) => api.revertVMSnapshot(uuid, name),
    onSuccess: (_result, name) => {
      setNotice({ tone: "success", text: `快照 ${name} 已回滚，虚拟机状态已刷新。` });
      setRevertTarget(null);
      setActionError("");
      invalidateSnapshots();
      void queryClient.invalidateQueries({ queryKey: vmQueryKey(uuid) });
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
    },
    onError: (cause) => {
      setActionError(cause instanceof ApiError ? cause.message : "回滚快照失败，请稍后重试。");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteVMSnapshot(uuid, name),
    onSuccess: (_result, name) => {
      setNotice({ tone: "success", text: `快照 ${name} 已删除。` });
      setDeleteTarget(null);
      setActionError("");
      invalidateSnapshots();
    },
    onError: (cause) => {
      setActionError(cause instanceof ApiError ? cause.message : "删除快照失败，请稍后重试。");
    },
  });

  function submitCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canMutate) return;
    const name = form.name.trim();
    if (!name) {
      setFormError("请输入快照名称。");
      return;
    }
    if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/.test(name)) {
      setFormError("快照名称需以字母或数字开头，仅允许字母、数字、点、下划线和短横线，最长 64 个字符。");
      return;
    }
    if ((form.description ?? "").length > 1024) {
      setFormError("快照描述不能超过 1024 个字符。");
      return;
    }
    setFormError("");
    setNotice(null);
    createMutation.mutate({ name, description: form.description?.trim() });
  }

  const snapshots = snapshotsQuery.data?.items ?? [];

  return (
    <>
      <Card className="mt-4" data-testid="vm-snapshots-card">
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>快照</CardTitle>
              <CardDescription>查看 libvirt 域快照时间线。回滚会恢复磁盘状态，创建与回滚可能受磁盘格式和虚拟机运行状态限制。</CardDescription>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              data-testid="vm-snapshot-create-toggle"
              disabled={!canMutate || createMutation.isPending}
              title={disabledReason || "创建虚拟机快照"}
              onClick={() => { setCreateOpen((open) => !open); setFormError(""); setNotice(null); }}
            >
              <Plus className="h-4 w-4" />创建快照
            </Button>
          </div>
        </CardHeader>

        {createOpen && canMutate ? (
          <form onSubmit={submitCreate} className="border-y bg-muted/20 p-4" data-testid="vm-snapshot-create-form">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="vm-snapshot-name">快照名称</Label>
                <Input id="vm-snapshot-name" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder="例如 before-upgrade" autoComplete="off" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-snapshot-description">描述（可选）</Label>
                <Input id="vm-snapshot-description" value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} placeholder="记录创建原因或变更窗口" autoComplete="off" />
              </div>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
              <p className="text-xs text-muted-foreground">快照由 libvirt 创建并以其为唯一事实来源。qcow2 磁盘通常支持内部快照。</p>
              <div className="flex items-center gap-2">
                <Button type="button" variant="ghost" size="sm" onClick={() => { setCreateOpen(false); setFormError(""); }}>取消</Button>
                <Button type="submit" size="sm" data-testid="vm-snapshot-create-submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Camera className="h-4 w-4" />}
                  {createMutation.isPending ? "创建中..." : "创建快照"}
                </Button>
              </div>
            </div>
            {formError ? (
              <p role="alert" className="mt-3 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">
                <AlertTriangle className="h-3.5 w-3.5" />{formError}
              </p>
            ) : null}
          </form>
        ) : null}

        {notice ? (
          <div className={`border-b px-4 py-2 text-xs ${notice.tone === "success" ? "bg-success/10 text-success" : "bg-destructive/5 text-destructive"}`} role="status" data-testid="vm-snapshot-notice">
            <span className="inline-flex items-center gap-2">{notice.tone === "success" ? <CheckCircle2 className="h-3.5 w-3.5" /> : <AlertTriangle className="h-3.5 w-3.5" />}{notice.text}</span>
          </div>
        ) : null}

        <CardContent className="p-0">
          {snapshotsQuery.isLoading ? <LoadingState label="正在读取虚拟机快照" /> : null}
          {snapshotsQuery.isError ? <ErrorState message={snapshotsQuery.error instanceof Error ? snapshotsQuery.error.message : "无法读取虚拟机快照"} onRetry={() => void snapshotsQuery.refetch()} className="m-4" /> : null}
          {snapshotsQuery.data && snapshots.length === 0 ? <p className="p-5 text-sm text-muted-foreground" data-testid="vm-snapshot-empty">暂无快照。创建快照后可按时间线查看和回滚。</p> : null}
          {snapshots.length ? (
            <Table className="min-w-[800px]">
              <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>创建时间</TableHead><TableHead>快照状态</TableHead><TableHead>当前</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {snapshots.map((snapshot) => (
                  <TableRow key={snapshot.name} data-testid={`vm-snapshot-row-${snapshot.name}`}>
                    <TableCell>
                      <p className="font-medium">{snapshot.name}</p>
                      <p className="max-w-80 truncate text-xs text-muted-foreground" title={snapshot.description}>{snapshot.description || "无描述"}</p>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatSnapshotTime(snapshot.createdAt)}</TableCell>
                    <TableCell><StatusBadge state={snapshot.state} /></TableCell>
                    <TableCell>{snapshot.current ? <Badge variant="success">当前</Badge> : <span className="text-xs text-muted-foreground">-</span>}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        <Button type="button" variant="ghost" size="icon-sm" data-testid={`vm-snapshot-revert-${snapshot.name}`} aria-label={`回滚快照 ${snapshot.name}`} disabled={!canMutate || revertMutation.isPending} title={canMutate ? "回滚到该快照" : disabledReason} onClick={() => { setActionError(""); setRevertTarget(snapshot.name); }}>
                          <RotateCcw className="h-4 w-4" />
                        </Button>
                        <Button type="button" variant="ghost" size="icon-sm" data-testid={`vm-snapshot-delete-${snapshot.name}`} aria-label={`删除快照 ${snapshot.name}`} disabled={!canMutate || deleteMutation.isPending} title={canMutate ? "删除该快照" : disabledReason} onClick={() => { setActionError(""); setDeleteTarget(snapshot.name); }}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : null}
        </CardContent>
      </Card>

      <AlertDialog open={revertTarget !== null} onOpenChange={(open) => { if (!open) { setRevertTarget(null); setActionError(""); } }}>
        <AlertDialogContent data-testid="vm-snapshot-revert-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>回滚到快照？</AlertDialogTitle>
            <AlertDialogDescription>虚拟机将恢复到快照 <span className="font-medium text-foreground">{revertTarget}</span> 的状态，当前状态和快照之后的磁盘变更可能丢失。此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          {actionError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{actionError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button data-testid="vm-snapshot-confirm-revert" disabled={revertMutation.isPending} onClick={() => { if (!revertTarget) return; setActionError(""); revertMutation.mutate(revertTarget); }}>
              {revertMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}确认回滚
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setActionError(""); } }}>
        <AlertDialogContent data-testid="vm-snapshot-delete-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>删除快照？</AlertDialogTitle>
            <AlertDialogDescription>将删除快照 <span className="font-medium text-foreground">{deleteTarget}</span> 及其 libvirt 元数据。存在子快照时删除会失败。</AlertDialogDescription>
          </AlertDialogHeader>
          {actionError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{actionError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button variant="destructive" data-testid="vm-snapshot-confirm-delete" disabled={deleteMutation.isPending} onClick={() => { if (!deleteTarget) return; setActionError(""); deleteMutation.mutate(deleteTarget); }}>
              {deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}确认删除
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
