import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { History, LoaderCircle } from "lucide-react";
import { api } from "@/api/client";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatRate } from "@/features/metrics/metric-history";
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
    queryKey: ["metrics", "history", scope, targetId, hours],
    queryFn: () => {
      const to = new Date();
      const from = new Date(to.getTime() - hours * 60 * 60 * 1000);
      return api.metricHistory({ scope, targetId, from: from.toISOString(), to: to.toISOString(), limit: 2000 });
    },
    enabled: Boolean(targetId),
    staleTime: 30_000,
    refetchInterval: 60_000,
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
                    <TableCell className="whitespace-nowrap font-mono">{formatRate(item.diskReadBytesPerSecond)} / {formatRate(item.diskWriteBytesPerSecond)}</TableCell>
                    <TableCell className="whitespace-nowrap font-mono">{formatRate(item.networkRxBytesPerSecond)} / {formatRate(item.networkTxBytesPerSecond)}</TableCell>
                  </TableRow>
                ))}</TableBody>
              </Table>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
