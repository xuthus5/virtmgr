import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Archive, DatabaseBackup, Info, LoaderCircle, LockKeyhole, ShieldCheck, Unlock } from "lucide-react";
import { ApiError, api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { sessionQueryKey, useSessionQuery } from "@/features/auth/use-session";
import { settingsQueryKey, useSettingsQuery } from "@/features/settings/use-settings-query";

export function SettingsPage() {
  const session = useSessionQuery();
  const settings = useSettingsQuery(Boolean(session.data));
  const queryClient = useQueryClient();
  const [confirmWrite, setConfirmWrite] = useState(false);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [retentionDays, setRetentionDays] = useState(30);
  const systemQuery = useQuery({ queryKey: ["system", "info"], queryFn: api.systemInfo, staleTime: 60_000 });

  useEffect(() => {
    if (settings.data?.auditRetentionDays) setRetentionDays(settings.data.auditRetentionDays);
  }, [settings.data?.auditRetentionDays]);

  const mutation = useMutation({
    mutationFn: (readOnly: boolean) => api.updateSettings({ readOnly }),
    onSuccess: (result) => {
      queryClient.setQueryData(settingsQueryKey, result);
      queryClient.setQueryData(sessionQueryKey, (current: typeof session.data) => current ? { ...current, readOnly: result.readOnly } : current);
      void queryClient.invalidateQueries({ queryKey: ["vms"] });
      void queryClient.invalidateQueries({ queryKey: ["storage"] });
      void queryClient.invalidateQueries({ queryKey: ["networks"] });
      void queryClient.invalidateQueries({ queryKey: ["users"] });
      setNotice({ tone: "success", text: result.readOnly ? "只读模式已启用，所有写操作现已阻止。" : "只读模式已关闭，Operator 和 Admin 可以执行写操作。" });
      setConfirmWrite(false);
    },
    onError: (error) => {
      setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "更新运行模式失败，请稍后重试。" });
      setConfirmWrite(false);
    },
  });

  const exportMutation = useMutation({
    mutationFn: api.exportDatabase,
    onSuccess: ({ blob, filename }) => {
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = filename;
      anchor.click();
      URL.revokeObjectURL(url);
      setNotice({ tone: "success", text: `数据库在线导出已下载：${filename}` });
    },
    onError: (error) => setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "数据库导出失败，请稍后重试。" }),
  });

  const retentionMutation = useMutation({
    mutationFn: (days: number) => api.updateAuditRetention(days),
    onSuccess: (result) => {
      queryClient.setQueryData(settingsQueryKey, { ...settings.data, ...result });
      setNotice({ tone: "success", text: `审计日志保留期已更新为 ${result.auditRetentionDays} 天。` });
    },
    onError: (error) => setNotice({ tone: "error", text: error instanceof ApiError ? error.message : "更新审计保留期失败，请稍后重试。" }),
  });

  if (session.isLoading || settings.isLoading) return <LoadingState label="正在读取系统设置" />;
  if (session.data?.user.role !== "admin") {
    return <ErrorState message="当前账号没有访问系统设置的权限" />;
  }
  if (settings.isError || !settings.data) {
    return <ErrorState message={settings.error instanceof Error ? settings.error.message : "无法读取系统设置"} onRetry={() => void settings.refetch()} />;
  }

  const readOnly = settings.data.readOnly;

  return (
    <div data-testid="settings-page">
      <PageHeader title="系统设置" breadcrumb="管理 / 系统设置" description="管理全局运行模式。设置会持久化到 SQLite，并在当前进程内立即生效。" />
      <div className="grid gap-4 xl:grid-cols-[1.35fr_0.65fr]">
        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <CardTitle>运行模式</CardTitle>
                <CardDescription>只读模式会阻止虚拟机、存储、网络和用户的所有写操作，但不阻止管理员修改此开关。</CardDescription>
              </div>
              <Badge variant={readOnly ? "secondary" : "warning"} data-testid="settings-readonly-status">
                {readOnly ? "只读模式已启用" : "写操作已启用"}
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="space-y-5">
            <div className={readOnly ? "flex items-start gap-3 rounded-lg border border-success/35 bg-success/10 px-4 py-3 text-sm" : "flex items-start gap-3 rounded-lg border border-warning/35 bg-warning/10 px-4 py-3 text-sm text-warning"}>
              {readOnly ? <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-success" /> : <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />}
              <p>{readOnly ? "当前处于安全状态。Viewer 可继续查看，所有写请求会返回 READ_ONLY。" : "当前允许 Operator 和 Admin 修改基础设施。请确认只对受信用户开放写权限。"}</p>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              {readOnly ? (
                <Button data-testid="settings-toggle-readonly" disabled={mutation.isPending} onClick={() => setConfirmWrite(true)}>
                  {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Unlock className="h-4 w-4" />}关闭只读模式
                </Button>
              ) : (
                <Button variant="outline" data-testid="settings-toggle-readonly" disabled={mutation.isPending} onClick={() => mutation.mutate(true)}>
                  {mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <LockKeyhole className="h-4 w-4" />}启用只读模式
                </Button>
              )}
              <span className="text-xs text-muted-foreground">修改操作会写入审计日志：`settings.readonly.update`。</span>
            </div>
            {notice ? (
              <p role="status" data-testid="settings-success" className={notice.tone === "error" ? "rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive" : "rounded-md border border-success/30 bg-success/5 px-3 py-2 text-sm text-success"}>
                {notice.text}
              </p>
            ) : null}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>权限边界</CardTitle>
            <CardDescription>运行模式不会改变账号角色。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <div className="rounded-md border bg-muted/30 p-3"><p className="font-medium">Viewer</p><p className="mt-1 text-xs text-muted-foreground">始终只读，不能执行写操作或打开交互式控制台。</p></div>
            <div className="rounded-md border bg-muted/30 p-3"><p className="font-medium">Operator</p><p className="mt-1 text-xs text-muted-foreground">可管理虚拟机、存储池、卷、网络和控制台，但不能修改设置或用户。</p></div>
            <div className="rounded-md border bg-muted/30 p-3"><p className="font-medium">Admin</p><p className="mt-1 text-xs text-muted-foreground">可修改全局运行模式和用户权限；仍受 CSRF 与会话校验保护。</p></div>
          </CardContent>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-3">
        <Card data-testid="settings-system-info">
          <CardHeader><CardTitle className="flex items-center gap-2"><Info className="h-4 w-4 text-primary" />版本信息</CardTitle><CardDescription>用于确认当前部署的二进制版本和升级结果。</CardDescription></CardHeader>
          <CardContent className="space-y-2 text-sm">
            {systemQuery.isLoading ? <p className="text-muted-foreground">正在读取版本信息…</p> : null}
            {systemQuery.isError ? <p className="text-destructive">{systemQuery.error instanceof Error ? systemQuery.error.message : "无法读取版本信息"}</p> : null}
            {systemQuery.data ? <>
              <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/25 px-3 py-2"><span className="text-muted-foreground">版本</span><span className="font-mono font-medium" data-testid="settings-version">{systemQuery.data.build.version || "dev"}</span></div>
              <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/25 px-3 py-2"><span className="text-muted-foreground">提交</span><span className="font-mono text-xs">{systemQuery.data.build.commit || "unknown"}</span></div>
              <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/25 px-3 py-2"><span className="text-muted-foreground">构建时间</span><span className="font-mono text-xs">{systemQuery.data.build.date || "-"}</span></div>
            </> : null}
          </CardContent>
        </Card>
        <Card data-testid="settings-database-export">
          <CardHeader><CardTitle className="flex items-center gap-2"><DatabaseBackup className="h-4 w-4 text-primary" />数据库在线导出</CardTitle><CardDescription>使用 SQLite `VACUUM INTO` 生成一致性快照，无需停止 VirtMgr。导出会写入审计日志。</CardDescription></CardHeader>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">建议在执行升级、迁移或大规模变更前下载并保存数据库副本。</p>
            <Button data-testid="settings-database-export-button" disabled={exportMutation.isPending} onClick={() => { setNotice(null); exportMutation.mutate(); }}>
              {exportMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <DatabaseBackup className="h-4 w-4" />}{exportMutation.isPending ? "正在导出..." : "下载数据库快照"}
            </Button>
          </CardContent>
        </Card>
        <Card data-testid="settings-audit-retention">
          <CardHeader><CardTitle className="flex items-center gap-2"><Archive className="h-4 w-4 text-primary" />审计日志保留</CardTitle><CardDescription>服务启动后和每 6 小时清理超过保留期的审计记录。</CardDescription></CardHeader>
          <CardContent className="space-y-3">
            <select aria-label="审计日志保留期" value={retentionDays} onChange={(event) => setRetentionDays(Number(event.target.value))} className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm">
              {[7, 30, 90, 180, 365].map((days) => <option key={days} value={days}>{days} 天</option>)}
            </select>
            <Button size="sm" disabled={retentionMutation.isPending || retentionDays === settings.data.auditRetentionDays} onClick={() => { setNotice(null); retentionMutation.mutate(retentionDays); }}>
              {retentionMutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Archive className="h-4 w-4" />}保存保留期
            </Button>
          </CardContent>
        </Card>
      </div>

      <AlertDialog open={confirmWrite} onOpenChange={setConfirmWrite}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>关闭只读模式？</AlertDialogTitle>
            <AlertDialogDescription>关闭后，所有具备权限的 Operator 和 Admin 都可以立即执行虚拟机生命周期、存储、网络和用户写操作。请确认当前环境允许写入 libvirt 和本地账号数据。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>保持只读</AlertDialogCancel>
            <AlertDialogAction data-testid="settings-confirm-write-enable" onClick={() => mutation.mutate(false)}>确认关闭只读</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
