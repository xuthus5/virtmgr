import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, LoaderCircle, Plus, Trash2 } from "lucide-react";
import { api } from "@/api/client";
import type { BackupRepository, BackupSchedule, BackupScheduleInput, Role } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useBackupRepositoriesQuery, useBackupSchedulesQuery } from "@/features/backups/use-backups-query";
import { useVMsQuery } from "@/features/vms/use-vms-query";
import { actionReason, errorText, formatTime, type BackupNotice } from "@/features/backups/backup-ui";
import { queryKeys } from "@/lib/query-keys";


export function SchedulesPanel({ role, readOnly, onTask, onNotice }: {
  role: Role | undefined;
  readOnly: boolean;
  onTask: (taskID: string, label: string) => void;
  onNotice: (notice: BackupNotice) => void;
}) {
  const queryClient = useQueryClient();
  const query = useBackupSchedulesQuery("");
  const repositories = useBackupRepositoriesQuery();
  const vmsQuery = useVMsQuery("", "", true);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BackupSchedule | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BackupSchedule | null>(null);
  const disabledReason = actionReason(role, readOnly, "create");
  const items = query.data?.items ?? [];
  const eligibleVMs = (vmsQuery.data?.items ?? []).filter((vm) => vm.persistent);
  const toggleMutation = useMutation({
    mutationFn: (schedule: BackupSchedule) => api.updateBackupSchedule(schedule.id, {
      repositoryId: schedule.repositoryId,
      name: schedule.name,
      intervalHours: schedule.intervalHours,
      scheduleMode: schedule.scheduleMode ?? "interval",
      dailyTime: schedule.dailyTime || "02:00",
      retentionCount: schedule.retentionCount,
      retentionDays: schedule.retentionDays,
      enabled: !schedule.enabled,
    }),
  });
  const runMutation = useMutation({ mutationFn: api.runBackupSchedule });
  const deleteMutation = useMutation({ mutationFn: api.deleteBackupSchedule });

  async function refresh() { await queryClient.invalidateQueries({ queryKey: queryKeys.backups.schedules("") }); }
  async function toggle(schedule: BackupSchedule) {
    try {
      await toggleMutation.mutateAsync(schedule);
      await refresh();
      onNotice({ tone: "success", text: schedule.enabled ? "计划已暂停。" : "计划已启用。" });
    } catch (cause) {
      onNotice({ tone: "error", text: errorText(cause, "更新计划失败") });
    }
  }
  async function run(schedule: BackupSchedule) {
    try {
      const task = await runMutation.mutateAsync(schedule.id);
      await refresh();
      onTask(task.id, "执行定时备份");
    } catch (cause) {
      onNotice({ tone: "error", text: errorText(cause, "立即执行失败") });
    }
  }
  async function remove() {
    if (!deleteTarget || deleteMutation.isPending) return;
    try {
      await deleteMutation.mutateAsync(deleteTarget.id);
      setDeleteTarget(null);
      await refresh();
      onNotice({ tone: "success", text: "定时计划已删除。" });
    } catch (cause) {
      onNotice({ tone: "error", text: errorText(cause, "删除定时计划失败") });
    }
  }
  return <>
    <Card>
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle>定时备份计划</CardTitle><CardDescription>按固定间隔创建冷备份。虚拟机运行中时任务会记录跳过原因，不会自动关机；全局同时只执行一个备份。</CardDescription></div><Button size="sm" data-testid="backup-schedule-create" disabled={Boolean(disabledReason)} title={disabledReason || "创建定时备份计划"} onClick={() => { setEditing(null); setFormOpen(true); }}><Plus className="h-4 w-4" />新建计划</Button></div>
      </CardHeader>
      <CardContent className="p-0">
        {query.isLoading ? <LoadingState label="正在读取定时计划" /> : null}
        {query.isError ? <ErrorState className="m-4" message={errorText(query.error, "无法读取定时计划")} onRetry={() => void query.refetch()} /> : null}
        {!query.isLoading && !query.isError && items.length === 0 ? <div className="px-5 py-16 text-center" data-testid="backup-schedules-empty"><CalendarClock className="mx-auto h-8 w-8 text-muted-foreground" /><p className="mt-3 font-medium">还没有定时计划</p><p className="mt-1 text-sm text-muted-foreground">为关机虚拟机设置周期备份和保留策略。</p></div> : null}
        {items.length ? <Table className="min-w-[1050px]"><TableHeader><TableRow><TableHead>计划 / 虚拟机</TableHead><TableHead>频率</TableHead><TableHead>保留</TableHead><TableHead>下次执行</TableHead><TableHead>最近结果</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{items.map((schedule) => <TableRow key={schedule.id} data-testid={`backup-schedule-row-${schedule.id}`}>
          <TableCell><p className="font-medium">{schedule.name}</p><p className="text-xs text-muted-foreground">{schedule.vmName || schedule.vmUuid} · {schedule.repositoryName || schedule.targetPool}</p></TableCell>
          <TableCell>{schedule.scheduleMode === "daily" ? `每日 ${schedule.dailyTime || "02:00"}` : `每 ${schedule.intervalHours} 小时`}</TableCell>
          <TableCell>{schedule.retentionCount} 份{schedule.retentionDays ? ` / ${schedule.retentionDays} 天` : ""}</TableCell>
          <TableCell className="whitespace-nowrap text-xs">{formatTime(schedule.nextRunAt)}</TableCell>
          <TableCell><span className="text-xs">{formatTime(schedule.lastRunAt)}</span>{schedule.lastError ? <p className="mt-1 max-w-56 truncate text-xs text-destructive" title={schedule.lastError}>{schedule.lastError}</p> : null}</TableCell>
          <TableCell><Badge variant={schedule.enabled ? "success" : "secondary"}>{schedule.enabled ? "已启用" : "已暂停"}</Badge></TableCell>
          <TableCell><div className="flex flex-wrap justify-end gap-1">
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-edit-${schedule.id}`} disabled={Boolean(disabledReason)} title={disabledReason || "编辑计划"} onClick={() => { setEditing(schedule); setFormOpen(true); }}>编辑</Button>
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-toggle-${schedule.id}`} disabled={Boolean(disabledReason) || (toggleMutation.isPending && toggleMutation.variables?.id === schedule.id)} title={disabledReason || (schedule.enabled ? "暂停计划" : "启用计划")} onClick={() => void toggle(schedule)}>{toggleMutation.isPending && toggleMutation.variables?.id === schedule.id ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}{schedule.enabled ? "暂停" : "启用"}</Button>
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-run-${schedule.id}`} disabled={Boolean(disabledReason) || (runMutation.isPending && runMutation.variables === schedule.id)} title={disabledReason || "立即执行一次"} onClick={() => void run(schedule)}>{runMutation.isPending && runMutation.variables === schedule.id ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}立即执行</Button>
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-delete-${schedule.id}`} disabled={Boolean(disabledReason)} title={disabledReason || "删除计划"} onClick={() => setDeleteTarget(schedule)}><Trash2 className="h-4 w-4" />删除</Button>
          </div></TableCell>
        </TableRow>)}</TableBody></Table> : null}</CardContent>
    </Card>
    <ScheduleFormModal open={formOpen} onClose={() => setFormOpen(false)} editing={editing} repositories={repositories.data?.items ?? []} vms={eligibleVMs} onSaved={async () => { setFormOpen(false); await refresh(); onNotice({ tone: "success", text: editing ? "定时计划已更新。" : "定时计划已创建。" }); }} onError={(cause) => onNotice({ tone: "error", text: errorText(cause, "保存定时计划失败") })} />
    <Modal open={Boolean(deleteTarget)} onClose={() => { if (!deleteMutation.isPending) setDeleteTarget(null); }} title="删除定时计划" description="只删除调度配置，不会删除已经创建的备份。" footer={<><Button variant="ghost" disabled={deleteMutation.isPending} onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="destructive" data-testid="backup-schedule-delete-confirm" disabled={deleteMutation.isPending} onClick={() => void remove()}>{deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}{deleteMutation.isPending ? "删除中..." : "确认删除"}</Button></>}>
      {deleteTarget ? <p className="text-sm">确认删除 <span className="font-medium">{deleteTarget.name}</span>（{deleteTarget.vmName || deleteTarget.vmUuid}）的定时计划？</p> : null}
    </Modal>
  </>;
}

function ScheduleFormModal({ open, onClose, editing, repositories, vms, onSaved, onError }: {
  open: boolean;
  onClose: () => void;
  editing: BackupSchedule | null;
  repositories: BackupRepository[];
  vms: Array<{ uuid: string; name: string }>;
  onSaved: () => Promise<void>;
  onError: (cause: unknown) => void;
}) {
  const [form, setForm] = useState<BackupScheduleInput>({ vmUuid: "", repositoryId: "", name: "", intervalHours: 24, scheduleMode: "interval", dailyTime: "02:00", retentionCount: 7, retentionDays: 30, enabled: true });
  const [error, setError] = useState("");
  const mutation = useMutation({
    mutationFn: (input: BackupScheduleInput) => editing
      ? api.updateBackupSchedule(editing.id, input)
      : api.createBackupSchedule(input),
  });
  useEffect(() => {
    if (!open) return;
    if (editing) setForm({ vmUuid: editing.vmUuid, repositoryId: editing.repositoryId, name: editing.name, intervalHours: editing.intervalHours, scheduleMode: editing.scheduleMode ?? "interval", dailyTime: editing.dailyTime || "02:00", retentionCount: editing.retentionCount, retentionDays: editing.retentionDays, enabled: editing.enabled });
    else setForm({ vmUuid: vms[0]?.uuid ?? "", repositoryId: repositories.find((item) => item.isDefault)?.id ?? repositories[0]?.id ?? "", name: "", intervalHours: 24, scheduleMode: "interval", dailyTime: "02:00", retentionCount: 7, retentionDays: 30, enabled: true });
    setError("");
  }, [editing, open, repositories, vms]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!form.vmUuid || !form.repositoryId || !form.name.trim()) { setError("请选择虚拟机、存储库并填写计划名称。"); return; }
    try {
      await mutation.mutateAsync({ ...form, name: form.name.trim() });
      await onSaved();
    } catch (cause) {
      setError(errorText(cause, "保存定时计划失败"));
      onError(cause);
    }
  }
  function close() { if (!mutation.isPending) onClose(); }
  return <Modal open={open} onClose={close} title={editing ? "编辑定时计划" : "新建定时计划"} description="支持固定间隔或每日时刻；不引入 Cron 表达式，时区跟随宿主机。" testId="backup-schedule-modal" footer={<><Button variant="ghost" disabled={mutation.isPending} onClick={close}>取消</Button><Button form="backup-schedule-form" type="submit" disabled={mutation.isPending}>{mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : null}{mutation.isPending ? "正在保存" : editing ? "保存修改" : "创建计划"}</Button></>}>
    <form id="backup-schedule-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
      <div className="space-y-2"><Label htmlFor="schedule-vm">虚拟机</Label><select id="schedule-vm" data-testid="backup-schedule-vm" disabled={Boolean(editing)} value={form.vmUuid} onChange={(event) => setForm({ ...form, vmUuid: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm disabled:opacity-60">{vms.map((vm) => <option key={vm.uuid} value={vm.uuid}>{vm.name}</option>)}</select></div>
      <div className="space-y-2"><Label htmlFor="schedule-name">计划名称</Label><Input id="schedule-name" data-testid="backup-schedule-name" value={form.name} maxLength={32} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="例如 daily-backup" /></div>
      <div className="space-y-2"><Label htmlFor="schedule-repository">备份存储库</Label><select id="schedule-repository" data-testid="backup-schedule-repository" value={form.repositoryId} onChange={(event) => setForm({ ...form, repositoryId: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{repositories.map((repository) => <option key={repository.id} value={repository.id}>{repository.name} · {repository.poolName}</option>)}</select></div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-2"><Label htmlFor="schedule-mode">调度方式</Label><select id="schedule-mode" data-testid="backup-schedule-mode" value={form.scheduleMode} onChange={(event) => setForm({ ...form, scheduleMode: event.target.value === "daily" ? "daily" : "interval" })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm"><option value="interval">固定间隔</option><option value="daily">每日时刻</option></select></div>
        {form.scheduleMode === "daily"
          ? <div className="space-y-2"><Label htmlFor="schedule-daily-time">每日执行时间</Label><Input id="schedule-daily-time" data-testid="backup-schedule-daily-time" type="time" value={form.dailyTime} onChange={(event) => setForm({ ...form, dailyTime: event.target.value })} /></div>
          : <div className="space-y-2"><Label htmlFor="schedule-interval">间隔（小时）</Label><Input id="schedule-interval" data-testid="backup-schedule-interval" type="number" min={1} max={168} value={form.intervalHours} onChange={(event) => setForm({ ...form, intervalHours: Number(event.target.value) })} /></div>}
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-2"><Label htmlFor="schedule-retention">保留份数</Label><Input id="schedule-retention" data-testid="backup-schedule-retention" type="number" min={1} max={50} value={form.retentionCount} onChange={(event) => setForm({ ...form, retentionCount: Number(event.target.value) })} /></div>
        <div className="space-y-2"><Label htmlFor="schedule-days">最大保留天数</Label><Input id="schedule-days" data-testid="backup-schedule-days" type="number" min={0} max={3650} value={form.retentionDays} onChange={(event) => setForm({ ...form, retentionDays: Number(event.target.value) })} /></div>
      </div>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} />创建后立即启用</label>
      <p className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">保留策略只清理该计划创建且未固定的成功备份，始终至少保留最近 N 份。最大保留天数为 0 时按份数保留。</p>
      {error ? <p role="alert" className="text-xs text-destructive">{error}</p> : null}
    </form>
  </Modal>;
}
