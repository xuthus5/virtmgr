import { describe, expect, it } from "vitest";
import type { ResourceMetrics } from "@/api/types";
import { appendMetricSample, formatCPUPercent, formatRate } from "@/features/metrics/metric-history";

function sample(timestamp: number): ResourceMetrics {
  return {
    timestamp,
    cpuPercent: timestamp / 10,
    memoryUsedBytes: 0,
    memoryTotalBytes: 0,
    networkRxBytes: 0,
    networkTxBytes: 0,
    networkRxBytesPerSecond: 0,
    networkTxBytesPerSecond: 0,
  };
}

describe("appendMetricSample", () => {
  it("ignores duplicate timestamps and keeps the newest samples", () => {
    let history = appendMetricSample([], sample(1), 3);
    history = appendMetricSample(history, sample(1), 3);
    history = appendMetricSample(history, sample(2), 3);
    history = appendMetricSample(history, sample(3), 3);
    history = appendMetricSample(history, sample(4), 3);
    expect(history.map((item) => item.timestamp)).toEqual([2, 3, 4]);
  });
});

describe("metric formatting", () => {
  it("formats and clamps CPU percentages", () => {
    expect(formatCPUPercent(12.345)).toBe("12.3%");
    expect(formatCPUPercent(120)).toBe("100.0%");
    expect(formatCPUPercent(-1)).toBe("0.0%");
  });

  it("formats network rates", () => {
    expect(formatRate(0)).toBe("0 B/s");
    expect(formatRate(1024)).toBe("1.0 KiB/s");
  });
});
