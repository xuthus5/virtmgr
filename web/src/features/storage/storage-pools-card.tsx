import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Info, LoaderCircle, Play, Plus, RotateCw, Square, Trash2 } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { StoragePool, StoragePoolAction, StoragePoolCreateInput, StoragePoolState } from "@/api/types";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { storagePoolsQueryKey } from "@/features/storage/use-storage-query";
import { cn } from "@/lib/utils";

type CreatePoolFormState = {
  name: string;
  targetPath: string;
  autostart: boolean;
  start: boolean;
};

const initialCreatePoolForm: CreatePoolFormState = { name: "", targetPath: "", autostart: false, start: true };

const poolStateLabel: Record<StoragePoolState, string> = {
  active: "活跃",
  inactive: "未激活",
  degraded: "降级",
  inaccessible: "不可访问",
  unknown: "未知",
};

function poolStateVariant(state: StoragePoolState): "success" | "secondary" | "warning" | "destructive" | "default" {
  switch (state) {
    case "active":
      return "success";
    case "inactive":
      return "secondary";
    case "degraded":
      return "warning";
    case "inaccessible":
      return "destructive";
    default:
      return "default";
  }
}

const actionLabels: Record<StoragePoolAction, { noun: string; done: string }> = {
  start: { noun: "启动", done: "已启动" },
  stop: { noun: "停止", done: "已停止" },
  refresh: { noun: "刷新", done: "已刷新" },
};

function poolActionHint(canManage: boolean, manageDisabledReason: string, pool: StoragePool) {
  if (!canManage) return manageDisabledReason;
  if (pool.state === "active") return "运行中：刷新用于同步卷列表；停止后才能删除存储池定义。";
  if (pool.state === "inactive") return "未激活：启动后可以管理卷；删除仅移除定义，不会删除磁盘文件。";
  return "当前状态不支持启停或删除，请检查目标路径和宿主机权限。";
}

export function StoragePoolsCard({
  pools,
  selectedPool,
  canManage,
  manageDisabledReason,
  onSelect,
  onNotice,
}: {
  pools: StoragePool[];
  selectedPool: string;
  canManage: boolean;
  manageDisabledReason: string;
  onSelect: (name: string) => void;
  onNotice: (message: string, testId: string) => void;
}) {
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreatePoolFormState>(initialCreatePoolForm);
  const [createError, setCreateError] = useState("");
  const [actionError, setActionError] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<StoragePool | null>(null);
  const [deleteName, setDeleteName] = useState("");
  const [deleteError, setDeleteError] = useState("");

  async function refreshPoolViews(name?: string) {
    await queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    if (name) await queryClient.invalidateQueries({ queryKey: ["storage", "volumes", name] });
  }

  const createMutation = useMutation({
    mutationFn: (input: StoragePoolCreateInput) => api.createStoragePool(input),
    onSuccess: async (pool) => {
      setCreateError("");
      setActionError("");
      setCreateOpen(false);
      setCreateForm(initialCreatePoolForm);
      onSelect(pool.name);
      onNotice(`存储池 ${pool.name} 已创建。`, "storage-pool-success");
      await refreshPoolViews(pool.name);
    },
    onError: (cause) => {
      setCreateError(cause instanceof ApiError ? cause.message : "创建存储池失败，请稍后重试。");
    },
  });

  const actionMutation = useMutation({
    mutationFn: ({ name, action }: { name: string; action: StoragePoolAction }) => api.storagePoolAction(name, action),
    onSuccess: async (_, variables) => {
      setActionError("");
      onNotice(`存储池 ${variables.name} ${actionLabels[variables.action].done}。`, "storage-pool-success");
      await refreshPoolViews(variables.name);
    },
    onError: (cause) => {
      setActionError(cause instanceof ApiError ? cause.message : "存储池操作失败，请稍后重试。");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.deleteStoragePool(name),
    onSuccess: async (_, name) => {
      setDeleteTarget(null);
      setDeleteName("");
      setDeleteError("");
      setActionError("");
      onNotice(`存储池 ${name} 已删除。`, "storage-pool-success");
      await refreshPoolViews(name);
    },
    onError: (cause) => {
      setDeleteError(cause instanceof ApiError ? cause.message : "删除存储池失败，请稍后重试。");
    },
  });

  function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage) return;
    const targetPath = createForm.targetPath.trim();
    if (!targetPath.startsWith("/")) {
      setCreateError("目标路径必须是绝对路径。");
      return;
    }
    setCreateError("");
    createMutation.mutate({
      name: createForm.name.trim(),
      type: "dir",
      targetPath,
      autostart: createForm.autostart,
      start: createForm.start,
    });
  }

  function runAction(pool: StoragePool, action: StoragePoolAction) {
    setActionError("");
    actionMutation.mutate({ name: pool.name, action });
  }

  function openDelete(pool: StoragePool) {
    setDeleteTarget(pool);
    setDeleteName("");
    setDeleteError("");
  }

  return (
    <>
      <Card data-testid="storage-pools-card">
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>存储池</CardTitle>
              <CardDescription>点击任意存储池，在下方查看其卷列表。活跃池可刷新或停止，未激活池可启动或删除定义。创建仅支持 libvirt 目录池（dir）。</CardDescription>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              data-testid="storage-create-pool"
              disabled={!canManage || createMutation.isPending}
              title={canManage ? "创建目录类型存储池" : manageDisabledReason}
              onClick={() => { setCreateOpen((open) => !open); setCreateError(""); setActionError(""); }}
            >
              <Plus className="h-4 w-4" />新建存储池
            </Button>
          </div>
        </CardHeader>
        {createOpen ? (
          <form onSubmit={handleCreate} className="border-y bg-muted/20 p-4" data-testid="storage-pool-create-form">
            <div className="grid gap-3 lg:grid-cols-[0.8fr_1.4fr]">
              <div className="space-y-2">
                <Label htmlFor="storage-pool-name">存储池名称</Label>
                <Input id="storage-pool-name" data-testid="storage-pool-name" value={createForm.name} onChange={(event) => setCreateForm((form) => ({ ...form, name: event.target.value }))} placeholder="例如 data-pool" required autoComplete="off" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="storage-pool-target-path">目标路径</Label>
                <Input id="storage-pool-target-path" data-testid="storage-pool-target-path" value={createForm.targetPath} onChange={(event) => setCreateForm((form) => ({ ...form, targetPath: event.target.value }))} placeholder="/srv/virtmgr/data" required autoComplete="off" className="font-mono" />
              </div>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-5 text-sm">
              <label className="flex items-center gap-2" htmlFor="storage-pool-autostart">
                <input id="storage-pool-autostart" type="checkbox" checked={createForm.autostart} onChange={(event) => setCreateForm((form) => ({ ...form, autostart: event.target.checked }))} className="h-4 w-4 rounded border-input" />
                启用开机自启动
              </label>
              <label className="flex items-center gap-2" htmlFor="storage-pool-start">
                <input id="storage-pool-start" type="checkbox" checked={createForm.start} onChange={(event) => setCreateForm((form) => ({ ...form, start: event.target.checked }))} className="h-4 w-4 rounded border-input" />
                创建后立即启动
              </label>
            </div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
              <p className="text-xs text-muted-foreground">目录必须已存在于宿主机，或由 libvirt 启动池时成功访问；VirtMgr 不直接创建宿主机目录。</p>
              <div className="flex items-center gap-2">
                <Button type="button" variant="ghost" size="sm" onClick={() => { setCreateOpen(false); setCreateError(""); }}>取消</Button>
                <Button type="submit" size="sm" data-testid="storage-pool-create-submit" disabled={createMutation.isPending}>{createMutation.isPending ? "创建中..." : "创建存储池"}</Button>
              </div>
            </div>
            {createError ? <p role="alert" className="mt-3 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"><AlertTriangle className="h-3.5 w-3.5" />{createError}</p> : null}
          </form>
        ) : null}
        {actionError ? <p role="alert" className="m-4 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"><AlertTriangle className="h-3.5 w-3.5" />{actionError}</p> : null}
        <CardContent className="p-0">
          <Table>
            <TableHeader><TableRow><TableHead>名称</TableHead><TableHead>类型</TableHead><TableHead>状态</TableHead><TableHead>目标路径</TableHead><TableHead>卷</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
            <TableBody>
              {pools.map((pool) => {
                const active = pool.state === "active";
                const inactive = pool.state === "inactive";
                const pending = actionMutation.isPending || deleteMutation.isPending;
                const hint = poolActionHint(canManage, manageDisabledReason, pool);
                const hintID = `storage-pool-action-hint-${pool.name}`;
                return (
                  <TableRow key={pool.uuid || pool.name} className={cn("cursor-pointer", selectedPool === pool.name && "bg-primary/5")} onClick={() => onSelect(pool.name)} data-testid={`storage-pool-row-${pool.name}`}>
                    <TableCell>
                      <button type="button" className={cn("text-left font-medium hover:text-primary", selectedPool === pool.name && "text-primary")} onClick={() => onSelect(pool.name)}>
                        {pool.name}
                      </button>
                      <p className="font-mono text-[10px] text-muted-foreground">{pool.uuid || "-"}</p>
                    </TableCell>
                    <TableCell><Badge variant="outline">{pool.type || "-"}</Badge></TableCell>
                    <TableCell><Badge variant={poolStateVariant(pool.state)}>{poolStateLabel[pool.state]}</Badge></TableCell>
                    <TableCell><span className="block max-w-64 truncate font-mono text-xs text-muted-foreground" title={pool.targetPath}>{pool.targetPath || "-"}</span></TableCell>
                    <TableCell className="tabular-nums" data-testid={`storage-pool-volumes-${pool.name}`}>{pool.volumeCount}</TableCell>
                    <TableCell className="min-w-60 align-top">
                      <div className="flex flex-col gap-2" onClick={(event) => event.stopPropagation()}>
                        <div className="flex flex-wrap items-center gap-1.5">
                          {active ? (
                            <>
                              <Button type="button" variant="outline" size="sm" data-testid={`storage-pool-refresh-${pool.name}`} aria-label={`刷新存储池 ${pool.name}`} aria-describedby={hintID} title={canManage ? "重新扫描存储池，刷新卷列表和容量信息" : manageDisabledReason} disabled={!canManage || pending} onClick={() => runAction(pool, "refresh")}>
                                <RotateCw className="h-3.5 w-3.5" />刷新
                              </Button>
                              <Button type="button" variant="outline" size="sm" data-testid={`storage-pool-stop-${pool.name}`} aria-label={`停止存储池 ${pool.name}`} aria-describedby={hintID} title={canManage ? "停止存储池；停止后才能删除其定义" : manageDisabledReason} disabled={!canManage || pending} onClick={() => runAction(pool, "stop")}>
                                <Square className="h-3.5 w-3.5" />停止
                              </Button>
                            </>
                          ) : null}
                          {inactive ? (
                            <>
                              <Button type="button" variant="outline" size="sm" data-testid={`storage-pool-start-${pool.name}`} aria-label={`启动存储池 ${pool.name}`} aria-describedby={hintID} title={canManage ? "启动存储池并使其中的卷可用" : manageDisabledReason} disabled={!canManage || pending} onClick={() => runAction(pool, "start")}>
                                <Play className="h-3.5 w-3.5" />启动
                              </Button>
                              <Button type="button" variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" data-testid={`storage-pool-delete-${pool.name}`} aria-label={`删除存储池 ${pool.name}`} aria-describedby={hintID} title={canManage ? "删除存储池定义，不会删除磁盘文件" : manageDisabledReason} disabled={!canManage || pending} onClick={() => openDelete(pool)}>
                                <Trash2 className="h-3.5 w-3.5" />删除
                              </Button>
                            </>
                          ) : null}
                          {!active && !inactive ? <span className="text-xs text-muted-foreground">当前状态不支持操作</span> : null}
                        </div>
                        <p id={hintID} data-testid={`storage-pool-action-hint-${pool.name}`} className="flex items-start gap-1.5 text-[11px] leading-relaxed text-muted-foreground">
                          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />{hint}
                        </p>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setDeleteError(""); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除存储池？</AlertDialogTitle>
            <AlertDialogDescription>
              将删除存储池 <span className="font-medium text-foreground">{deleteTarget?.name}</span> 的 libvirt 定义。池内磁盘文件不会自动删除，请先确认没有虚拟机依赖该目录。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-2">
            <Label htmlFor="delete-pool-name">输入存储池名称确认</Label>
            <Input id="delete-pool-name" value={deleteName} onChange={(event) => setDeleteName(event.target.value)} placeholder={deleteTarget?.name} autoComplete="off" />
          </div>
          {deleteError ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive" role="alert">{deleteError}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button variant="destructive" data-testid="confirm-delete-pool" disabled={!deleteTarget || deleteName !== deleteTarget.name || deleteMutation.isPending} onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget.name); }}>
              {deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}
              确认删除
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
