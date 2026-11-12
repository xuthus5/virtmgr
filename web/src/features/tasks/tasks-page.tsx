import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { Ban, CheckCircle2, ChevronLeft, ChevronRight, CircleAlert, ListChecks, LoaderCircle, XCircle } from "lucide-react";
import type { Task, TaskStatus } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { useTasksQuery } from "@/features/tasks/use-tasks-query";
import { cn } from "@/lib/utils";

const pageSize = 20;

const statusLabels: Record<TaskStatus, string> = {
  queued: "排队中",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  interrupted: "已中断",
};

const kindLabels: Record<string, string> = {
  "vm.create": "创建虚拟机",
  "vm.clone": "克隆虚拟机",
  "vm.backup": "备份虚拟机",
  "vm.backup.verify": "校验虚拟机备份",
  "vm.backup.restore": "从备份恢复虚拟机",
  "vm.backup.delete": "删除虚拟机备份",
  "vm.delete": "删除虚拟机",
  "vm.snapshot.create": "创建快照",
  "vm.snapshot.revert": "回滚快照",
  "vm.snapshot.delete": "删除快照",
  "storage.volume.create": "创建存储卷",
  "storage.volume.upload": "上传存储卷",
  "storage.volume.delete": "删除存储卷",
  "network.create": "创建虚拟网络",
  "network.start": "启动虚拟网络",
  "network.stop": "停止虚拟网络",
  "network.delete": "删除虚拟网络",
};

const targetLabels: Record<string, string> = {
  vm: "虚拟机",
  backup: "备份",
  vm_snapshot: "快照",
  storage_volume: "存储卷",
  network: "虚拟网络",
};

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
});

function formatTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date);
}

function formatDuration(task: Task) {
  const start = new Date(task.createdAt).getTime();
  const end = new Date(task.updatedAt).getTime();
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return "-";
  const seconds = Math.round((end - start) / 1000);
  if (seconds < 60) return `${seconds} 秒`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分 ${seconds % 60} 秒`;
  return `${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分`;
}

function taskVMUUID(task: Task) {
  if (task.targetKind === "vm") return task.targetId;
  if (task.targetKind === "vm_snapshot") return task.targetId.split("/")[0] || undefined;
  return undefined;
}

function StatusBadge({ status }: { status: TaskStatus }) {
  const icon = status === "succeeded" ? <CheckCircle2 className="h-3 w-3" />
    : status === "failed" ? <XCircle className="h-3 w-3" />
      : status === "interrupted" ? <Ban className="h-3 w-3" />
        : status === "queued" ? <CircleAlert className="h-3 w-3" /> : <LoaderCircle className="h-3 w-3 animate-spin" />;
  const variant = status === "succeeded" ? "success" : status === "failed" ? "destructive" : status === "running" ? "warning" : "outline";
  return <Badge variant={variant} className="whitespace-nowrap">{icon}{statusLabels[status]}</Badge>;
}

function TaskRow({ task }: { task: Task }) {
  const vmUUID = taskVMUUID(task);
  const progressClass = task.status === "failed" ? "bg-destructive" : task.status === "succeeded" ? "bg-success" : task.status === "interrupted" ? "bg-muted-foreground" : "bg-primary";
  return (
    <TableRow data-testid={`task-row-${task.id}`} data-status={task.status}>
      <TableCell>
        <p className="font-medium">{kindLabels[task.kind] ?? task.kind}</p>
        <p className="max-w-56 truncate font-mono text-[10px] text-muted-foreground" title={task.id}>{task.id}</p>
      </TableCell>
      <TableCell><StatusBadge status={task.status} /></TableCell>
      <TableCell>
        <Badge variant="outline">{targetLabels[task.targetKind] ?? task.targetKind}</Badge>
        {vmUUID ? <Link to="/vms/$uuid" params={{ uuid: vmUUID }} className="mt-1 block max-w-60 truncate font-mono text-xs text-primary hover:underline" title={task.targetId}>{task.targetId}</Link>
          : <p className="mt-1 max-w-60 truncate font-mono text-xs text-muted-foreground" title={task.targetId}>{task.targetId}</p>}
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <div className="h-1.5 w-24 overflow-hidden rounded-full bg-muted"><div className={cn("h-full rounded-full transition-[width]", progressClass)} style={{ width: `${Math.max(0, Math.min(100, task.progress))}%` }} /></div>
          <span className="w-10 text-right font-mono text-xs">{task.progress}%</span>
        </div>
        <p className="mt-1 max-w-64 truncate text-xs text-muted-foreground" title={task.error || task.step}>{task.error || task.step || "-"}</p>
        {task.error && task.step ? <p className="max-w-64 truncate text-[10px] text-muted-foreground" title={task.step}>{task.step}</p> : null}
      </TableCell>
      <TableCell><span className="font-mono text-xs">{task.actor}</span></TableCell>
      <TableCell><p className="whitespace-nowrap font-mono text-xs text-muted-foreground">{formatTime(task.updatedAt)}</p><p className="text-[10px] text-muted-foreground">{formatDuration(task)}</p></TableCell>
    </TableRow>
  );
}

export function TasksPage() {
  const [status, setStatus] = useState<"" | TaskStatus>("");
  const [page, setPage] = useState(1);
  const query = useTasksQuery({ status: status || undefined, page, pageSize });
  const tasks = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const firstItem = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const lastItem = Math.min(page * pageSize, total);
  const running = tasks.filter((task) => task.status === "running" || task.status === "queued").length;

  return (
    <>
      <PageHeader
        title="任务中心"
        breadcrumb="总览 / 任务中心"
        description="跟踪虚拟机、存储和网络长任务的进度、步骤与失败原因。"
        actions={<Badge variant={running ? "warning" : "outline"}>{running ? <LoaderCircle className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}{running ? `${running} 个任务执行中` : "当前页无运行任务"}</Badge>}
      />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><ListChecks className="h-4 w-4 text-primary" />任务记录</CardTitle>
          <CardDescription>任务持久化在 SQLite 中，页面每 5 秒刷新。服务重启时未完成任务会标记为已中断。</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <div className="flex flex-col gap-3 border-b p-4 md:flex-row md:items-center" data-testid="tasks-filters">
            <select aria-label="任务状态筛选" data-testid="task-status-filter" value={status} onChange={(event) => { setStatus(event.target.value as "" | TaskStatus); setPage(1); }} className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
              <option value="">全部状态</option>
              <option value="queued">排队中</option>
              <option value="running">执行中</option>
              <option value="succeeded">已完成</option>
              <option value="failed">失败</option>
              <option value="interrupted">已中断</option>
            </select>
            <p className="text-xs text-muted-foreground md:ml-auto" data-testid="tasks-total">共 {total} 个任务</p>
          </div>

          {query.isLoading ? <LoadingState label="正在读取任务" /> : null}
          {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取任务"} onRetry={() => void query.refetch()} className="m-4" /> : null}
          {!query.isLoading && !query.isError && tasks.length === 0 ? (
            <div className="px-5 py-16 text-center" data-testid="tasks-empty">
              <ListChecks className="mx-auto h-7 w-7 text-muted-foreground" />
              <p className="mt-3 text-sm font-medium">没有匹配的任务</p>
              <p className="mt-1 text-xs text-muted-foreground">长任务创建后会在这里显示进度和结果。</p>
            </div>
          ) : null}
          {tasks.length ? <Table className="min-w-[960px]">
            <TableHeader><TableRow><TableHead>任务</TableHead><TableHead>状态</TableHead><TableHead>目标</TableHead><TableHead>进度</TableHead><TableHead>操作者</TableHead><TableHead>更新时间</TableHead></TableRow></TableHeader>
            <TableBody>{tasks.map((task) => <TaskRow key={task.id} task={task} />)}</TableBody>
          </Table> : null}
          {query.data ? <div className="flex flex-col gap-3 border-t p-4 sm:flex-row sm:items-center" data-testid="tasks-pagination">
            <p className="text-xs text-muted-foreground">显示 {firstItem}-{lastItem}，共 {total} 条</p>
            <div className="flex items-center gap-2 sm:ml-auto">
              <Button variant="outline" size="sm" data-testid="tasks-prev" disabled={page <= 1 || query.isFetching} title={query.isFetching ? "正在加载任务列表" : page <= 1 ? "已经是第一页" : "查看上一页任务"} onClick={() => setPage((current) => Math.max(1, current - 1))}><ChevronLeft className="h-4 w-4" />上一页</Button>
              <span className="text-xs text-muted-foreground">第 {page} / {totalPages} 页</span>
              <Button variant="outline" size="sm" data-testid="tasks-next" disabled={page >= totalPages || query.isFetching} title={query.isFetching ? "正在加载任务列表" : page >= totalPages ? "已经是最后一页" : "查看下一页任务"} onClick={() => setPage((current) => Math.min(totalPages, current + 1))}>下一页<ChevronRight className="h-4 w-4" /></Button>
            </div>
          </div> : null}
        </CardContent>
      </Card>
    </>
  );
}
