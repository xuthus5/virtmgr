import { useEffect, useState, type FormEvent } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Archive, CheckCircle2, HardDrive, LoaderCircle, ShieldAlert } from "lucide-react";
import { ApiError, api } from "@/api/client";
import type { Role, VMState } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { useTaskQuery } from "@/features/tasks/use-tasks-query";
import { storagePoolsQueryKey, useStoragePoolsQuery } from "@/features/storage/use-storage-query";
import { canBackupVM, defaultVMBackupName, validateVMBackupDraft } from "@/features/vms/vm-backup-model";

const taskStatusLabel = {
  queued: "排队中",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  interrupted: "已中断",
} as const;

export function VMBackupCard({ uuid, vmName, state, persistent, role, readOnly, disabledReason }: {
  uuid: string;
  vmName: string;
  state: VMState;
  persistent: boolean;
  role: Role | undefined;
  readOnly: boolean;
  disabledReason: string;
}) {
  const queryClient = useQueryClient();
  const eligible = canBackupVM(role, readOnly, persistent, state);
  const poolsQuery = useStoragePoolsQuery(eligible);
  const activePools = (poolsQuery.data?.items ?? []).filter((pool) => pool.state === "active");
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(() => defaultVMBackupName(vmName));
  const [targetPool, setTargetPool] = useState("");
  const [formError, setFormError] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [taskID, setTaskID] = useState<string | null>(null);
  const taskQuery = useTaskQuery(taskID);

  useEffect(() => {
    if (taskQuery.data?.status !== "succeeded") return;
    setNotice({ tone: "success", text: `虚拟机 ${vmName} 备份完成。` });
    void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    void queryClient.invalidateQueries({ queryKey: ["storage", "volumes"] });
    void queryClient.invalidateQueries({ queryKey: ["tasks"] });
  }, [queryClient, taskQuery.data?.status, vmName]);

  const mutation = useMutation({
    mutationFn: (input: { name: string; targetPool: string }) => api.backupVM(uuid, input),
    onSuccess: (task) => {
      setTaskID(task.id);
      setOpen(false);
      setFormError("");
      setNotice(null);
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: (cause) => {
      setFormError(cause instanceof ApiError ? cause.message : "创建备份任务失败，请稍后重试。");
    },
  });

  function toggleForm() {
    if (!eligible) return;
    setName(defaultVMBackupName(vmName));
    setTargetPool(activePools[0]?.name ?? "");
    setFormError("");
    setNotice(null);
    setOpen((current) => !current);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!eligible) return;
    const validation = validateVMBackupDraft(name, targetPool);
    if (validation) {
      setFormError(validation);
      return;
    }
    setFormError("");
    mutation.mutate({ name: name.trim(), targetPool });
  }

  const disabledTitle = disabledReason || (!activePools.length && poolsQuery.data ? "没有可用的活跃存储池" : "");
  const task = taskQuery.data;
  const canOpen = eligible && !poolsQuery.isLoading && activePools.length > 0;

  return (
    <Card className="mt-4" data-testid="vm-backup-card">
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2"><Archive className="h-4 w-4 text-primary" />冷备份</CardTitle>
            <CardDescription>将持久配置 XML、manifest 和可写磁盘复制到指定活跃存储池。仅支持关机或异常停止的持久虚拟机。</CardDescription>
          </div>
          <Button type="button" variant="outline" size="sm" data-testid="vm-backup-open" disabled={!canOpen || mutation.isPending} title={disabledTitle || "创建虚拟机冷备份"} onClick={toggleForm}>
            <Archive className="h-4 w-4" />创建备份
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {disabledReason ? <p className="flex items-center gap-2 text-xs text-muted-foreground" data-testid="vm-backup-disabled"><ShieldAlert className="h-3.5 w-3.5" />{disabledReason}。</p> : null}
        {eligible && poolsQuery.isError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">无法读取存储池：{poolsQuery.error instanceof Error ? poolsQuery.error.message : "请稍后重试"}</p> : null}
        {eligible && !poolsQuery.isLoading && !poolsQuery.isError && activePools.length === 0 ? <p className="text-xs text-muted-foreground" data-testid="vm-backup-no-pool">没有可用的活跃存储池，请先在存储页启动存储池。</p> : null}

        {open ? (
          <form className="rounded-lg border bg-muted/20 p-4" data-testid="vm-backup-form" onSubmit={submit}>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="vm-backup-name">备份名称</Label>
                <Input id="vm-backup-name" data-testid="vm-backup-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="backup-20261108" autoComplete="off" required pattern="[A-Za-z0-9][A-Za-z0-9_.-]{0,63}" />
                <p className="text-[11px] text-muted-foreground">将生成 &lt;名称&gt;-disk-NN、&lt;名称&gt;-domain.xml 和 &lt;名称&gt;-manifest.json。</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="vm-backup-pool">目标存储池</Label>
                <select id="vm-backup-pool" data-testid="vm-backup-pool" value={targetPool} onChange={(event) => setTargetPool(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" required>
                  {activePools.map((pool) => <option key={pool.name} value={pool.name}>{pool.name} · {pool.targetPath || pool.type}</option>)}
                </select>
              </div>
            </div>
            <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
              <p className="flex items-center gap-2 text-xs text-muted-foreground"><HardDrive className="h-3.5 w-3.5" />备份内容不包含内存状态，完成后可在存储页查看卷。</p>
              <div className="flex items-center gap-2">
                <Button type="button" variant="ghost" size="sm" onClick={() => { setOpen(false); setFormError(""); }}>取消</Button>
                <Button type="submit" size="sm" data-testid="vm-backup-submit" disabled={mutation.isPending || !name.trim() || !targetPool}>
                  {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Archive className="h-4 w-4" />}
                  {mutation.isPending ? "正在提交..." : "开始备份"}
                </Button>
              </div>
            </div>
            {formError ? <p role="alert" data-testid="vm-backup-error" className="mt-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{formError}</p> : null}
          </form>
        ) : null}

        {notice ? <p role="status" data-testid="vm-backup-notice" className={notice.tone === "success" ? "rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs text-success" : "rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"}>{notice.text}</p> : null}

        {task ? (
          <div className="space-y-3 rounded-lg border bg-card px-4 py-3" data-testid="vm-backup-task" data-status={task.status}>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={task.status === "succeeded" ? "success" : task.status === "failed" ? "destructive" : "warning"}>
                {task.status === "succeeded" ? <CheckCircle2 className="h-3 w-3" /> : task.status === "failed" ? <ShieldAlert className="h-3 w-3" /> : <LoaderCircle className="h-3 w-3 animate-spin" />}
                {taskStatusLabel[task.status]}
              </Badge>
              <span className="text-sm font-medium">{task.step}</span>
              <span className="ml-auto text-xs tabular-nums text-muted-foreground">{task.progress}%</span>
            </div>
            <Progress value={task.progress} />
            {task.error ? <p className="text-xs text-destructive" data-testid="vm-backup-task-error">{task.error}</p> : null}
            <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
              <span className="font-mono">{task.id}</span>
              <Link to="/tasks" className="text-primary hover:underline">在任务中心查看</Link>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
