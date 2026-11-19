import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, Clock3, RefreshCw, ShieldCheck, UnlockKeyhole } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { DataPlaneOperation } from "@/api/types";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { queryKeys } from "@/lib/query-keys";

const kindLabels: Record<string, string> = {
  "vm.backup": "虚拟机备份",
  "vm.backup.restore": "从备份恢复虚拟机",
  "vm.clone": "克隆虚拟机",
  "vm.disk.migrate": "迁移虚拟机磁盘",
  "vm.import": "导入虚拟机",
  "storage.volume.convert": "转换存储卷",
  "storage.volume.flatten": "展平存储卷",
};

const statusLabels: Record<DataPlaneOperation["status"], string> = {
  running: "执行中",
  succeeded: "已完成",
  failed: "恢复失败",
  recovered: "已恢复",
  needs_attention: "需人工处理",
};

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
});

function formatTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date);
}

function statusVariant(status: DataPlaneOperation["status"]) {
  if (status === "needs_attention" || status === "failed") return "destructive" as const;
  if (status === "recovered") return "success" as const;
  if (status === "running") return "warning" as const;
  return "outline" as const;
}

function operationTarget(operation: DataPlaneOperation) {
  if (operation.sourcePool && operation.sourceVolume) return `${operation.sourcePool}/${operation.sourceVolume}`;
  if (operation.targetPool && operation.targetName) return `${operation.targetPool}/${operation.targetName}`;
  return operation.vmName || operation.vmUuid || operation.taskId || "-";
}

export function DataPlanePage() {
  const queryClient = useQueryClient();
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [unblockTarget, setUnblockTarget] = useState<DataPlaneOperation | null>(null);
  const query = useQuery({
    queryKey: queryKeys.dataPlaneOperations,
    queryFn: () => api.dataPlaneOperations(),
    refetchInterval: 10_000,
  });
  const operations = query.data?.items ?? [];
  const pending = useMemo(() => operations.filter((item) => item.status === "needs_attention" || item.status === "failed"), [operations]);
  const running = operations.filter((item) => item.status === "running").length;
  const recovered = operations.filter((item) => item.status === "recovered").length;

  const actionMutation = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "recheck" | "unblock" }) => action === "unblock"
      ? api.unblockDataPlaneOperation(id)
      : api.recheckDataPlaneOperation(id),
    onSuccess: (result, variables) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.dataPlaneOperations });
      void queryClient.invalidateQueries({ queryKey: queryKeys.tasks.root });
      setUnblockTarget(null);
      setNotice({
        tone: "success",
        text: `${variables.action === "unblock" ? "阻塞已解除" : "重新校验完成"}：${result.recovery.message || statusLabels[result.operation.status]}`,
      });
    },
    onError: (cause) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.dataPlaneOperations });
      setNotice({ tone: "error", text: cause instanceof ApiError ? cause.message : "操作失败，请稍后重试。" });
    },
  });

  return (
    <div data-testid="dataplane-page">
      <PageHeader
        title="数据面运维"
        breadcrumb="总览 / 数据面运维"
        description="查看崩溃恢复无法自动闭环的备份、克隆、导入和存储卷操作。解除阻塞前系统会重新核对 libvirt 实际状态。"
        actions={<Button variant="outline" size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}><RefreshCw className={query.isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />刷新</Button>}
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <Card><CardHeader className="pb-2"><CardDescription>需人工处理</CardDescription><CardTitle className="flex items-center gap-2 text-2xl"><AlertTriangle className="h-5 w-5 text-destructive" />{pending.length}</CardTitle></CardHeader></Card>
        <Card><CardHeader className="pb-2"><CardDescription>仍记录为执行中</CardDescription><CardTitle className="flex items-center gap-2 text-2xl"><Clock3 className="h-5 w-5 text-warning" />{running}</CardTitle></CardHeader></Card>
        <Card><CardHeader className="pb-2"><CardDescription>历史已恢复</CardDescription><CardTitle className="flex items-center gap-2 text-2xl"><CheckCircle2 className="h-5 w-5 text-success" />{recovered}</CardTitle></CardHeader></Card>
      </div>

      {notice ? <p role="status" data-testid="dataplane-notice" className={notice.tone === "success" ? "mt-4 rounded-md border border-success/30 bg-success/10 px-4 py-3 text-sm text-success" : "mt-4 rounded-md border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive"}>{notice.text}</p> : null}

      <Card className="mt-4">
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><ShieldCheck className="h-4 w-4 text-primary" />持久化操作日志</CardTitle>
          <CardDescription>解除阻塞不会强制关闭仍被虚拟机引用的磁盘。只有后端确认操作已生效、已回滚或无需处理时，资源锁才会释放。</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {query.isLoading ? <LoadingState label="正在读取数据面操作" /> : null}
          {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取数据面操作"} onRetry={() => void query.refetch()} className="m-4" /> : null}
          {!query.isLoading && !query.isError && operations.length === 0 ? (
            <div className="px-5 py-16 text-center" data-testid="dataplane-empty">
              <ShieldCheck className="mx-auto h-8 w-8 text-success" />
              <p className="mt-3 text-sm font-medium">没有数据面操作记录</p>
              <p className="mt-1 text-xs text-muted-foreground">当前没有备份、克隆、导入或存储卷恢复任务需要关注。</p>
            </div>
          ) : null}
          {operations.length ? (
            <>
              <div className="hidden overflow-x-auto md:block">
                <Table className="min-w-[1040px]">
                  <TableHeader><TableRow><TableHead>操作</TableHead><TableHead>状态</TableHead><TableHead>资源</TableHead><TableHead>阶段 / 原因</TableHead><TableHead>更新时间</TableHead><TableHead className="text-right">处理</TableHead></TableRow></TableHeader>
                  <TableBody>{operations.map((operation) => (
                    <TableRow key={operation.id} data-testid={`dataplane-row-${operation.id}`}>
                      <TableCell><p className="font-medium">{kindLabels[operation.kind] ?? operation.kind}</p><p className="mt-1 max-w-64 truncate font-mono text-[11px] text-muted-foreground" title={operation.id}>{operation.id}</p></TableCell>
                      <TableCell><Badge variant={statusVariant(operation.status)}>{statusLabels[operation.status]}</Badge></TableCell>
                      <TableCell><p className="max-w-56 break-words text-sm">{operation.vmName || operation.vmUuid || "-"}</p><p className="mt-1 max-w-56 truncate font-mono text-[11px] text-muted-foreground" title={operationTarget(operation)}>{operationTarget(operation)}</p></TableCell>
                      <TableCell><p className="font-mono text-xs">{operation.phase || "-"}</p>{operation.error ? <p className="mt-1 max-w-80 text-xs leading-relaxed text-destructive">{operation.error}</p> : null}</TableCell>
                      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{formatTime(operation.updatedAt)}</TableCell>
                      <TableCell><OperationActions operation={operation} pending={actionMutation.isPending} onRecheck={() => { setNotice(null); actionMutation.mutate({ id: operation.id, action: "recheck" }); }} onUnblock={() => setUnblockTarget(operation)} /></TableCell>
                    </TableRow>
                  ))}</TableBody>
                </Table>
              </div>
              <div className="divide-y md:hidden">{operations.map((operation) => (
                <article key={operation.id} className="space-y-3 p-4" data-testid={`dataplane-card-${operation.id}`}>
                  <div className="flex items-start justify-between gap-3"><div><p className="font-medium">{kindLabels[operation.kind] ?? operation.kind}</p><p className="mt-1 break-all font-mono text-[10px] text-muted-foreground">{operation.id}</p></div><Badge variant={statusVariant(operation.status)}>{statusLabels[operation.status]}</Badge></div>
                  <div className="grid gap-1 text-xs"><p><span className="text-muted-foreground">资源：</span>{operation.vmName || operationTarget(operation)}</p><p><span className="text-muted-foreground">阶段：</span><span className="font-mono">{operation.phase || "-"}</span></p><p className="text-muted-foreground">更新于 {formatTime(operation.updatedAt)}</p></div>
                  {operation.error ? <p className="rounded-md bg-destructive/5 p-2 text-xs leading-relaxed text-destructive">{operation.error}</p> : null}
                  <OperationActions operation={operation} pending={actionMutation.isPending} onRecheck={() => { setNotice(null); actionMutation.mutate({ id: operation.id, action: "recheck" }); }} onUnblock={() => setUnblockTarget(operation)} />
                </article>
              ))}</div>
            </>
          ) : null}
        </CardContent>
      </Card>

      <AlertDialog open={unblockTarget !== null} onOpenChange={(open) => { if (!open) setUnblockTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>解除数据面资源阻塞？</AlertDialogTitle>
            <AlertDialogDescription>
              系统会先重新检查 <span className="font-mono text-foreground">{unblockTarget?.kind}</span> 的实际状态。只有在目标卷未被任何虚拟机引用、操作已生效或已安全回滚时才会解除；否则将保留阻塞并显示原因。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <Button data-testid="dataplane-confirm-unblock" disabled={actionMutation.isPending} onClick={() => { if (unblockTarget) actionMutation.mutate({ id: unblockTarget.id, action: "unblock" }); }}>
              <UnlockKeyhole className="h-4 w-4" />{actionMutation.isPending ? "正在校验..." : "重新校验并解除"}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function OperationActions({ operation, pending, onRecheck, onUnblock }: {
  operation: DataPlaneOperation;
  pending: boolean;
  onRecheck: () => void;
  onUnblock: () => void;
}) {
  const actionable = operation.status === "needs_attention" || operation.status === "failed";
  if (!actionable) return <span className="block text-right text-xs text-muted-foreground">无需处理</span>;
  return (
    <div className="flex flex-wrap justify-end gap-2">
      <Button variant="outline" size="sm" disabled={pending} data-testid={`dataplane-recheck-${operation.id}`} onClick={onRecheck}><RefreshCw className="h-3.5 w-3.5" />重新校验</Button>
      <Button size="sm" disabled={pending} data-testid={`dataplane-unblock-${operation.id}`} onClick={onUnblock}><UnlockKeyhole className="h-3.5 w-3.5" />解除阻塞</Button>
    </div>
  );
}
