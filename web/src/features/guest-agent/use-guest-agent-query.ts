import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const guestAgentQueryKey = (uuid: string) => queryKeys.vm(uuid).guestAgent;

export function useGuestAgentQuery(uuid: string, enabled = true) {
  return useQuery({
    queryKey: guestAgentQueryKey(uuid),
    queryFn: () => api.guestAgent(uuid),
    enabled,
    staleTime: 15_000,
    refetchInterval: 30_000,
    retry: 1,
  });
}

