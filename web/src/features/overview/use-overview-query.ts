import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const overviewQueryKey = ["overview"] as const;

export function useOverviewQuery(enabled = true) {
  return useQuery({
    queryKey: overviewQueryKey,
    queryFn: api.overview,
    staleTime: 15_000,
    enabled,
  });
}
