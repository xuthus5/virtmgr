import { Download, Info, LoaderCircle, Pin, PinOff, RotateCcw, ShieldCheck, Trash2 } from "lucide-react";
import type { Backup, Role } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { actionReason, backupStatusMeta, formatBytes, formatTime, verifyMeta } from "@/features/backups/backup-ui";
import { cn } from "@/lib/utils";


export function BackupActions({ backup, role, readOnly, pinPending, exportPending, onDetail, onExport, onVerify, onRestore, onDelete, onPin }: {
  backup: Backup;
  role: Role | undefined;
  readOnly: boolean;
  pinPending: boolean;
  exportPending: boolean;
  onDetail: () => void;
  onExport: () => void;
  onVerify: () => void;
  onRestore: () => void;
  onDelete: () => void;
  onPin: () => void;
}) {
  const verifyReason = actionReason(role, readOnly, "verify") || (backup.status !== "completed" ? "只有已完成的备份可校验" : "");
  const restoreReason = actionReason(role, readOnly, "restore") || (backup.status !== "completed" ? "只有已完成的备份可恢复" : "");
  const deleteReason = actionReason(role, readOnly, "delete") || (backup.status === "running" || backup.status === "deleting" ? "执行中的备份不能删除" : "");
  const exportReason = actionReason(role, readOnly, "create") || (backup.status !== "completed" ? "只有已完成的备份可导出" : "");
  return <div className="flex flex-wrap justify-end gap-1">
    <Button variant="ghost" size="sm" title="查看备份详情" aria-label={`查看 ${backup.name} 详情`} data-testid={`backup-detail-${backup.id}`} onClick={onDetail}><Info className="h-4 w-4" />详情</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(exportReason) || exportPending} title={exportReason || "下载可移植备份 ZIP 包"} aria-label={`下载 ${backup.name} 备份包`} data-testid={`backup-export-${backup.id}`} onClick={onExport}>{exportPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}{exportPending ? "下载中" : "下载包"}</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(verifyReason)} title={verifyReason || "校验备份完整性"} aria-label={`校验 ${backup.name}`} data-testid={`backup-verify-${backup.id}`} onClick={onVerify}><ShieldCheck className="h-4 w-4" />校验</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(restoreReason)} title={restoreReason || "恢复为一台新虚拟机"} aria-label={`恢复 ${backup.name}`} data-testid={`backup-restore-${backup.id}`} onClick={onRestore}><RotateCcw className="h-4 w-4" />恢复</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(actionReason(role, readOnly, "create")) || pinPending} title={pinPending ? "正在更新固定保留状态" : actionReason(role, readOnly, "create") || (backup.pinned ? "取消固定保留" : "固定保留，自动清理时跳过")} aria-label={backup.pinned ? `取消固定 ${backup.name}` : `固定 ${backup.name}`} data-testid={`backup-pin-${backup.id}`} onClick={onPin}>{pinPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : backup.pinned ? <PinOff className="h-4 w-4" /> : <Pin className="h-4 w-4" />}{pinPending ? "更新中" : backup.pinned ? "取消固定" : "固定"}</Button>
    <Button variant="ghost" size="sm" disabled={Boolean(deleteReason)} title={deleteReason || "删除备份集"} aria-label={`删除 ${backup.name}`} data-testid={`backup-delete-${backup.id}`} onClick={onDelete}><Trash2 className="h-4 w-4" />删除</Button>
  </div>;
}

export function BackupDetail({ backup }: { backup: Backup }) {
  return <div className="space-y-4">
    <div className="grid gap-3 sm:grid-cols-2">
      <Detail label="虚拟机" value={backup.vmName} />
      <Detail label="备份 ID" value={backup.id} mono />
      <Detail label="状态" value={backupStatusMeta[backup.status].label} />
      <Detail label="格式" value={backup.formatVersion} mono />
      <Detail label="存储库" value={`${backup.repositoryName || "-"} / ${backup.targetPool}`} />
      <Detail label="总大小" value={formatBytes(backup.totalBytes)} />
      <Detail label="备份模式" value={backup.online ? `在线${backup.quiesced ? " · Agent 静默" : ""}` : "冷备份"} />
      <Detail label="一致性" value={backup.consistent ? "磁盘快照一致性" : "离线静态副本"} />
      <Detail label="vTPM" value={backup.containsTpm ? (backup.tpmStateSaved ? "设备与状态已保存" : "仅保存配置，状态未保存") : "未使用"} />
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
