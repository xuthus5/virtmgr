import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Archive, CalendarClock, CheckCircle2, ChevronLeft, ChevronRight, Database,
  Download, HardDrive, LoaderCircle, Plus, RefreshCw, RotateCcw, ShieldCheck, Trash2, TriangleAlert, Upload,
} from "lucide-react";
import { api, backupExportUrl } from "@/api/client";
import type { Backup, BackupStatus } from "@/api/types";
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
import { CreateBackupModal, RestoreBackupModal, VerifyBackupModal } from "@/features/backups/backup-dialogs";
import { BackupActions, BackupDetail } from "@/features/backups/backup-list";
import { RepositoriesPanel } from "@/features/backups/backup-repositories-panel";
import { SchedulesPanel } from "@/features/backups/backup-schedules-panel";
import {
  actionReason, backupPageSize, backupStatusMeta, errorText, formatBytes, formatTime,
  verifyMeta, type BackupNotice, type BackupTab,
} from "@/features/backups/backup-ui";
import {
  useBackupRepositoriesQuery, useBackupSummaryQuery, useBackupsQuery,
} from "@/features/backups/use-backups-query";
import { useTaskQuery } from "@/features/tasks/use-tasks-query";
import { useVMsQuery } from "@/features/vms/use-vms-query";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";

export function BackupsPage() {
  const session = useSessionQuery();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<BackupTab>("backups");
  const [status, setStatus] = useState<BackupStatus | "">("");
  const [repositoryFilter, setRepositoryFilter] = useState("");
  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);
  const [detail, setDetail] = useState<Backup | null>(null);
  const [verifyTarget, setVerifyTarget] = useState<Backup | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Backup | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState("");
  const [exportingID, setExportingID] = useState<string | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importName, setImportName] = useState("");
  const [importRepositoryID, setImportRepositoryID] = useState("");
  const [notice, setNotice] = useState<BackupNotice | null>(null);
  const [activeTaskID, setActiveTaskID] = useState<string | null>(null);
  const [taskLabel, setTaskLabel] = useState("任务");

  const role = session.data?.user.role;
  const readOnly = Boolean(session.data?.readOnly);
  const backupsQuery = useBackupsQuery({ status, repositoryId: repositoryFilter, page, pageSize: backupPageSize }, tab === "backups");
  const summaryQuery = useBackupSummaryQuery();
  const repositoriesQuery = useBackupRepositoriesQuery();
  const vmsQuery = useVMsQuery("", "", Boolean(session.data) && createOpen);
  const activeTask = useTaskQuery(activeTaskID);
  const pinMutation = useMutation({
    mutationFn: ({ id, pinned }: { id: string; pinned: boolean }) => api.pinBackup(id, { pinned }),
  });
  const deleteMutation = useMutation({ mutationFn: api.deleteBackup });
  const importMutation = useMutation({
    mutationFn: ({ file, name, repositoryId }: { file: File; name: string; repositoryId: string }) => api.importBackup(file, { name, repositoryId }),
    onSuccess: (backup) => {
      setImportOpen(false);
      setImportFile(null);
      setImportName("");
      setImportRepositoryID("");
      setNotice({ tone: "success", text: `备份包已导入为“${backup.name}”，可在备份集列表中选择恢复。` });
      void queryClient.invalidateQueries({ queryKey: queryKeys.backups.root });
      void queryClient.invalidateQueries({ queryKey: queryKeys.storage.pools });
    },
    onError: (cause) => setNotice({ tone: "error", text: errorText(cause, "导入备份包失败") }),
  });
  const backups = backupsQuery.data?.items ?? [];
  const repositories = repositoriesQuery.data?.items ?? [];
  const total = backupsQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / backupPageSize));
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
    void queryClient.invalidateQueries({ queryKey: queryKeys.backups.root });
    void queryClient.invalidateQueries({ queryKey: queryKeys.storage.pools });
    void queryClient.invalidateQueries({ queryKey: queryKeys.tasks.root });
    void queryClient.invalidateQueries({ queryKey: queryKeys.vms.root });
  }, [activeTask.data, queryClient, taskLabel]);

  function beginTask(taskID: string, label: string) {
    setTaskLabel(label);
    setActiveTaskID(taskID);
    setNotice(null);
  }

  async function togglePin(backup: Backup) {
    try {
      await pinMutation.mutateAsync({ id: backup.id, pinned: !backup.pinned });
      setNotice({ tone: "success", text: backup.pinned ? "已取消固定保留。" : "已固定保留，自动清理不会删除该备份。" });
      await queryClient.invalidateQueries({ queryKey: queryKeys.backups.root });
    } catch (cause) {
      setNotice({ tone: "error", text: errorText(cause, "更新固定保留状态失败") });
    }
  }

  async function deleteBackup() {
    if (!deleteTarget || deleteMutation.isPending) return;
    try {
      const task = await deleteMutation.mutateAsync(deleteTarget.id);
      setDeleteTarget(null);
      beginTask(task.id, "删除备份");
    } catch (cause) {
      setNotice({ tone: "error", text: errorText(cause, "删除备份失败") });
    }
  }

  function exportBackup(backup: Backup) {
    if (exportingID) return;
    setExportingID(backup.id);
    setNotice(null);
    const anchor = document.createElement("a");
    anchor.href = backupExportUrl(backup.id);
    anchor.rel = "noopener";
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    setNotice({ tone: "success", text: `已开始下载“${backup.name}”备份包。` });
    setExportingID(null);
  }

  return (
    <>
      <PageHeader
        title="备份中心"
        breadcrumb="运行与资源 / 备份中心"
        description="集中管理虚拟机冷备份、在线一致性备份、完整性校验、恢复为新虚拟机和定时保留策略。"
        actions={<Button variant="outline" size="sm" disabled={backupsQuery.isFetching} onClick={() => void queryClient.invalidateQueries({ queryKey: queryKeys.backups.root })}><RefreshCw className={cn("h-4 w-4", backupsQuery.isFetching && "animate-spin")} />刷新</Button>}
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
        {tab === "backups" ? <div className="flex flex-wrap items-center gap-2 sm:ml-auto">
          <Button variant="outline" size="sm" data-testid="backup-import-open" disabled={Boolean(actionReason(role, readOnly, "create"))} title={actionReason(role, readOnly, "create") || "从 VirtMgr 备份 ZIP 包导入备份集"} onClick={() => setImportOpen(true)}><Upload className="h-4 w-4" />导入备份包</Button>
          <Button size="sm" data-testid="backup-create-open" disabled={Boolean(actionReason(role, readOnly, "create"))} title={actionReason(role, readOnly, "create") || "创建虚拟机冷备份或在线一致性备份"} onClick={() => setCreateOpen(true)}><Plus className="h-4 w-4" />创建备份</Button>
        </div> : null}
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
                <p className="mt-1 text-sm text-muted-foreground">关机虚拟机可执行冷备份，运行中虚拟机可执行在线一致性备份；备份不会包含内存状态。</p>
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
                    <TableCell><BackupActions
                      backup={backup}
                      role={role}
                      readOnly={readOnly}
                      pinPending={pinMutation.isPending && pinMutation.variables?.id === backup.id}
                      exportPending={exportingID === backup.id}
                      onDetail={() => setDetail(backup)}
                      onExport={() => exportBackup(backup)}
                      onVerify={() => setVerifyTarget(backup)}
                      onRestore={() => setRestoreTarget(backup)}
                      onDelete={() => { setDeleteTarget(backup); setDeleteConfirm(""); }}
                      onPin={() => void togglePin(backup)}
                    /></TableCell>
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
        vms={(vmsQuery.data?.items ?? []).filter((vm) => vm.persistent && (vm.state === "shutoff" || vm.state === "crashed" || vm.state === "running"))}
        onCreated={(taskID) => { setCreateOpen(false); beginTask(taskID, "创建备份"); }}
        onNotice={setNotice}
      />

      <Modal open={Boolean(detail)} onClose={() => setDetail(null)} title={detail?.name ?? "备份详情"} description="备份产物清单与校验信息" testId="backup-detail-modal" className="max-w-3xl" footer={detail ? <>
        <Button variant="outline" size="sm" disabled={Boolean(actionReason(role, readOnly, "create")) || detail.status !== "completed" || exportingID === detail.id} title={actionReason(role, readOnly, "create") || (detail.status !== "completed" ? "只有已完成的备份可导出" : "下载可移植备份 ZIP 包")} onClick={() => exportBackup(detail)}>{exportingID === detail.id ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}{exportingID === detail.id ? "下载中" : "下载备份包"}</Button>
        <Button variant="outline" size="sm" disabled={Boolean(actionReason(role, readOnly, "verify"))} title={actionReason(role, readOnly, "verify") || "校验备份完整性"} onClick={() => { setVerifyTarget(detail); setDetail(null); }}><ShieldCheck className="h-4 w-4" />校验</Button>
        <Button variant="outline" size="sm" disabled={Boolean(actionReason(role, readOnly, "restore")) || detail.status !== "completed"} title={actionReason(role, readOnly, "restore") || (detail.status !== "completed" ? "只有已完成的备份可恢复" : "恢复为新虚拟机")} onClick={() => { setRestoreTarget(detail); setDetail(null); }}><RotateCcw className="h-4 w-4" />恢复为新虚拟机</Button>
      </> : null}>
        {detail ? <BackupDetail backup={detail} /> : null}
      </Modal>

      <VerifyBackupModal target={verifyTarget} onClose={() => setVerifyTarget(null)} onStarted={(taskID) => { setVerifyTarget(null); beginTask(taskID, "校验备份"); }} onNotice={setNotice} />

      <RestoreBackupModal target={restoreTarget} repositories={repositories} onClose={() => setRestoreTarget(null)} onStarted={(taskID) => { setRestoreTarget(null); beginTask(taskID, "恢复虚拟机"); }} onNotice={setNotice} />

      <Modal open={importOpen} onClose={() => { if (!importMutation.isPending) setImportOpen(false); }} title="导入备份包" description="上传由 VirtMgr 导出的 .zip 备份包。导入只登记备份集，不会自动创建或覆盖虚拟机。" testId="backup-import-modal" footer={<>
        <Button variant="ghost" disabled={importMutation.isPending} onClick={() => setImportOpen(false)}>取消</Button>
        <Button data-testid="backup-import-submit" disabled={!importFile || importMutation.isPending} onClick={() => { if (importFile) importMutation.mutate({ file: importFile, name: importName, repositoryId: importRepositoryID }); }}>{importMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}{importMutation.isPending ? "正在导入..." : "上传并导入"}</Button>
      </>}>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="backup-import-file">备份包文件</Label>
            <Input id="backup-import-file" data-testid="backup-import-file" type="file" accept=".zip,application/zip" disabled={importMutation.isPending} onChange={(event) => setImportFile(event.target.files?.[0] ?? null)} />
            {importFile ? <p className="break-all text-xs text-muted-foreground">{importFile.name} · {formatBytes(importFile.size)}</p> : <p className="text-xs text-muted-foreground">文件在服务器端会再次校验 ZIP 结构、路径、条目数量和解压大小。</p>}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="backup-import-name">备份名称（可选）</Label>
              <Input id="backup-import-name" data-testid="backup-import-name" value={importName} maxLength={64} placeholder="留空沿用包内名称" onChange={(event) => setImportName(event.target.value)} disabled={importMutation.isPending} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="backup-import-repository">备份存储库</Label>
              <select id="backup-import-repository" data-testid="backup-import-repository" value={importRepositoryID} onChange={(event) => setImportRepositoryID(event.target.value)} disabled={importMutation.isPending} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
                <option value="">使用默认存储库</option>
                {repositories.map((repository) => <option key={repository.id} value={repository.id}>{repository.name} · {repository.poolName}{repository.isDefault ? " · 默认" : ""}</option>)}
              </select>
            </div>
          </div>
          <p className="rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">导入会为备份集生成新的 ID 和产物卷名，不会覆盖已有备份。最大包大小 512 GiB，仅接受 VirtMgr v1 备份包。</p>
        </div>
      </Modal>

      <Modal open={Boolean(deleteTarget)} onClose={() => setDeleteTarget(null)} title="删除备份集" description="删除会移除存储池中的配置、清单和全部磁盘副本，无法撤销。" testId="backup-delete-modal" footer={<>
        <Button variant="ghost" onClick={() => setDeleteTarget(null)}>取消</Button>
        <Button variant="destructive" data-testid="backup-delete-confirm" disabled={deleteMutation.isPending || !deleteTarget || deleteConfirm !== deleteTarget.name} onClick={() => void deleteBackup()}>{deleteMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}{deleteMutation.isPending ? "删除中..." : "确认删除"}</Button>
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
