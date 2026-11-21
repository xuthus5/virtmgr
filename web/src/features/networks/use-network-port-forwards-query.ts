import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export function useNetworkPortForwardsQuery(name: string | null) {
  return useQuery({
    queryKey: queryKeys.networks.portForwards(name),
    queryFn: () => api.networkPortForwards(name as string),
    enabled: Boolean(name),
    staleTime: 5_000,
  });
}
