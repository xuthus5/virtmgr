import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const hostQueryKey = ["host"] as const;
export const hostMetricsQueryKey = ["host", "metrics"] as const;

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
