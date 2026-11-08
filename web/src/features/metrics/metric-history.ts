import type { ResourceMetrics } from "@/api/types";
import { formatBytes } from "@/lib/utils";

export const maxMetricSamples = 30;

export function appendMetricSample(
  history: ResourceMetrics[],
  sample: ResourceMetrics,
  limit = maxMetricSamples,
): ResourceMetrics[] {
  if (history.at(-1)?.timestamp === sample.timestamp) return history;
  const next = [...history, sample];
  return next.length > limit ? next.slice(next.length - limit) : next;
}

export function formatCPUPercent(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "0.0%";
  return `${Math.min(100, value).toFixed(1)}%`;
}

export function formatRate(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return "0 B/s";
  return `${formatBytes(bytesPerSecond)}/s`;
}
