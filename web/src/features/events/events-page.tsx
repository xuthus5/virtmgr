import { useEffect, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Activity, CircleAlert, CircleCheck, RefreshCw, Search, TriangleAlert, Wifi, WifiOff } from "lucide-react";
import type { EventSeverity, SystemEvent } from "@/api/types";
import { MetricCard } from "@/components/data/metric-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { PageHeader } from "@/components/layout/page-header";
import { cn } from "@/lib/utils";

type StreamState = "connecting" | "live" | "reconnecting" | "unsupported";

const severityLabels: Record<EventSeverity, string> = { info: "信息", warning: "警告", critical: "严重" };
const kindLabels: Record<string, string> = {
  "domain.defined": "虚拟机已定义",
  "domain.updated": "虚拟机配置更新",
  "domain.autostart": "虚拟机开机自启动",
  "domain.undefined": "虚拟机定义删除",
  "domain.started": "虚拟机启动",
  "domain.suspended": "虚拟机暂停",
  "domain.resumed": "虚拟机恢复",
  "domain.stopped": "虚拟机停止",
  "domain.shutdown": "虚拟机关机",
  "domain.crashed": "虚拟机崩溃",
  "domain.pmsuspended": "虚拟机电源休眠",
  "domain.changed": "虚拟机状态变化",
  "snapshot.created": "快照创建",
  "snapshot.reverted": "快照回滚",
  "snapshot.deleted": "快照删除",
  "vm.backup.created": "虚拟机备份完成",
  "vm.backup.deleted": "虚拟机备份删除",
  "vm.backup.restored": "虚拟机备份恢复",
  "storage.volume.created": "存储卷创建",
  "storage.volume.uploaded": "ISO 上传",
  "storage.volume.deleted": "存储卷删除",
  "network.created": "虚拟网络创建",
  "network.started": "虚拟网络启动",
  "network.stopped": "虚拟网络停止",
  "network.deleted": "虚拟网络删除",
};
const targetLabels: Record<string, string> = { vm: "虚拟机", vm_snapshot: "快照", storage_volume: "存储卷", network: "虚拟网络", host: "物理主机" };

const timeFormatter = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
});

function formatTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timeFormatter.format(date);
}

function severityVariant(severity: EventSeverity) {
  if (severity === "critical") return "destructive" as const;
  if (severity === "warning") return "warning" as const;
  return "outline" as const;
}

export function EventsPage() {
  const [events, setEvents] = useState<SystemEvent[]>([]);
  const [streamState, setStreamState] = useState<StreamState>("connecting");
  const [severity, setSeverity] = useState<"" | EventSeverity>("");
  const [query, setQuery] = useState("");
  const [streamKey, setStreamKey] = useState(0);

  useEffect(() => {
    if (typeof EventSource === "undefined") {
      setStreamState("unsupported");
      return;
    }
    setStreamState("connecting");
    const source = new EventSource("/api/v1/events/stream?limit=100");
    source.onopen = () => setStreamState("live");
    source.onmessage = (message) => {
      try {
        const event = JSON.parse(message.data) as SystemEvent;
        setEvents((current) => [event, ...current.filter((item) => item.id !== event.id)]
          .sort((left, right) => right.id - left.id)
          .slice(0, 200));
        setStreamState("live");
      } catch {
        setStreamState("reconnecting");
      }
    };
    source.onerror = () => setStreamState("reconnecting");
    return () => source.close();
  }, [streamKey]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return events.filter((event) => {
      if (severity && event.severity !== severity) return false;
      if (!needle) return true;
      return `${event.message} ${event.targetName ?? ""} ${event.targetId} ${event.kind}`.toLowerCase().includes(needle);
    });
  }, [events, query, severity]);

  const warnings = events.filter((event) => event.severity === "warning").length;
  const critical = events.filter((event) => event.severity === "critical").length;
  const live = streamState === "live";

  return (
    <>
      <PageHeader
        title="事件"
        breadcrumb="总览 / 事件"
        description="订阅 libvirt 域生命周期事件和 VirtMgr 资源变更事件，页面保持实时更新。"
        actions={<div className="flex items-center gap-2">
          <span data-testid="events-live-status" className={cn("inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-xs", live ? "border-success/35 bg-success/10 text-success" : "border-warning/35 bg-warning/10 text-warning")}>
            {live ? <Wifi className="h-3.5 w-3.5" /> : <WifiOff className="h-3.5 w-3.5" />}
            {streamState === "connecting" ? "正在连接" : live ? "实时连接" : "正在重连"}
          </span>
          <Button variant="outline" size="sm" onClick={() => setStreamKey((value) => value + 1)}><RefreshCw className="h-4 w-4" />重连</Button>
        </div>}
      />

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="已接收事件" value={events.length} hint="当前页面会话内的事件" icon={Activity} tone="primary" testId="events-total" />
        <MetricCard label="警告" value={warnings} hint="需要关注的资源变化" icon={TriangleAlert} tone={warnings ? "warning" : "neutral"} testId="events-warning" />
        <MetricCard label="严重" value={critical} hint="崩溃或关键故障" icon={CircleAlert} tone={critical ? "danger" : "neutral"} testId="events-critical" />
        <MetricCard label="实时通道" value={live ? "在线" : "连接中"} hint="SSE 自动重连" icon={live ? Wifi : WifiOff} tone={live ? "success" : "warning"} testId="events-stream-metric" />
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle>事件流</CardTitle>
          <CardDescription>按时间倒序展示。事件只在内存中保留最近 500 条，不作为长期审计数据。</CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <div className="flex flex-col gap-3 border-b p-4 md:flex-row md:items-center">
            <div className="relative w-full md:max-w-sm">
              <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input aria-label="搜索事件" className="pl-9" placeholder="搜索事件、虚拟机或网络" value={query} onChange={(event) => setQuery(event.target.value)} />
            </div>
            <select aria-label="事件级别筛选" data-testid="events-severity-filter" className="h-9 rounded-md border border-input bg-card px-3 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" value={severity} onChange={(event) => setSeverity(event.target.value as "" | EventSeverity)}>
              <option value="">全部级别</option>
              <option value="info">信息</option>
              <option value="warning">警告</option>
              <option value="critical">严重</option>
            </select>
            <span className="text-xs text-muted-foreground md:ml-auto">显示 {filtered.length} / {events.length} 条</span>
          </div>
          {filtered.length ? <Table>
            <TableHeader><TableRow><TableHead>时间</TableHead><TableHead>级别</TableHead><TableHead>事件</TableHead><TableHead>目标</TableHead><TableHead>来源</TableHead></TableRow></TableHeader>
            <TableBody>{filtered.map((event) => (
              <TableRow key={event.id} data-testid={`event-row-${event.id}`} data-kind={event.kind}>
                <TableCell className="whitespace-nowrap font-mono text-xs text-muted-foreground">{formatTime(event.occurredAt)}</TableCell>
                <TableCell><Badge variant={severityVariant(event.severity)}>{severityLabels[event.severity] ?? event.severity}</Badge></TableCell>
                <TableCell><p className="font-medium">{event.message}</p><p className="font-mono text-[10px] text-muted-foreground">{kindLabels[event.kind] ?? event.kind}</p></TableCell>
                <TableCell>
                  <Badge variant="outline">{targetLabels[event.targetKind] ?? event.targetKind}</Badge>
                  {event.targetKind === "vm" ? <Link to="/vms/$uuid" params={{ uuid: event.targetId }} className="mt-1 block max-w-56 truncate font-mono text-xs text-primary hover:underline">{event.targetName || event.targetId}</Link> : <p className="mt-1 max-w-56 truncate font-mono text-xs text-muted-foreground">{event.targetName || event.targetId}</p>}
                </TableCell>
                <TableCell><span className="font-mono text-xs">{event.source}</span></TableCell>
              </TableRow>
            ))}</TableBody>
          </Table> : (
            <div className="flex flex-col items-center px-4 py-14 text-center" data-testid="events-empty">
              {streamState === "reconnecting" ? <WifiOff className="h-8 w-8 text-warning" /> : <CircleCheck className="h-8 w-8 text-muted-foreground" />}
              <p className="mt-3 font-medium">暂无匹配事件</p>
              <p className="mt-1 text-sm text-muted-foreground">{events.length ? "调整筛选条件后重试。" : "新事件产生后会自动出现在这里。"}</p>
            </div>
          )}
        </CardContent>
      </Card>
    </>
  );
}
