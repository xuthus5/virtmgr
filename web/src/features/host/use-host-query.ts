import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const hostQueryKey = queryKeys.host.detail;
export const hostMetricsQueryKey = queryKeys.host.metrics;

export function useHostQuery() {
  return useQuery({
    queryKey: hostQueryKey,
    queryFn: api.host,
    staleTime: 10_000,
  });
}

export function useHostMetricsQuery() {
  return useQuery({
    queryKey: hostMetricsQueryKey,
    queryFn: api.hostMetrics,
    staleTime: 1_500,
    refetchInterval: 2_000,
    retry: 1,
  });
}
