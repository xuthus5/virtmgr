import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const guestAgentQueryKey = (uuid: string) => ["vm", uuid, "guest-agent"] as const;

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

