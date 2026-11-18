import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const sessionQueryKey = queryKeys.session;

export function useSessionQuery() {
  return useQuery({
    queryKey: sessionQueryKey,
    queryFn: api.session,
    retry: false,
    staleTime: 30_000,
  });
}
