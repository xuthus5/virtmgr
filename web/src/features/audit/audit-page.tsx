import { useState } from "react";
import { CheckCircle2, ChevronLeft, ChevronRight, ScrollText, ShieldAlert, XCircle } from "lucide-react";
import type { AuditEvent, AuditEventFilters, AuditResult, Role } from "@/api/types";
import { ErrorState, LoadingState } from "@/components/feedback/states";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useSessionQuery } from "@/features/auth/use-session";
import { useAuditEventsQuery } from "@/features/audit/use-audit-events-query";

const pageSize = 20;

const actions = [
  { value: "vm.create", label: "创建虚拟机" },
  { value: "vm.clone", label: "克隆虚拟机" },
  { value: "vm.backup", label: "备份虚拟机" },
  { value: "backup.create", label: "创建备份" },
  { value: "backup.verify", label: "校验备份" },
  { value: "backup.restore", label: "恢复备份" },
  { value: "backup.delete", label: "删除备份" },
  { value: "backup.pin", label: "更新备份保留" },
  { value: "backup.schedule.create", label: "创建备份计划" },
  { value: "backup.schedule.update", label: "更新备份计划" },
  { value: "backup.schedule.delete", label: "删除备份计划" },
  { value: "backup.schedule", label: "执行备份计划" },
  { value: "backup.schedule.run", label: "立即执行备份计划" },
  { value: "backup.repository.create", label: "添加备份存储库" },
  { value: "backup.repository.update", label: "更新备份存储库" },
  { value: "backup.repository.delete", label: "删除备份存储库" },
  { value: "vm.delete", label: "删除虚拟机" },
  { value: "vm.export", label: "导出虚拟机配置" },
  { value: "vm.xml.update", label: "更新虚拟机 XML" },
  { value: "vm.config.version.create", label: "归档配置版本" },
  { value: "vm.config.version.pin", label: "更新版本保护" },
  { value: "vm.config.rollback", label: "回滚域 XML 版本" },
  { value: "vm.autostart", label: "更新开机自启动" },
  { value: "vm.resources.update", label: "调整虚拟机资源" },
  { value: "vm.disk.resize", label: "扩容虚拟机磁盘" },
  { value: "vm.start", label: "启动虚拟机" },
  { value: "vm.shutdown", label: "关闭虚拟机" },
  { value: "vm.reboot", label: "重启虚拟机" },
  { value: "vm.reset", label: "强制重启" },
  { value: "vm.pause", label: "暂停虚拟机" },
  { value: "vm.resume", label: "恢复虚拟机" },
  { value: "vm.destroy", label: "强制下电" },
  { value: "vm.snapshot.create", label: "创建快照" },
  { value: "vm.snapshot.revert", label: "回滚快照" },
  { value: "vm.snapshot.delete", label: "删除快照" },
  { value: "vm.snapshot_policy.create", label: "创建快照策略" },
  { value: "vm.snapshot_policy.update", label: "更新快照策略" },
  { value: "vm.snapshot_policy.delete", label: "删除快照策略" },
  { value: "vm.snapshot.schedule", label: "执行快照策略" },
  { value: "vm.device.attach", label: "直通主机设备" },
  { value: "vm.device.detach", label: "移除直通设备" },
  { value: "vm_template.create", label: "创建虚拟机模板" },
  { value: "vm_template.update", label: "更新虚拟机模板" },
  { value: "vm_template.delete", label: "删除虚拟机模板" },
  { value: "storage.volume.create", label: "创建存储卷" },
  { value: "storage.volume.upload", label: "上传 ISO" },
  { value: "storage.volume.delete", label: "删除存储卷" },
  { value: "storage.pool.create", label: "创建存储池" },
  { value: "storage.pool.start", label: "启动存储池" },
  { value: "storage.pool.stop", label: "停止存储池" },
  { value: "storage.pool.refresh", label: "刷新存储池" },
  { value: "storage.pool.delete", label: "删除存储池" },
  { value: "network.create", label: "创建虚拟网络" },
  { value: "network.update", label: "编辑虚拟网络" },
  { value: "network.dhcp.host.add", label: "添加静态主机" },
  { value: "network.dhcp.host.delete", label: "删除静态主机" },
  { value: "network.start", label: "启动虚拟网络" },
  { value: "network.stop", label: "停止虚拟网络" },
  { value: "network.delete", label: "删除虚拟网络" },
  { value: "user.create", label: "创建用户" },
  { value: "user.update", label: "更新用户" },
  { value: "settings.readonly.update", label: "更新只读模式" },
];

const actionLabels = new Map(actions.map((action) => [action.value, action.label]));

const roleLabels: Record<Role, string> = {
  viewer: "只读用户",
  operator: "运维人员",
  admin: "管理员",
};

const targetKindLabels: Record<string, string> = {
  vm: "虚拟机",
  backup: "备份",
  backup_schedule: "备份计划",
  backup_repository: "备份存储库",
  vm_snapshot: "快照",
  vm_template: "虚拟机模板",
  storage_pool: "存储池",
  storage_volume: "存储卷",
  network: "虚拟网络",
  user: "用户",
  settings: "系统设置",
};

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
});

function formatTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date);
}

function formatDetailValue(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function detailSummary(detail?: Record<string, unknown>) {
  if (!detail) return "-";
  const entries = Object.entries(detail);
  if (!entries.length) return "-";
  return entries.map(([key, value]) => `${key}: ${formatDetailValue(value)}`).join(" · ");
}

function AuditRow({ event }: { event: AuditEvent }) {
  const success = event.result === "success";
  const detail = detailSummary(event.detail);
  return (
    <TableRow data-testid={`audit-row-${event.id}`}>
      <TableCell className="whitespace-nowrap font-mono text-xs text-muted-foreground">{formatTime(event.occurredAt)}</TableCell>
      <TableCell>
        <p className="font-medium">{event.actor}</p>
        <p className="text-[10px] text-muted-foreground">{roleLabels[event.role] ?? event.role}</p>
      </TableCell>
      <TableCell>
        <p className="font-medium">{actionLabels.get(event.action) ?? event.action}</p>
        <p className="font-mono text-[10px] text-muted-foreground">{event.action}</p>
      </TableCell>
      <TableCell>
        <Badge variant="outline">{targetKindLabels[event.targetKind] ?? event.targetKind}</Badge>
        <p className="mt-1 max-w-56 truncate font-mono text-xs text-muted-foreground" title={event.targetId}>{event.targetId}</p>
      </TableCell>
      <TableCell>
        <Badge variant={success ? "success" : "destructive"} className="whitespace-nowrap">
          {success ? <CheckCircle2 className="h-3 w-3" /> : <XCircle className="h-3 w-3" />}
          {success ? "成功" : "失败"}
        </Badge>
      </TableCell>
      <TableCell><span className="font-mono text-xs">{event.sourceIp}</span></TableCell>
      <TableCell><span className="block max-w-[26rem] truncate text-xs text-muted-foreground" title={detail}>{detail}</span></TableCell>
    </TableRow>
  );
}

export function AuditPage() {
  const session = useSessionQuery();
  const [action, setAction] = useState("");
  const [result, setResult] = useState<"" | AuditResult>("");
  const [page, setPage] = useState(1);
  const isAdmin = session.data?.user.role === "admin";
  const filters: AuditEventFilters = {
    action: action || undefined,
    result: result || undefined,
    page,
    pageSize,
  };
  const query = useAuditEventsQuery(filters, isAdmin);

  if (!session.data) return <LoadingState label="正在确认访问权限" />;
  if (!isAdmin) {
    return (
      <>
        <PageHeader title="审计日志" breadcrumb="总览 / 审计日志" description="审计记录包含全部写操作的执行主体、资源与结果。" />
        <Card>
          <CardContent className="flex flex-col items-center py-14 text-center">
            <ShieldAlert className="h-8 w-8 text-warning" />
            <p className="mt-4 font-medium">当前账号无权查看审计日志</p>
            <p className="mt-1 text-sm text-muted-foreground">审计日志仅对管理员开放。</p>
          </CardContent>
        </Card>
      </>
    );
  }

  const events = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const firstItem = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const lastItem = Math.min(page * pageSize, total);

  return (
    <>
      <PageHeader
        title="审计日志"
        breadcrumb="总览 / 审计日志"
        description="追踪谁在何时对哪个资源执行了什么操作，并核对执行结果。"
        actions={<Badge variant="outline"><ScrollText className="h-3 w-3" />管理员可见</Badge>}
      />

      <Card>
        <CardHeader>
          <CardTitle>操作记录</CardTitle>
          <CardDescription>按执行时间倒序排列。审计记录只读，查询不会修改宿主机状态。</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <div className="flex flex-col gap-3 border-b p-4 md:flex-row md:items-center" data-testid="audit-filters">
            <select
              aria-label="审计动作筛选"
              data-testid="audit-action-filter"
              value={action}
              onChange={(event) => { setAction(event.target.value); setPage(1); }}
              className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">全部动作</option>
              {actions.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
            </select>
            <select
              aria-label="审计结果筛选"
              data-testid="audit-result-filter"
              value={result}
              onChange={(event) => { setResult(event.target.value as "" | AuditResult); setPage(1); }}
              className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">全部结果</option>
              <option value="success">成功</option>
              <option value="failure">失败</option>
            </select>
            <p className="text-xs text-muted-foreground md:ml-auto" data-testid="audit-total">共 {total} 条审计记录</p>
          </div>

          {query.isLoading ? <LoadingState label="正在读取审计记录" /> : null}
          {query.isError ? <ErrorState message={query.error instanceof Error ? query.error.message : "无法读取审计记录"} onRetry={() => void query.refetch()} className="m-4" /> : null}
          {!query.isLoading && !query.isError && events.length === 0 ? (
            <div className="px-5 py-16 text-center" data-testid="audit-empty">
              <ScrollText className="mx-auto h-7 w-7 text-muted-foreground" />
              <p className="mt-3 text-sm font-medium">没有匹配的审计记录</p>
              <p className="mt-1 text-xs text-muted-foreground">写操作执行后会在这里留下不可变更的记录。</p>
            </div>
          ) : null}
          {events.length ? (
            <Table className="min-w-[1080px]">
              <TableHeader>
                <TableRow>
                  <TableHead>时间</TableHead>
                  <TableHead>操作者</TableHead>
                  <TableHead>动作</TableHead>
                  <TableHead>资源</TableHead>
                  <TableHead>结果</TableHead>
                  <TableHead>来源 IP</TableHead>
                  <TableHead>详情</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>{events.map((event) => <AuditRow key={event.id} event={event} />)}</TableBody>
            </Table>
          ) : null}
          {query.data ? (
            <div className="flex flex-col gap-3 border-t p-4 sm:flex-row sm:items-center" data-testid="audit-pagination">
              <p className="text-xs text-muted-foreground">显示 {firstItem}-{lastItem}，共 {total} 条</p>
              <div className="flex items-center gap-2 sm:ml-auto">
                <Button variant="outline" size="sm" data-testid="audit-prev" disabled={page <= 1 || query.isFetching} title={query.isFetching ? "正在加载审计记录" : page <= 1 ? "已经是第一页" : "查看上一页审计记录"} onClick={() => setPage((current) => Math.max(1, current - 1))}>
                  <ChevronLeft className="h-4 w-4" />上一页
                </Button>
                <span className="text-xs text-muted-foreground">第 {page} / {totalPages} 页</span>
                <Button variant="outline" size="sm" data-testid="audit-next" disabled={page >= totalPages || query.isFetching} title={query.isFetching ? "正在加载审计记录" : page >= totalPages ? "已经是最后一页" : "查看下一页审计记录"} onClick={() => setPage((current) => Math.min(totalPages, current + 1))}>
                  下一页<ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </>
  );
}
