import { Link } from "@tanstack/react-router";
import { Archive, ArrowRight } from "lucide-react";
import type { BackupStatus } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useBackupsQuery } from "@/features/backups/use-backups-query";

const statusLabels: Record<BackupStatus, string> = {
  running: "备份中",
  completed: "已完成",
  failed: "失败",
  interrupted: "已中断",
  deleting: "删除中",
  delete_failed: "删除失败",
};

const statusTones: Record<BackupStatus, "success" | "warning" | "destructive" | "secondary"> = {
  running: "warning",
  completed: "success",
  failed: "destructive",
  interrupted: "secondary",
  deleting: "warning",
  delete_failed: "destructive",
};

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

function formatBytes(value: number) {
  if (value <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size >= 100 || index === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

export function VMRecentBackupsCard({ uuid }: { uuid: string }) {
  const query = useBackupsQuery({ vmUuid: uuid, pageSize: 5 });
  const items = query.data?.items ?? [];
  return (
    <Card className="mt-4" data-testid="vm-recent-backups-card">
      <CardHeader className="flex-row items-start justify-between gap-3">
        <div>
          <CardTitle className="flex items-center gap-2"><Archive className="h-4 w-4 text-primary" />最近备份</CardTitle>
          <CardDescription>该虚拟机最近 5 个备份集。校验、恢复和删除统一在备份中心完成。</CardDescription>
        </div>
        <Link to="/backups" className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-primary hover:underline">备份中心<ArrowRight className="h-3.5 w-3.5" /></Link>
      </CardHeader>
      <CardContent className="p-0">
        {query.isLoading ? <p className="px-5 pb-5 text-sm text-muted-foreground">正在读取最近备份…</p> : null}
        {query.isError ? <p role="alert" className="mx-5 mb-5 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive">无法读取最近备份：{query.error instanceof Error ? query.error.message : "请稍后重试"}</p> : null}
        {!query.isLoading && !query.isError && items.length === 0 ? <p className="px-5 pb-5 text-sm text-muted-foreground">还没有备份记录。关机后可创建首个冷备份。</p> : null}
        {items.length ? <Table className="min-w-[640px]"><TableHeader><TableRow><TableHead>备份</TableHead><TableHead>状态</TableHead><TableHead>大小</TableHead><TableHead>创建时间</TableHead><TableHead>校验</TableHead></TableRow></TableHeader><TableBody>{items.map((backup) => <TableRow key={backup.id} data-testid={`vm-recent-backup-${backup.id}`}><TableCell><p className="max-w-64 truncate font-medium">{backup.name}</p><p className="font-mono text-[10px] text-muted-foreground">{backup.id}</p></TableCell><TableCell><Badge variant={statusTones[backup.status]}>{statusLabels[backup.status]}</Badge></TableCell><TableCell className="whitespace-nowrap">{formatBytes(backup.totalBytes)}</TableCell><TableCell className="whitespace-nowrap text-xs">{formatTime(backup.completedAt || backup.createdAt)}</TableCell><TableCell><Badge variant={backup.verifyStatus === "verified" ? "success" : backup.verifyStatus === "failed" ? "destructive" : "secondary"}>{backup.verifyStatus === "verified" ? "通过" : backup.verifyStatus === "failed" ? "失败" : "未校验"}</Badge></TableCell></TableRow>)}</TableBody></Table> : null}
      </CardContent>
    </Card>
  );
}
