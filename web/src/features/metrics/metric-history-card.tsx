import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Database, History, LoaderCircle, TrendingUp } from "lucide-react";
import { api } from "@/api/client";
import type { CapacityForecast, GuestFilesystemSample } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatRate } from "@/features/metrics/metric-history";
import { queryKeys } from "@/lib/query-keys";
import { formatBytes } from "@/lib/utils";

type RangeHours = 1 | 24 | 168;

const ranges: Array<{ value: RangeHours; label: string }> = [
  { value: 1, label: "最近 1 小时" },
  { value: 24, label: "最近 24 小时" },
  { value: 168, label: "最近 7 天" },
];

export function MetricHistoryCard({ scope, targetId, title, description, testId }: {
  scope: "host" | "vm";
  targetId: string;
  title: string;
  description: string;
  testId: string;
}) {
  const [hours, setHours] = useState<RangeHours>(24);
  const query = useQuery({
    queryKey: queryKeys.metricsHistory(scope, targetId, hours),
    queryFn: () => {
      const to = new Date();
      const from = new Date(to.getTime() - hours * 60 * 60 * 1000);
      return api.metricHistory({ scope, targetId, from: from.toISOString(), to: to.toISOString(), limit: 2000 });
    },
    enabled: Boolean(targetId),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const guestFilesystemsQuery = useQuery({
    queryKey: queryKeys.metricsGuestFilesystems(targetId, hours),
    queryFn: () => {
      const to = new Date();
      const from = new Date(to.getTime() - hours * 60 * 60 * 1000);
      return api.guestFilesystemHistory({ vmUuid: targetId, from: from.toISOString(), to: to.toISOString(), limit: 5000 });
    },
    enabled: scope === "vm" && Boolean(targetId),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const capacityQuery = useQuery({
    queryKey: queryKeys.metricsCapacityForecast,
    queryFn: () => api.capacityForecast(),
    enabled: scope === "host",
    staleTime: 60_000,
    refetchInterval: 120_000,
  });
  const items = query.data?.items ?? [];
  const latest = items.at(-1);

  return (
    <Card data-testid={testId}>
      <CardHeader>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2"><History className="h-4 w-4 text-primary" />{title}</CardTitle>
            <CardDescription>{description} 服务端每分钟采样并保留 7 天。</CardDescription>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" data-testid={`${testId}-count`}>{items.length} 个采样点</Badge>
            <select aria-label="历史指标时间范围" value={hours} onChange={(event) => setHours(Number(event.target.value) as RangeHours)} className="h-9 rounded-md border border-input bg-card px-3 text-sm">
              {ranges.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
            </select>
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {query.isLoading ? <p className="flex items-center gap-2 p-5 text-sm text-muted-foreground"><LoaderCircle className="h-4 w-4 animate-spin" />正在读取历史指标…</p> : null}
        {query.isError ? <p role="alert" className="border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">{query.error instanceof Error ? query.error.message : "无法读取历史指标"}</p> : null}
        {!query.isLoading && !query.isError && items.length === 0 ? <p className="p-5 text-sm text-muted-foreground">当前时间范围内还没有历史采样。服务启动后约一分钟生成首批数据。</p> : null}
        {items.length ? (
          <>
            <div className="grid gap-3 border-b p-4 sm:grid-cols-2 xl:grid-cols-4">
              <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">最新 CPU</p><p className="mt-1 font-mono font-semibold">{latest ? latest.cpuPercent.toFixed(1) : "-"}%</p></div>
              <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">最新内存</p><p className="mt-1 font-mono font-semibold">{latest && latest.memoryTotalBytes ? `${((latest.memoryUsedBytes / latest.memoryTotalBytes) * 100).toFixed(1)}%` : "-"}</p><p className="text-[10px] text-muted-foreground">{latest ? `${formatBytes(latest.memoryUsedBytes)} / ${formatBytes(latest.memoryTotalBytes)}` : "-"}</p></div>
              <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">最新网络</p><p className="mt-1 font-mono text-sm">↓ {latest ? formatRate(latest.networkRxBytesPerSecond) : "-"}</p><p className="text-[10px] text-muted-foreground">↑ {latest ? formatRate(latest.networkTxBytesPerSecond) : "-"}</p></div>
              <div className="rounded-md border bg-muted/20 p-3"><p className="text-xs text-muted-foreground">最新磁盘</p><p className="mt-1 font-mono text-sm">读 {latest ? formatRate(latest.diskReadBytesPerSecond) : "-"}</p><p className="text-[10px] text-muted-foreground">写 {latest ? formatRate(latest.diskWriteBytesPerSecond) : "-"}</p></div>
            </div>
            <div className="max-h-72 overflow-auto">
              <Table className="min-w-[760px] text-xs">
                <TableHeader><TableRow><TableHead>采样时间</TableHead><TableHead>CPU</TableHead><TableHead>内存</TableHead><TableHead>磁盘读 / 写</TableHead><TableHead>网络接收 / 发送</TableHead></TableRow></TableHeader>
                <TableBody>{items.slice(-50).reverse().map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className="whitespace-nowrap font-mono">{new Date(item.sampledAt).toLocaleString("zh-CN", { hour12: false })}</TableCell>
                    <TableCell className="font-mono">{item.cpuPercent.toFixed(1)}%</TableCell>
                    <TableCell className="font-mono">{item.memoryTotalBytes ? `${((item.memoryUsedBytes / item.memoryTotalBytes) * 100).toFixed(1)}%` : "-"}</TableCell>
                    <TableCell className="whitespace-nowrap font-mono">{formatRate(item.diskReadBytesPerSecond)} / {formatRate(item.diskWriteBytesPerSecond)}<p className="text-[10px] text-muted-foreground">{formatIOPS(item.diskReadIops)} / {formatIOPS(item.diskWriteIops)} IOPS</p></TableCell>
                    <TableCell className="whitespace-nowrap font-mono">{formatRate(item.networkRxBytesPerSecond)} / {formatRate(item.networkTxBytesPerSecond)}</TableCell>
                  </TableRow>
                ))}</TableBody>
              </Table>
            </div>
          </>
        ) : null}
        {scope === "vm" ? <GuestFilesystemInsight items={guestFilesystemsQuery.data?.items ?? []} isLoading={guestFilesystemsQuery.isLoading} error={guestFilesystemsQuery.isError ? (guestFilesystemsQuery.error instanceof Error ? guestFilesystemsQuery.error.message : "无法读取客户机文件系统采样") : ""} /> : null}
        {scope === "host" ? <CapacityForecastInsight items={capacityQuery.data?.items ?? []} isLoading={capacityQuery.isLoading} error={capacityQuery.isError ? (capacityQuery.error instanceof Error ? capacityQuery.error.message : "无法读取容量预测") : ""} /> : null}
      </CardContent>
    </Card>
  );
}

function GuestFilesystemInsight({ items, isLoading, error }: { items: GuestFilesystemSample[]; isLoading: boolean; error: string }) {
  const trends = guestFilesystemTrends(items);
  return (
    <section className="border-t p-4" data-testid="guest-filesystem-history">
      <div className="flex items-center gap-2"><Database className="h-4 w-4 text-primary" /><p className="text-sm font-medium">客户机文件系统趋势</p><Badge variant="outline">{trends.length} 个挂载点</Badge></div>
      {isLoading ? <p className="mt-3 flex items-center gap-2 text-xs text-muted-foreground"><LoaderCircle className="h-3.5 w-3.5 animate-spin" />正在读取 Guest Agent 文件系统采样…</p> : null}
      {error ? <p role="alert" className="mt-3 text-xs text-destructive">{error}</p> : null}
      {!isLoading && !error && trends.length === 0 ? <p className="mt-3 text-xs text-muted-foreground">暂无文件系统采样。虚拟机运行且 Guest Agent 连接后，每分钟采集一次。</p> : null}
      {trends.length ? <div className="mt-3 space-y-2">{trends.map((trend) => (
        <div key={trend.mountPoint} className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-md border bg-muted/20 px-3 py-2 text-xs" data-testid={`guest-filesystem-${trend.mountPoint}`}>
          <span className="min-w-20 font-mono font-medium">{trend.mountPoint}</span>
          <span className="tabular-nums">{formatBytes(trend.usedBytes)} / {formatBytes(trend.totalBytes)}</span>
          <span className="font-mono font-semibold">{trend.percent.toFixed(1)}%</span>
          <span className={trend.deltaPercent > 0 ? "text-warning" : "text-muted-foreground"}>{trend.deltaPercent > 0 ? "+" : ""}{trend.deltaPercent.toFixed(1)}% 区间变化</span>
        </div>
      ))}</div> : null}
    </section>
  );
}

function CapacityForecastInsight({ items, isLoading, error }: { items: CapacityForecast[]; isLoading: boolean; error: string }) {
  return (
    <section className="border-t p-4" data-testid="capacity-forecast">
      <div className="flex items-center gap-2"><TrendingUp className="h-4 w-4 text-primary" /><p className="text-sm font-medium">存储池容量预测</p><Badge variant="outline">7 天线性回归</Badge></div>
      {isLoading ? <p className="mt-3 flex items-center gap-2 text-xs text-muted-foreground"><LoaderCircle className="h-3.5 w-3.5 animate-spin" />正在计算容量趋势…</p> : null}
      {error ? <p role="alert" className="mt-3 text-xs text-destructive">{error}</p> : null}
      {!isLoading && !error && items.length === 0 ? <p className="mt-3 text-xs text-muted-foreground">暂无足够容量采样，服务运行一段时间后生成预测。</p> : null}
      {items.length ? <div className="mt-3 grid gap-2 sm:grid-cols-2">{items.map((item) => (
        <div key={item.targetId} className="rounded-md border bg-muted/20 px-3 py-2 text-xs" data-testid={`capacity-forecast-${item.targetId}`}>
          <div className="flex items-center justify-between gap-2"><span className="font-mono font-medium">{item.targetId}</span><span className="font-mono font-semibold">{item.usagePercent.toFixed(1)}%</span></div>
          <p className="mt-1 text-muted-foreground">{formatBytes(item.usedBytes)} / {formatBytes(item.totalBytes)} · {item.sampleCount} 个样本</p>
          <p className={item.growthBytesPerDay > 0 ? "mt-1 text-warning" : "mt-1 text-muted-foreground"}>{item.growthBytesPerDay > 0 ? `增长 ${formatBytes(item.growthBytesPerDay)}/天` : "容量保持稳定"}{item.daysToFull !== undefined ? ` · 预计 ${item.daysToFull.toFixed(1)} 天后写满` : ""}</p>
        </div>
      ))}</div> : null}
    </section>
  );
}

function guestFilesystemTrends(items: GuestFilesystemSample[]) {
  const grouped = new Map<string, GuestFilesystemSample[]>();
  for (const item of items) {
    const current = grouped.get(item.mountPoint) ?? [];
    current.push(item);
    grouped.set(item.mountPoint, current);
  }
  return [...grouped.entries()].map(([mountPoint, samples]) => {
    const first = samples[0];
    const latest = samples[samples.length - 1];
    const percent = latest.totalBytes ? latest.usedBytes / latest.totalBytes * 100 : 0;
    const firstPercent = first.totalBytes ? first.usedBytes / first.totalBytes * 100 : percent;
    return { mountPoint, usedBytes: latest.usedBytes, totalBytes: latest.totalBytes, percent, deltaPercent: percent - firstPercent };
  }).sort((left, right) => right.percent - left.percent);
}

function formatIOPS(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0";
  return Math.round(value).toLocaleString("zh-CN");
}
