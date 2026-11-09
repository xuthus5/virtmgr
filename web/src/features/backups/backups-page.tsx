import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Archive, CalendarClock, CheckCircle2, ChevronLeft, ChevronRight, Database,
  HardDrive, Info, LoaderCircle, Pin, PinOff, Plus, RefreshCw, RotateCcw,
  ShieldCheck, Trash2, TriangleAlert,
} from "lucide-react";
import { ApiError, api } from "@/api/client";
import type {
  Backup, BackupRepository, BackupRepositoryInput, BackupSchedule, BackupScheduleInput,
  BackupStatus, Role, VMRestoreInput,
} from "@/api/types";
import { MetricCard } from "@/components/data/metric-card";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/ui/modal";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";
import {
  backupRepositoriesQueryKey, backupSchedulesQueryKey,
  useBackupRepositoriesQuery, useBackupSchedulesQuery, useBackupSummaryQuery,
  useBackupsQuery,
} from "@/features/backups/use-backups-query";
import { storagePoolsQueryKey, useStoragePoolsQuery } from "@/features/storage/use-storage-query";
import { useTaskQuery } from "@/features/tasks/use-tasks-query";
import { useVMsQuery } from "@/features/vms/use-vms-query";
import { cn } from "@/lib/utils";

type Tab = "backups" | "schedules" | "repositories";

const pageSize = 25;

const backupStatusMeta: Record<BackupStatus, { label: string; tone: "success" | "warning" | "destructive" | "secondary" | "default" }> = {
  running: { label: "备份中", tone: "warning" },
  completed: { label: "已完成", tone: "success" },
  failed: { label: "失败", tone: "destructive" },
  interrupted: { label: "已中断", tone: "secondary" },
  deleting: { label: "删除中", tone: "warning" },
  delete_failed: { label: "删除失败", tone: "destructive" },
};

const verifyMeta = {
  unknown: { label: "未校验", tone: "secondary" as const },
  verified: { label: "校验通过", tone: "success" as const },
  failed: { label: "校验失败", tone: "destructive" as const },
};

const dateTime = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false,
});

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : dateTime.format(date);
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size >= 100 || index === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

function errorText(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}

function backupName(vmName: string) {
  const stamp = new Date().toISOString().replace(/\D/g, "").slice(0, 14);
  return `${vmName}-backup-${stamp}`.slice(0, 64);
}

function actionReason(role: Role | undefined, readOnly: boolean, action: "create" | "verify" | "restore" | "delete" | "admin") {
  if (readOnly && action !== "verify") return "系统当前处于只读模式";
  if (!role) return "正在读取账号权限";
  if (action === "admin" && role !== "admin") return "仅管理员可执行此操作";
  if (action === "restore" && role !== "admin") return "仅管理员可将备份恢复为新虚拟机";
  if (action === "delete" && role !== "admin") return "仅管理员可删除备份";
  if ((action === "create" || action === "verify") && role === "viewer") return "Viewer 账号只能查看备份";
  return "";
}

export function BackupsPage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("backups");
  const [status, setStatus] = useState<BackupStatus | "">("");
  const [repositoryFilter, setRepositoryFilter] = useState("");
  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);
  const [detail, setDetail] = useState<Backup | null>(null);
  const [verifyTarget, setVerifyTarget] = useState<Backup | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Backup | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState("");
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [activeTaskID, setActiveTaskID] = useState<string | null>(null);
  const [taskLabel, setTaskLabel] = useState("任务");

  const role = session.data?.user.role;
  const readOnly = Boolean(session.data?.readOnly);
  const backupsQuery = useBackupsQuery({ status, repositoryId: repositoryFilter, page, pageSize });
  const summaryQuery = useBackupSummaryQuery();
  const repositoriesQuery = useBackupRepositoriesQuery();
  const vmsQuery = useVMsQuery("", "", Boolean(session.data));
  const activeTask = useTaskQuery(activeTaskID);
  const backups = backupsQuery.data?.items ?? [];
  const repositories = repositoriesQuery.data?.items ?? [];
  const total = backupsQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const summary = summaryQuery.data;

  useEffect(() => {
    const task = activeTask.data;
    if (!task || !["succeeded", "failed", "interrupted"].includes(task.status)) return;
    setActiveTaskID(null);
    if (task.status === "succeeded") {
      setNotice({ tone: "success", text: `${taskLabel}已完成。` });
    } else {
      setNotice({ tone: "error", text: `${taskLabel}失败：${task.error || task.step}` });
    }
    void queryClient.invalidateQueries({ queryKey: ["backups"] });
    void queryClient.invalidateQueries({ queryKey: backupRepositoriesQueryKey });
    void queryClient.invalidateQueries({ queryKey: backupSchedulesQueryKey("") });
    void queryClient.invalidateQueries({ queryKey: storagePoolsQueryKey });
    void queryClient.invalidateQueries({ queryKey: ["tasks"] });
    void queryClient.invalidateQueries({ queryKey: ["vms"] });
  }, [activeTask.data, queryClient, taskLabel]);

  function beginTask(taskID: string, label: string) {
    setTaskLabel(label);
    setActiveTaskID(taskID);
    setNotice(null);
  }

  return (
    <>
      <PageHeader
        title="备份中心"
        breadcrumb="运行与资源 / 备份中心"
        description="集中管理虚拟机冷备份、完整性校验、恢复为新虚拟机和定时保留策略。"
        actions={<Button variant="outline" size="sm" disabled={backupsQuery.isFetching} onClick={() => void queryClient.invalidateQueries({ queryKey: ["backups"] })}><RefreshCw className={cn("h-4 w-4", backupsQuery.isFetching && "animate-spin")} />刷新</Button>}
      />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="备份总数" value={summary?.total ?? 0} hint="所有虚拟机备份集" icon={Archive} testId="backup-total" />
        <MetricCard label="成功备份" value={summary?.completed ?? 0} hint="可用于恢复的备份" icon={CheckCircle2} tone="success" testId="backup-completed" />
        <MetricCard label="占用容量" value={formatBytes(summary?.totalBytes ?? 0)} hint="按备份产物记录汇总" icon={HardDrive} tone="neutral" testId="backup-size" />
        <MetricCard label="异常/执行中" value={`${summary?.failed ?? 0} / ${summary?.running ?? 0}`} hint="失败备份可清理后重试" icon={TriangleAlert} tone="warning" testId="backup-attention" />
      </div>

      {notice ? <p role="status" data-testid="backup-notice" className={cn("mb-4 rounded-md border px-3 py-2 text-sm", notice.tone === "success" ? "border-success/30 bg-success/10 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")}>{notice.text}</p> : null}
      {activeTaskID ? <p role="status" data-testid="backup-task-progress" className="mb-4 flex items-center gap-2 rounded-md border bg-muted/40 px-3 py-2 text-sm"><LoaderCircle className="h-4 w-4 animate-spin text-primary" />{taskLabel}：{activeTask.data?.step ?? "正在创建任务"} {activeTask.data ? `${activeTask.data.progress}%` : ""}</p> : null}

      <div className="mb-4 flex flex-wrap items-center gap-2" role="tablist" aria-label="备份中心视图">
        {[
          { id: "backups" as const, label: "备份集", icon: Archive },
          { id: "schedules" as const, label: "定时计划", icon: CalendarClock },
          { id: "repositories" as const, label: "备份存储库", icon: Database },
        ].map((item) => {
          const Icon = item.icon;
          return <Button key={item.id} role="tab" aria-selected={tab === item.id} variant={tab === item.id ? "default" : "outline"} size="sm" data-testid={`backup-tab-${item.id}`} onClick={() => setTab(item.id)}><Icon className="h-4 w-4" />{item.label}</Button>;
        })}
        {tab === "backups" ? <Button className="sm:ml-auto" size="sm" data-testid="backup-create-open" disabled={Boolean(actionReason(role, readOnly, "create"))} title={actionReason(role, readOnly, "create") || "创建新的虚拟机冷备份"} onClick={() => setCreateOpen(true)}><Plus className="h-4 w-4" />创建备份</Button> : null}
      </div>

      {tab === "backups" ? (
        <Card>
          <CardHeader>
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <CardTitle>备份集</CardTitle>
                <CardDescription>备份按虚拟机分组，只有显示“已完成”的备份可用于恢复；校验用于确认产物完整性。</CardDescription>
              </div>
              <div className="flex flex-col gap-2 sm:flex-row">
                <select aria-label="备份状态筛选" data-testid="backup-status-filter" value={status} onChange={(event) => { setStatus(event.target.value as BackupStatus | ""); setPage(1); }} className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm">
                  <option value="">全部状态</option>
                  <option value="completed">已完成</option>
                  <option value="running">备份中</option>
                  <option value="failed">失败</option>
                  <option value="interrupted">已中断</option>
                  <option value="delete_failed">删除失败</option>
                </select>
                <select aria-label="备份存储库筛选" value={repositoryFilter} onChange={(event) => { setRepositoryFilter(event.target.value); setPage(1); }} className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm">
                  <option value="">全部存储库</option>
                  {repositories.map((repository) => <option key={repository.id} value={repository.id}>{repository.name}</option>)}
                </select>
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {backupsQuery.isLoading ? <LoadingState label="正在读取备份目录" /> : null}
            {backupsQuery.isError ? <ErrorState className="m-4" message={errorText(backupsQuery.error, "无法读取备份目录")} onRetry={() => void backupsQuery.refetch()} /> : null}
            {!backupsQuery.isLoading && !backupsQuery.isError && backups.length === 0 ? (
              <div className="px-5 py-16 text-center" data-testid="backups-empty">
                <Archive className="mx-auto h-8 w-8 text-muted-foreground" />
                <p className="mt-3 font-medium">还没有备份集</p>
                <p className="mt-1 text-sm text-muted-foreground">先关闭虚拟机，再创建冷备份。备份不会包含内存状态。</p>
              </div>
            ) : null}
            {backups.length ? (
              <Table className="min-w-[1120px]">
                <TableHeader><TableRow><TableHead>备份 / 虚拟机</TableHead><TableHead>状态</TableHead><TableHead>大小</TableHead><TableHead>存储库</TableHead><TableHead>校验</TableHead><TableHead>创建时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader>
                <TableBody>{backups.map((backup) => (
                  <TableRow key={backup.id} data-testid={`backup-row-${backup.id}`}>
                    <TableCell>
                      <p className="max-w-64 truncate font-medium" title={backup.name}>{backup.name}</p>
                      <p className="mt-0.5 text-xs text-muted-foreground">{backup.vmName} · {backup.id.slice(0, 12)}</p>
                    </TableCell>
                    <TableCell><Badge variant={backupStatusMeta[backup.status].tone}>{backupStatusMeta[backup.status].label}</Badge></TableCell>
                    <TableCell><span className="whitespace-nowrap tabular-nums">{formatBytes(backup.totalBytes)}</span><p className="text-xs text-muted-foreground">{backup.diskCount} 块磁盘</p></TableCell>
                    <TableCell><span className="whitespace-nowrap">{backup.repositoryName || backup.targetPool}</span><p className="text-xs text-muted-foreground">{backup.targetPool}</p></TableCell>
                    <TableCell><Badge variant={verifyMeta[backup.verifyStatus].tone}>{verifyMeta[backup.verifyStatus].label}</Badge>{backup.verifiedAt ? <p className="mt-1 text-[10px] text-muted-foreground">{formatTime(backup.verifiedAt)}</p> : null}</TableCell>
                    <TableCell><span className="whitespace-nowrap text-xs">{formatTime(backup.completedAt || backup.createdAt)}</span><p className="text-xs text-muted-foreground">{backup.createdBy}</p></TableCell>
                    <TableCell><BackupActions backup={backup} role={role} readOnly={readOnly} onDetail={() => setDetail(backup)} onVerify={() => setVerifyTarget(backup)} onRestore={() => setRestoreTarget(backup)} onDelete={() => { setDeleteTarget(backup); setDeleteConfirm(""); }} onPin={async () => {
                      try {
                        await api.pinBackup(backup.id, { pinned: !backup.pinned });
                        setNotice({ tone: "success", text: backup.pinned ? "已取消固定保留。" : "已固定保留，自动清理不会删除该备份。" });
                        await queryClient.invalidateQueries({ queryKey: ["backups"] });
                      } catch (cause) {
                        setNotice({ tone: "error", text: errorText(cause, "更新固定保留状态失败") });
                      }
                    }} /></TableCell>
                  </TableRow>
                ))}</TableBody>
              </Table>
            ) : null}
            {backupsQuery.data ? <div className="flex flex-col gap-2 border-t p-4 sm:flex-row sm:items-center">
              <p className="text-xs text-muted-foreground">共 {total} 个备份集</p>
              <div className="flex items-center gap-2 sm:ml-auto">
                <Button variant="outline" size="sm" data-testid="backup-prev" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}><ChevronLeft className="h-4 w-4" />上一页</Button>
                <span className="text-xs text-muted-foreground">第 {page} / {totalPages} 页</span>
                <Button variant="outline" size="sm" data-testid="backup-next" disabled={page >= totalPages} onClick={() => setPage((current) => Math.min(totalPages, current + 1))}>下一页<ChevronRight className="h-4 w-4" /></Button>
              </div>
            </div> : null}
          </CardContent>
        </Card>
      ) : null}

      {tab === "schedules" ? <SchedulesPanel role={role} readOnly={readOnly} onTask={beginTask} onNotice={setNotice} /> : null}
      {tab === "repositories" ? <RepositoriesPanel role={role} readOnly={readOnly} onNotice={setNotice} /> : null}

      <CreateBackupModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        repositories={repositories}
        vms={(vmsQuery.data?.items ?? []).filter((vm) => vm.persistent && (vm.state === "shutoff" || vm.state === "crashed"))}
        onCreated={(taskID) => { setCreateOpen(false); beginTask(taskID, "创建备份"); }}
        onNotice={setNotice}
      />

      <Modal open={Boolean(detail)} onClose={() => setDetail(null)} title={detail?.name ?? "备份详情"} description="备份产物清单与校验信息" testId="backup-detail-modal" className="max-w-3xl" footer={detail ? <>
        <Button variant="outline" size="sm" disabled={Boolean(actionReason(role, readOnly, "verify"))} title={actionReason(role, readOnly, "verify") || "校验备份完整性"} onClick={() => { setVerifyTarget(detail); setDetail(null); }}><ShieldCheck className="h-4 w-4" />校验</Button>
        <Button variant="outline" size="sm" disabled={Boolean(actionReason(role, readOnly, "restore")) || detail.status !== "completed"} title={actionReason(role, readOnly, "restore") || (detail.status !== "completed" ? "只有已完成的备份可恢复" : "恢复为新虚拟机")} onClick={() => { setRestoreTarget(detail); setDetail(null); }}><RotateCcw className="h-4 w-4" />恢复为新虚拟机</Button>
      </> : null}>
        {detail ? <BackupDetail backup={detail} /> : null}
      </Modal>

      <VerifyBackupModal target={verifyTarget} onClose={() => setVerifyTarget(null)} onStarted={(taskID) => { setVerifyTarget(null); beginTask(taskID, "校验备份"); }} onNotice={setNotice} />

      <RestoreBackupModal target={restoreTarget} repositories={repositories} onClose={() => setRestoreTarget(null)} onStarted={(taskID) => { setRestoreTarget(null); beginTask(taskID, "恢复虚拟机"); }} onNotice={setNotice} />

      <Modal open={Boolean(deleteTarget)} onClose={() => setDeleteTarget(null)} title="删除备份集" description="删除会移除存储池中的配置、清单和全部磁盘副本，无法撤销。" testId="backup-delete-modal" footer={<>
        <Button variant="ghost" onClick={() => setDeleteTarget(null)}>取消</Button>
        <Button variant="destructive" data-testid="backup-delete-confirm" disabled={!deleteTarget || deleteConfirm !== deleteTarget.name} onClick={async () => {
          if (!deleteTarget) return;
          try {
            const task = await api.deleteBackup(deleteTarget.id);
            setDeleteTarget(null);
            beginTask(task.id, "删除备份");
          } catch (cause) {
            setNotice({ tone: "error", text: errorText(cause, "删除备份失败") });
          }
        }}><Trash2 className="h-4 w-4" />确认删除</Button>
      </>}>
        {deleteTarget ? <div className="space-y-3">
          <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm"><p className="font-medium text-destructive">{deleteTarget.name}</p><p className="mt-1 text-xs text-muted-foreground">{deleteTarget.vmName} · {formatBytes(deleteTarget.totalBytes)} · {deleteTarget.diskCount} 块磁盘</p></div>
          <Label htmlFor="backup-delete-name">输入备份名称确认</Label>
          <Input id="backup-delete-name" data-testid="backup-delete-name" value={deleteConfirm} onChange={(event) => setDeleteConfirm(event.target.value)} autoComplete="off" />
        </div> : null}
      </Modal>
    </>
  );
}

function BackupActions({ backup, role, readOnly, onDetail, onVerify, onRestore, onDelete, onPin }: {
  backup: Backup;
  role: Role | undefined;
  readOnly: boolean;
  onDetail: () => void;
  onVerify: () => void;
  onRestore: () => void;
  onDelete: () => void;
  onPin: () => void;
}) {
  const verifyReason = actionReason(role, readOnly, "verify") || (backup.status !== "completed" ? "只有已完成的备份可校验" : "");
  const restoreReason = actionReason(role, readOnly, "restore") || (backup.status !== "completed" ? "只有已完成的备份可恢复" : "");
  const deleteReason = actionReason(role, readOnly, "delete") || (backup.status === "running" || backup.status === "deleting" ? "执行中的备份不能删除" : "");
  return <div className="flex flex-wrap justify-end gap-1">
    <Button variant="ghost" size="sm" title="查看备份详情" aria-label={`查看 ${backup.name} 详情`} data-testid={`backup-detail-${backup.id}`} onClick={onDetail}><Info className="h-4 w-4" />详情</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(verifyReason)} title={verifyReason || "校验备份完整性"} aria-label={`校验 ${backup.name}`} data-testid={`backup-verify-${backup.id}`} onClick={onVerify}><ShieldCheck className="h-4 w-4" />校验</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(restoreReason)} title={restoreReason || "恢复为一台新虚拟机"} aria-label={`恢复 ${backup.name}`} data-testid={`backup-restore-${backup.id}`} onClick={onRestore}><RotateCcw className="h-4 w-4" />恢复</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(actionReason(role, readOnly, "create"))} title={actionReason(role, readOnly, "create") || (backup.pinned ? "取消固定保留" : "固定保留，自动清理时跳过")} aria-label={backup.pinned ? `取消固定 ${backup.name}` : `固定 ${backup.name}`} data-testid={`backup-pin-${backup.id}`} onClick={onPin}>{backup.pinned ? <PinOff className="h-4 w-4" /> : <Pin className="h-4 w-4" />}{backup.pinned ? "取消固定" : "固定"}</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(deleteReason)} title={deleteReason || "删除备份集"} aria-label={`删除 ${backup.name}`} data-testid={`backup-delete-${backup.id}`} onClick={onDelete}><Trash2 className="h-4 w-4" />删除</Button>
  </div>;
}

function BackupDetail({ backup }: { backup: Backup }) {
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-2">
      <Detail label="虚拟机" value={backup.vmName} />
      <Detail label="备份 ID" value={backup.id} mono />
      <Detail label="状态" value={backupStatusMeta[backup.status].label} />
      <Detail label="格式" value={backup.formatVersion} mono />
      <Detail label="存储库" value={`${backup.repositoryName || "-"} / ${backup.targetPool}`} />
      <Detail label="总大小" value={formatBytes(backup.totalBytes)} />
      <Detail label="创建人" value={backup.createdBy} />
      <Detail label="创建时间" value={formatTime(backup.createdAt)} />
      <Detail label="完成时间" value={formatTime(backup.completedAt)} />
      <Detail label="校验" value={`${verifyMeta[backup.verifyStatus].label}${backup.verifyMessage ? ` · ${backup.verifyMessage}` : ""}`} />
    </div>
    {backup.error ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{backup.error}</p> : null}
    <div>
      <h3 className="mb-2 text-sm font-semibold">磁盘产物</h3>
      {backup.disks?.length ? <div className="overflow-x-auto rounded-md border"><Table className="min-w-[620px]"><TableHeader><TableRow><TableHead>序号</TableHead><TableHead>目标</TableHead><TableHead>来源卷</TableHead><TableHead>产物卷</TableHead><TableHead>格式</TableHead><TableHead>容量</TableHead></TableRow></TableHeader><TableBody>{backup.disks.map((disk) => <TableRow key={disk.artifact}><TableCell>{disk.sequence + 1}</TableCell><TableCell>{disk.target || "-"}</TableCell><TableCell className="font-mono text-xs">{disk.sourceVolume}</TableCell><TableCell className="font-mono text-xs">{disk.artifact}</TableCell><TableCell>{disk.format}</TableCell><TableCell>{formatBytes(disk.capacityBytes)}</TableCell></TableRow>)}</TableBody></Table></div> : <p className="text-sm text-muted-foreground">暂无磁盘产物记录。</p>}
    </div>
  </div>;
}

function Detail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="rounded-md border bg-muted/20 px-3 py-2"><p className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</p><p className={cn("mt-1 break-all text-sm", mono && "font-mono text-xs")}>{value}</p></div>;
}

function CreateBackupModal({ open, onClose, repositories, vms, onCreated, onNotice }: {
  open: boolean;
  onClose: () => void;
  repositories: BackupRepository[];
  vms: Array<{ uuid: string; name: string; state: string }>;
  onCreated: (taskID: string) => void;
  onNotice: (notice: { tone: "success" | "error"; text: string }) => void;
}) {
  const [vmUuid, setVMUuid] = useState("");
  const [repositoryID, setRepositoryID] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const repo = repositories.find((item) => item.id === repositoryID);

  useEffect(() => {
    if (!open) return;
    const vm = vms[0];
    const repository = repositories.find((item) => item.isDefault) ?? repositories[0];
    setVMUuid(vm?.uuid ?? "");
    setName(vm ? backupName(vm.name) : "");
    setRepositoryID(repository?.id ?? "");
    setError("");
  }, [open, repositories, vms]);

  const mutation = useMutation({ mutationFn: api.createBackup });
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!vmUuid || !repo || !name.trim()) { setError("请选择关机状态的虚拟机、备份名称和存储库。"); return; }
    try {
      const task = await mutation.mutateAsync({ vmUuid, name: name.trim(), repositoryId: repo.id, targetPool: repo.poolName });
      onCreated(task.id);
    } catch (cause) {
      setError(errorText(cause, "创建备份失败"));
      onNotice({ tone: "error", text: errorText(cause, "创建备份失败") });
    }
  }
  return <Modal open={open} onClose={onClose} title="创建冷备份" description="仅支持已关机或异常停止的持久虚拟机；备份不包含内存状态。" testId="backup-create-modal" footer={<><Button variant="ghost" onClick={onClose}>取消</Button><Button form="backup-create-form" type="submit" disabled={mutation.isPending || !vms.length || !repositories.length}><Archive className="h-4 w-4" />{mutation.isPending ? "正在提交" : "开始备份"}</Button></>}>
    <form id="backup-create-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
      <div className="space-y-2"><Label htmlFor="backup-vm">虚拟机</Label><select id="backup-vm" data-testid="backup-create-vm" value={vmUuid} onChange={(event) => { const vm = vms.find((item) => item.uuid === event.target.value); setVMUuid(event.target.value); if (vm) setName(backupName(vm.name)); }} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
        {vms.length ? vms.map((vm) => <option key={vm.uuid} value={vm.uuid}>{vm.name} · {vm.state === "crashed" ? "异常停止" : "已关机"}</option>) : <option value="">没有可备份的关机虚拟机</option>}
      </select><p className="text-xs text-muted-foreground">运行中的虚拟机必须先正常关机，系统不会自动强制关机。</p></div>
      <div className="space-y-2"><Label htmlFor="backup-name">备份名称</Label><Input id="backup-name" data-testid="backup-create-name" value={name} onChange={(event) => setName(event.target.value)} maxLength={64} /></div>
      <div className="space-y-2"><Label htmlFor="backup-repository">备份存储库</Label><select id="backup-repository" data-testid="backup-create-repository" value={repositoryID} onChange={(event) => setRepositoryID(event.target.value)} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{repositories.map((repository) => <option key={repository.id} value={repository.id}>{repository.name} · {repository.poolName}{repository.isDefault ? " · 默认" : ""}</option>)}</select></div>
      {error ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{error}</p> : null}
    </form>
  </Modal>;
}

function VerifyBackupModal({ target, onClose, onStarted, onNotice }: {
  target: Backup | null;
  onClose: () => void;
  onStarted: (taskID: string) => void;
  onNotice: (notice: { tone: "success" | "error"; text: string }) => void;
}) {
  const [deep, setDeep] = useState(false);
  useEffect(() => { if (target) setDeep(false); }, [target]);
  return <Modal open={Boolean(target)} onClose={onClose} title="校验备份完整性" description={target ? `${target.name} · ${target.vmName}` : ""} testId="backup-verify-modal" footer={<><Button variant="ghost" onClick={onClose}>取消</Button><Button onClick={async () => { if (!target) return; try { const task = await api.verifyBackup(target.id, deep); onStarted(task.id); } catch (cause) { onNotice({ tone: "error", text: errorText(cause, "启动校验失败") }); } }}><ShieldCheck className="h-4 w-4" />开始校验</Button></>}>
    <div className="space-y-3 text-sm">
      <label className="flex items-start gap-3 rounded-md border p-3"><input type="checkbox" className="mt-1" checked={deep} onChange={(event) => setDeep(event.target.checked)} /><span><span className="font-medium">深度校验</span><span className="mt-1 block text-xs text-muted-foreground">额外检查每个磁盘产物是否存在、容量和格式是否匹配。不会计算整块磁盘 SHA-256，避免大磁盘产生持续 IO。</span></span></label>
      <p className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">快速校验会检查 manifest、配置 XML 的 SHA-256、以及全部磁盘产物是否存在。</p>
    </div>
  </Modal>;
}

function RestoreBackupModal({ target, repositories, onClose, onStarted, onNotice }: {
  target: Backup | null;
  repositories: BackupRepository[];
  onClose: () => void;
  onStarted: (taskID: string) => void;
  onNotice: (notice: { tone: "success" | "error"; text: string }) => void;
}) {
  const [form, setForm] = useState<VMRestoreInput>({ name: "", targetPool: "", autostart: false, start: false, preservePassthrough: false });
  const [error, setError] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);
  useEffect(() => {
    if (!target) return;
    setForm({ name: `${target.vmName}-restored`.slice(0, 64), targetPool: target.targetPool, autostart: false, start: false, preservePassthrough: false });
    setError("");
    setConfirmOpen(false);
  }, [target]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!target || !form.name.trim() || !form.targetPool) { setError("请填写新虚拟机名称并选择目标存储池。"); return; }
    setError("");
    setConfirmOpen(true);
  }
  async function execute() {
    if (!target) return;
    try {
      const task = await api.restoreBackup(target.id, { ...form, name: form.name.trim() });
      onStarted(task.id);
    } catch (cause) {
      setError(errorText(cause, "恢复虚拟机失败"));
      onNotice({ tone: "error", text: errorText(cause, "恢复虚拟机失败") });
    }
  }
  return <Modal open={Boolean(target)} onClose={onClose} title="恢复为新虚拟机" description="原备份和原虚拟机不会被修改；恢复会生成新的 UUID、MAC 和独立磁盘。" testId="backup-restore-modal" footer={confirmOpen ? <><Button variant="ghost" onClick={() => setConfirmOpen(false)}>返回修改</Button><Button onClick={() => void execute()}><RotateCcw className="h-4 w-4" />确认恢复</Button></> : <><Button variant="ghost" onClick={onClose}>取消</Button><Button form="backup-restore-form" type="submit"><RotateCcw className="h-4 w-4" />预检并恢复</Button></>}>
    {target ? <form id="backup-restore-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
      <div className="rounded-md border bg-muted/20 p-3 text-sm"><p className="font-medium">{target.name}</p><p className="mt-1 text-xs text-muted-foreground">来源 {target.vmName} · {formatBytes(target.totalBytes)} · {target.diskCount} 块磁盘</p></div>
      <div className="space-y-2"><Label htmlFor="restore-name">新虚拟机名称</Label><Input id="restore-name" data-testid="restore-name" value={form.name} maxLength={64} onChange={(event) => setForm({ ...form, name: event.target.value })} /></div>
      <div className="space-y-2"><Label htmlFor="restore-pool">目标存储池</Label><select id="restore-pool" data-testid="restore-pool" value={form.targetPool} onChange={(event) => setForm({ ...form, targetPool: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{[...new Set([target.targetPool, ...repositories.map((item) => item.poolName)])].map((pool) => <option key={pool} value={pool}>{pool}</option>)}</select></div>
      <div className="grid gap-2 sm:grid-cols-3">
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={form.autostart} onChange={(event) => setForm({ ...form, autostart: event.target.checked })} />开机自启动</label>
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={form.start} onChange={(event) => setForm({ ...form, start: event.target.checked })} />恢复后启动</label>
        <label className="flex items-center gap-2 rounded-md border p-3 text-sm"><input type="checkbox" checked={form.preservePassthrough} onChange={(event) => setForm({ ...form, preservePassthrough: event.target.checked })} />保留 PCI/USB 直通</label>
      </div>
      <p className="rounded-md border border-warning/30 bg-warning/10 p-3 text-xs text-warning">默认移除 PCI/USB 直通设备，避免与仍在运行的虚拟机争用设备。恢复前会检查名称冲突、存储池容量和备份产物。</p>
      {confirmOpen ? <div className="rounded-md border border-primary/30 bg-primary/5 p-3 text-sm">即将创建 <span className="font-semibold">{form.name}</span>，目标存储池为 <span className="font-mono">{form.targetPool}</span>。此操作会复制全部磁盘，可能耗时较长。</div> : null}
      {error ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">{error}</p> : null}
    </form> : null}
  </Modal>;
}

function SchedulesPanel({ role, readOnly, onTask, onNotice }: {
  role: Role | undefined;
  readOnly: boolean;
  onTask: (taskID: string, label: string) => void;
  onNotice: (notice: { tone: "success" | "error"; text: string }) => void;
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

  async function refresh() { await queryClient.invalidateQueries({ queryKey: backupSchedulesQueryKey("") }); }
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
          <TableCell>每 {schedule.intervalHours} 小时</TableCell>
          <TableCell>{schedule.retentionCount} 份{schedule.retentionDays ? ` / ${schedule.retentionDays} 天` : ""}</TableCell>
          <TableCell className="whitespace-nowrap text-xs">{formatTime(schedule.nextRunAt)}</TableCell>
          <TableCell><span className="text-xs">{formatTime(schedule.lastRunAt)}</span>{schedule.lastError ? <p className="mt-1 max-w-56 truncate text-xs text-destructive" title={schedule.lastError}>{schedule.lastError}</p> : null}</TableCell>
          <TableCell><Badge variant={schedule.enabled ? "success" : "secondary"}>{schedule.enabled ? "已启用" : "已暂停"}</Badge></TableCell>
          <TableCell><div className="flex flex-wrap justify-end gap-1">
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-edit-${schedule.id}`} disabled={Boolean(disabledReason)} title={disabledReason || "编辑计划"} onClick={() => { setEditing(schedule); setFormOpen(true); }}>编辑</Button>
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-toggle-${schedule.id}`} disabled={Boolean(disabledReason)} title={disabledReason || (schedule.enabled ? "暂停计划" : "启用计划")} onClick={async () => { try { await api.updateBackupSchedule(schedule.id, { repositoryId: schedule.repositoryId, name: schedule.name, intervalHours: schedule.intervalHours, retentionCount: schedule.retentionCount, retentionDays: schedule.retentionDays, enabled: !schedule.enabled }); await refresh(); onNotice({ tone: "success", text: schedule.enabled ? "计划已暂停。" : "计划已启用。" }); } catch (cause) { onNotice({ tone: "error", text: errorText(cause, "更新计划失败") }); } }}>{schedule.enabled ? "暂停" : "启用"}</Button>
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-run-${schedule.id}`} disabled={Boolean(disabledReason)} title={disabledReason || "立即执行一次"} onClick={async () => { try { const task = await api.runBackupSchedule(schedule.id); await refresh(); onTask(task.id, "执行定时备份"); } catch (cause) { onNotice({ tone: "error", text: errorText(cause, "立即执行失败") }); } }}>立即执行</Button>
            <Button variant="ghost" size="sm" data-testid={`backup-schedule-delete-${schedule.id}`} disabled={Boolean(disabledReason)} title={disabledReason || "删除计划"} onClick={() => setDeleteTarget(schedule)}><Trash2 className="h-4 w-4" />删除</Button>
          </div></TableCell>
        </TableRow>)}</TableBody></Table> : null}</CardContent>
    </Card>
    <ScheduleFormModal open={formOpen} onClose={() => setFormOpen(false)} editing={editing} repositories={repositories.data?.items ?? []} vms={eligibleVMs} onSaved={async () => { setFormOpen(false); await refresh(); onNotice({ tone: "success", text: editing ? "定时计划已更新。" : "定时计划已创建。" }); }} onError={(cause) => onNotice({ tone: "error", text: errorText(cause, "保存定时计划失败") })} />
    <Modal open={Boolean(deleteTarget)} onClose={() => setDeleteTarget(null)} title="删除定时计划" description="只删除调度配置，不会删除已经创建的备份。" footer={<><Button variant="ghost" onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="destructive" data-testid="backup-schedule-delete-confirm" onClick={async () => { if (!deleteTarget) return; try { await api.deleteBackupSchedule(deleteTarget.id); setDeleteTarget(null); await refresh(); onNotice({ tone: "success", text: "定时计划已删除。" }); } catch (cause) { onNotice({ tone: "error", text: errorText(cause, "删除定时计划失败") }); } }}><Trash2 className="h-4 w-4" />确认删除</Button></>}>
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
  const [form, setForm] = useState<BackupScheduleInput>({ vmUuid: "", repositoryId: "", name: "", intervalHours: 24, retentionCount: 7, retentionDays: 30, enabled: true });
  const [error, setError] = useState("");
  useEffect(() => {
    if (!open) return;
    if (editing) setForm({ vmUuid: editing.vmUuid, repositoryId: editing.repositoryId, name: editing.name, intervalHours: editing.intervalHours, retentionCount: editing.retentionCount, retentionDays: editing.retentionDays, enabled: editing.enabled });
    else setForm({ vmUuid: vms[0]?.uuid ?? "", repositoryId: repositories.find((item) => item.isDefault)?.id ?? repositories[0]?.id ?? "", name: "", intervalHours: 24, retentionCount: 7, retentionDays: 30, enabled: true });
    setError("");
  }, [editing, open, repositories, vms]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!form.vmUuid || !form.repositoryId || !form.name.trim()) { setError("请选择虚拟机、存储库并填写计划名称。"); return; }
    try {
      if (editing) {
        await api.updateBackupSchedule(editing.id, { repositoryId: form.repositoryId, name: form.name.trim(), intervalHours: form.intervalHours, retentionCount: form.retentionCount, retentionDays: form.retentionDays, enabled: form.enabled });
      } else {
        await api.createBackupSchedule({ ...form, name: form.name.trim() });
      }
      await onSaved();
    } catch (cause) {
      setError(errorText(cause, "保存定时计划失败"));
      onError(cause);
    }
  }
  return <Modal open={open} onClose={onClose} title={editing ? "编辑定时计划" : "新建定时计划"} description="首期使用固定间隔，不引入 Cron 表达式；时区跟随宿主机。" testId="backup-schedule-modal" footer={<><Button variant="ghost" onClick={onClose}>取消</Button><Button form="backup-schedule-form" type="submit">{editing ? "保存修改" : "创建计划"}</Button></>}>
    <form id="backup-schedule-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
      <div className="space-y-2"><Label htmlFor="schedule-vm">虚拟机</Label><select id="schedule-vm" data-testid="backup-schedule-vm" disabled={Boolean(editing)} value={form.vmUuid} onChange={(event) => setForm({ ...form, vmUuid: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm disabled:opacity-60">{vms.map((vm) => <option key={vm.uuid} value={vm.uuid}>{vm.name}</option>)}</select></div>
      <div className="space-y-2"><Label htmlFor="schedule-name">计划名称</Label><Input id="schedule-name" data-testid="backup-schedule-name" value={form.name} maxLength={32} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="例如 daily-backup" /></div>
      <div className="space-y-2"><Label htmlFor="schedule-repository">备份存储库</Label><select id="schedule-repository" data-testid="backup-schedule-repository" value={form.repositoryId} onChange={(event) => setForm({ ...form, repositoryId: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">{repositories.map((repository) => <option key={repository.id} value={repository.id}>{repository.name} · {repository.poolName}</option>)}</select></div>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="space-y-2"><Label htmlFor="schedule-interval">间隔（小时）</Label><Input id="schedule-interval" data-testid="backup-schedule-interval" type="number" min={1} max={168} value={form.intervalHours} onChange={(event) => setForm({ ...form, intervalHours: Number(event.target.value) })} /></div>
        <div className="space-y-2"><Label htmlFor="schedule-retention">保留份数</Label><Input id="schedule-retention" data-testid="backup-schedule-retention" type="number" min={1} max={50} value={form.retentionCount} onChange={(event) => setForm({ ...form, retentionCount: Number(event.target.value) })} /></div>
        <div className="space-y-2"><Label htmlFor="schedule-days">最大保留天数</Label><Input id="schedule-days" data-testid="backup-schedule-days" type="number" min={0} max={3650} value={form.retentionDays} onChange={(event) => setForm({ ...form, retentionDays: Number(event.target.value) })} /></div>
      </div>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} />创建后立即启用</label>
      <p className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">保留策略只清理该计划创建且未固定的成功备份，始终至少保留最近 N 份。最大保留天数为 0 时按份数保留。</p>
      {error ? <p role="alert" className="text-xs text-destructive">{error}</p> : null}
    </form>
  </Modal>;
}

function RepositoriesPanel({ role, readOnly, onNotice }: {
  role: Role | undefined;
  readOnly: boolean;
  onNotice: (notice: { tone: "success" | "error"; text: string }) => void;
}) {
  const queryClient = useQueryClient();
  const repositories = useBackupRepositoriesQuery();
  const pools = useStoragePoolsQuery();
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<BackupRepository | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<BackupRepository | null>(null);
  const [form, setForm] = useState<BackupRepositoryInput>({ name: "", poolName: "", isDefault: false });
  const [error, setError] = useState("");
  const disabledReason = actionReason(role, readOnly, "admin");
  const items = repositories.data?.items ?? [];
  const activePools = (pools.data?.items ?? []).filter((pool) => pool.state === "active");
  function openCreate() { setEditing(null); setForm({ name: "", poolName: activePools[0]?.name ?? "", isDefault: !items.some((item) => item.isDefault) }); setError(""); setFormOpen(true); }
  function openEdit(repository: BackupRepository) { setEditing(repository); setForm({ name: repository.name, poolName: repository.poolName, isDefault: repository.isDefault }); setError(""); setFormOpen(true); }
  async function refresh() { await queryClient.invalidateQueries({ queryKey: backupRepositoriesQueryKey }); }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      if (editing) await api.updateBackupRepository(editing.id, form);
      else await api.createBackupRepository(form);
      setFormOpen(false); await refresh(); onNotice({ tone: "success", text: editing ? "备份存储库已更新。" : "备份存储库已创建。" });
    } catch (cause) { setError(errorText(cause, "保存备份存储库失败")); onNotice({ tone: "error", text: errorText(cause, "保存备份存储库失败") }); }
  }
  return <>
    <Card>
      <CardHeader><div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle>备份存储库</CardTitle><CardDescription>首期仅支持活跃的 libvirt 存储池。建议使用独立磁盘或独立 NFS 目录，不与虚拟机主磁盘共用。</CardDescription></div><Button size="sm" data-testid="backup-repository-create" disabled={Boolean(disabledReason)} title={disabledReason || "添加备份存储库"} onClick={openCreate}><Plus className="h-4 w-4" />添加存储库</Button></div></CardHeader>
      <CardContent className="p-0">
        {repositories.isLoading ? <LoadingState label="正在读取备份存储库" /> : null}
        {repositories.isError ? <ErrorState className="m-4" message={errorText(repositories.error, "无法读取备份存储库")} onRetry={() => void repositories.refetch()} /> : null}
        {!repositories.isLoading && !repositories.isError && items.length === 0 ? <div className="px-5 py-16 text-center" data-testid="backup-repositories-empty"><Database className="mx-auto h-8 w-8 text-muted-foreground" /><p className="mt-3 font-medium">没有可用的备份存储库</p><p className="mt-1 text-sm text-muted-foreground">先创建并启动 libvirt 存储池，再添加为备份存储库。</p></div> : null}
        {items.length ? <Table className="min-w-[760px]"><TableHeader><TableRow><TableHead>名称</TableHead><TableHead>类型</TableHead><TableHead>libvirt 存储池</TableHead><TableHead>默认</TableHead><TableHead>更新时间</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{items.map((repository) => <TableRow key={repository.id} data-testid={`backup-repository-row-${repository.id}`}><TableCell className="font-medium">{repository.name}</TableCell><TableCell><Badge variant="outline">libvirt-pool</Badge></TableCell><TableCell className="font-mono text-xs">{repository.poolName}</TableCell><TableCell>{repository.isDefault ? <Badge variant="success">默认</Badge> : <span className="text-muted-foreground">-</span>}</TableCell><TableCell className="text-xs">{formatTime(repository.updatedAt)}</TableCell><TableCell><div className="flex justify-end gap-1"><Button variant="ghost" size="sm" disabled={Boolean(disabledReason)} title={disabledReason || "编辑存储库"} data-testid={`backup-repository-edit-${repository.id}`} onClick={() => openEdit(repository)}>编辑</Button><Button variant="ghost" size="sm" disabled={Boolean(disabledReason)} title={disabledReason || "删除存储库；仍有关联备份或计划时会被阻止"} data-testid={`backup-repository-delete-${repository.id}`} onClick={() => setDeleteTarget(repository)}><Trash2 className="h-4 w-4" />删除</Button></div></TableCell></TableRow>)}</TableBody></Table> : null}</CardContent>
    </Card>
    <Modal open={formOpen} onClose={() => setFormOpen(false)} title={editing ? "编辑备份存储库" : "添加备份存储库"} description="只有活跃的 libvirt 存储池可以被添加；存储池名称创建后不可更改。" testId="backup-repository-modal" footer={<><Button variant="ghost" onClick={() => setFormOpen(false)}>取消</Button><Button form="backup-repository-form" type="submit">{editing ? "保存修改" : "添加存储库"}</Button></>}>
      <form id="backup-repository-form" className="space-y-4" onSubmit={(event) => void submit(event)}>
        <div className="space-y-2"><Label htmlFor="repository-name">名称</Label><Input id="repository-name" data-testid="backup-repository-name" value={form.name} maxLength={64} onChange={(event) => setForm({ ...form, name: event.target.value })} /></div>
        <div className="space-y-2"><Label htmlFor="repository-pool">libvirt 存储池</Label><select id="repository-pool" data-testid="backup-repository-pool" disabled={Boolean(editing)} value={form.poolName} onChange={(event) => setForm({ ...form, poolName: event.target.value })} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm disabled:opacity-60">{editing && !activePools.some((pool) => pool.name === form.poolName) ? <option value={form.poolName}>{form.poolName}</option> : null}{activePools.map((pool) => <option key={pool.name} value={pool.name}>{pool.name} · 可用 {formatBytes(pool.availableBytes)}</option>)}</select></div>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.isDefault} onChange={(event) => setForm({ ...form, isDefault: event.target.checked })} />设为默认备份存储库</label>
        {error ? <p role="alert" className="text-xs text-destructive">{error}</p> : null}
      </form>
    </Modal>
    <Modal open={Boolean(deleteTarget)} onClose={() => setDeleteTarget(null)} title="删除备份存储库" description="不会删除存储池或备份产物；仍有关联备份和定时计划时删除会被阻止。" footer={<><Button variant="ghost" onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="destructive" data-testid="backup-repository-delete-confirm" onClick={async () => { if (!deleteTarget) return; try { await api.deleteBackupRepository(deleteTarget.id); setDeleteTarget(null); await refresh(); onNotice({ tone: "success", text: "备份存储库已删除。" }); } catch (cause) { onNotice({ tone: "error", text: errorText(cause, "删除备份存储库失败") }); setDeleteTarget(null); } }}><Trash2 className="h-4 w-4" />确认删除</Button></>}>
      {deleteTarget ? <p className="text-sm">确认删除备份存储库 <span className="font-medium">{deleteTarget.name}</span>？</p> : null}
    </Modal>
  </>;
}
