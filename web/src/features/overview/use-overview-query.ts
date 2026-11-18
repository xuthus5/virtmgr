import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const overviewQueryKey = queryKeys.overview;

export function useOverviewQuery(enabled = true) {
  return useQuery({
    queryKey: overviewQueryKey,
    queryFn: api.overview,
    staleTime: 15_000,
    enabled,
  });
}
