import { useEffect, useState } from "react";
import { Cpu, MemoryStick, Network } from "lucide-react";
import type { ResourceMetrics } from "@/api/types";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { appendMetricSample, formatCPUPercent, formatRate, maxMetricSamples } from "@/features/metrics/metric-history";
import { formatBytes, percent } from "@/lib/utils";

type Series = {
  label: string;
  values: number[];
  className: string;
};

type TrendChartProps = {
  series: Series[];
  max?: number;
  testId: string;
};

function chartPoints(values: number[], maxValue: number): string {
  if (values.length === 0) return "";
  return values
    .map((value, index) => {
      const x = values.length === 1 ? 100 : (index / (values.length - 1)) * 100;
      const ratio = Math.min(1, Math.max(0, value / Math.max(maxValue, 1)));
      return `${x.toFixed(2)},${(30 - ratio * 27).toFixed(2)}`;
    })
    .join(" ");
}

function TrendChart({ series, max, testId }: TrendChartProps) {
  const values = series.flatMap((item) => item.values);
  const maxValue = max ?? Math.max(1, ...values);
  return (
    <svg
      viewBox="0 0 100 32"
      preserveAspectRatio="none"
      className="h-16 w-full overflow-visible"
      role="img"
      aria-label={`${series.map((item) => item.label).join("、")}趋势`}
      data-testid={testId}
      data-samples={Math.max(0, ...series.map((item) => item.values.length))}
    >
      {[9, 18, 27].map((y) => (
        <line key={y} x1="0" x2="100" y1={y} y2={y} className="stroke-border" strokeWidth="0.5" vectorEffect="non-scaling-stroke" />
      ))}
      {series.map((item, index) => (
        <polyline
          key={item.label}
          points={chartPoints(item.values, maxValue)}
          fill="none"
          stroke="currentColor"
          strokeWidth={index === 0 ? "2" : "1.5"}
          strokeLinecap="round"
          strokeLinejoin="round"
          vectorEffect="non-scaling-stroke"
          className={item.className}
        />
      ))}
    </svg>
  );
}

type TrendMetricProps = {
  icon: typeof Cpu;
  label: string;
  value: string;
  detail: string;
  chart: TrendChartProps;
  testId: string;
};

function TrendMetric({ icon: Icon, label, value, detail, chart, testId }: TrendMetricProps) {
  return (
    <div className="rounded-lg border bg-muted/20 p-4" data-testid={testId}>
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-2 text-sm font-medium"><Icon className="h-4 w-4 text-primary" />{label}</span>
        <span className="font-mono text-sm font-semibold tabular-nums">{value}</span>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
      <TrendChart {...chart} />
    </div>
  );
}

type ResourceTrendCardProps = {
  title: string;
  description: string;
  metrics?: ResourceMetrics;
  isLoading: boolean;
  error?: string;
  testId: string;
  historyKey: string;
};

export function ResourceTrendCard({ title, description, metrics, isLoading, error, testId, historyKey }: ResourceTrendCardProps) {
  const [history, setHistory] = useState<ResourceMetrics[]>([]);

  useEffect(() => {
    setHistory([]);
  }, [historyKey]);

  useEffect(() => {
    if (metrics) setHistory((current) => appendMetricSample(current, metrics));
  }, [metrics]);

  const latest = history.at(-1) ?? metrics;
  const memoryPercent = latest ? percent(latest.memoryUsedBytes, latest.memoryTotalBytes) : 0;

  return (
    <Card data-testid={testId}>
      <CardHeader className="flex-row items-start justify-between gap-3">
        <div>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
        <Badge variant="outline">每 2 秒刷新 · {history.length}/{maxMetricSamples}</Badge>
      </CardHeader>
      <CardContent>
        {error ? (
          <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive" role="alert" data-testid={`${testId}-error`}>{error}</p>
        ) : isLoading && !latest ? (
          <p className="text-sm text-muted-foreground" role="status">正在采集资源指标…</p>
        ) : latest ? (
          <div className="grid gap-3 lg:grid-cols-3">
            <TrendMetric
              icon={Cpu}
              label="CPU 使用率"
              value={formatCPUPercent(latest.cpuPercent)}
              detail={`${history.length} 个采样点`}
              testId={`${testId}-cpu`}
              chart={{ testId: `${testId}-cpu-chart`, max: 100, series: [{ label: "CPU", values: history.map((item) => item.cpuPercent), className: "text-primary" }] }}
            />
            <TrendMetric
              icon={MemoryStick}
              label="内存使用率"
              value={`${memoryPercent}%`}
              detail={`${formatBytes(latest.memoryUsedBytes)} / ${formatBytes(latest.memoryTotalBytes)}`}
              testId={`${testId}-memory`}
              chart={{ testId: `${testId}-memory-chart`, max: 100, series: [{ label: "内存", values: history.map((item) => percent(item.memoryUsedBytes, item.memoryTotalBytes)), className: "text-success" }] }}
            />
            <TrendMetric
              icon={Network}
              label="网络吞吐"
              value={`↓ ${formatRate(latest.networkRxBytesPerSecond)}`}
              detail={`↑ ${formatRate(latest.networkTxBytesPerSecond)} 发送`}
              testId={`${testId}-network`}
              chart={{ testId: `${testId}-network-chart`, series: [
                { label: "接收", values: history.map((item) => item.networkRxBytesPerSecond), className: "text-primary" },
                { label: "发送", values: history.map((item) => item.networkTxBytesPerSecond), className: "text-success" },
              ] }}
            />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">暂无资源指标。</p>
        )}
      </CardContent>
    </Card>
  );
}
