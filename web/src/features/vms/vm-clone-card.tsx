import type { FormEventHandler } from "react";
import { Link } from "@tanstack/react-router";
import { Copy, LoaderCircle } from "lucide-react";
import type { Task, TaskStatus, VM } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";

const taskStatusLabel: Record<TaskStatus, string> = {
  queued: "排队中",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  interrupted: "已中断",
  cancelled: "已取消",
};

export function VMCloneCard({ vm, disabledReason, open, name, start, autostart, error, pending, task, onOpen, onNameChange, onStartChange, onAutostartChange, onSubmit, onCancel }: {
  vm: VM;
  disabledReason: string;
  open: boolean;
  name: string;
  start: boolean;
  autostart: boolean;
  error: string;
  pending: boolean;
  task?: Task;
  onOpen: () => void;
  onNameChange: (value: string) => void;
  onStartChange: (value: boolean) => void;
  onAutostartChange: (value: boolean) => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  onCancel: () => void;
}) {
  return <Card className="mt-4" data-testid="vm-clone-card">
    <CardHeader>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>克隆虚拟机</CardTitle>
          <CardDescription>复制持久域定义和可写磁盘，并生成新的 UUID、MAC 和磁盘文件。源虚拟机必须处于关机或异常停止状态。</CardDescription>
        </div>
        <Button type="button" variant="outline" size="sm" data-testid="vm-clone-open" disabled={Boolean(disabledReason) || pending} title={disabledReason || "创建虚拟机克隆"} onClick={onOpen}>
          <Copy className="h-4 w-4" />克隆虚拟机
        </Button>
      </div>
    </CardHeader>
    <CardContent className="space-y-4">
      {disabledReason ? <p className="text-xs text-muted-foreground">{disabledReason}。</p> : null}
      {open ? (
        <form data-testid="vm-clone-form" className="rounded-lg border bg-muted/20 p-4" onSubmit={onSubmit}>
          <div className="grid gap-4 md:grid-cols-[1fr_auto] md:items-end">
            <div className="space-y-2">
              <Label htmlFor="vm-clone-name">克隆名称</Label>
              <Input id="vm-clone-name" value={name} onChange={(event) => onNameChange(event.target.value)} placeholder={`${vm.name}-clone`} autoComplete="off" required pattern="[A-Za-z0-9][A-Za-z0-9_.-]{0,63}" />
            </div>
            <div className="flex flex-col gap-2 text-sm">
              <label className="inline-flex items-center gap-2"><input type="checkbox" checked={start} onChange={(event) => onStartChange(event.target.checked)} />克隆后启动</label>
              <label className="inline-flex items-center gap-2"><input type="checkbox" checked={autostart} onChange={(event) => onAutostartChange(event.target.checked)} />设置开机自启动</label>
            </div>
          </div>
          <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">磁盘会复制到源卷所在的活跃存储池；克隆任务可在任务中心查看。</p>
            <div className="flex items-center gap-2">
              <Button type="button" variant="ghost" size="sm" onClick={onCancel}>取消</Button>
              <Button type="submit" size="sm" data-testid="vm-clone-submit" disabled={pending || !name.trim()}>
                {pending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Copy className="h-4 w-4" />}
                {pending ? "正在提交..." : "开始克隆"}
              </Button>
            </div>
          </div>
          {error ? <p role="alert" className="mt-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{error}</p> : null}
        </form>
      ) : null}
      {task ? (
        <div className="space-y-3 rounded-lg border bg-card px-4 py-3" data-testid="vm-clone-task" data-status={task.status}>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={task.status === "succeeded" ? "success" : task.status === "failed" ? "destructive" : "warning"}>{taskStatusLabel[task.status]}</Badge>
            <span className="text-sm font-medium">{task.step}</span>
            <span className="ml-auto text-xs tabular-nums text-muted-foreground">{task.progress}%</span>
          </div>
          <Progress value={task.progress} />
          {task.error ? <p className="text-xs text-destructive">{task.error}</p> : null}
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span className="font-mono">{task.id}</span>
            <Link to="/tasks" className="text-primary hover:underline">在任务中心查看</Link>
          </div>
        </div>
      ) : null}
    </CardContent>
  </Card>;
}
