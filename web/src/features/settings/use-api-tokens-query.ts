import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const apiTokensQueryKey = queryKeys.apiTokens;

export function useAPITokensQuery() {
  return useQuery({
    queryKey: apiTokensQueryKey,
    queryFn: api.apiTokens,
    staleTime: 10_000,
  });
}
