import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const apiTokensQueryKey = ["api-tokens"] as const;

export function useAPITokensQuery() {
  return useQuery({
    queryKey: apiTokensQueryKey,
    queryFn: api.apiTokens,
    staleTime: 10_000,
  });
}
