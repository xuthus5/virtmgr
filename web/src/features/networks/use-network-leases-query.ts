import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const networkLeasesQueryKey = queryKeys.networks.leases;

export function useNetworkLeasesQuery(name: string | null) {
  return useQuery({
    queryKey: networkLeasesQueryKey(name),
    queryFn: () => api.networkLeases(name as string),
    enabled: Boolean(name),
    staleTime: 10_000,
  });
}
