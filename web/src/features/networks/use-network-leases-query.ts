import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const networkLeasesQueryKey = (name: string | null) => ["networks", name, "leases"] as const;

export function useNetworkLeasesQuery(name: string | null) {
  return useQuery({
    queryKey: networkLeasesQueryKey(name),
    queryFn: () => api.networkLeases(name as string),
    enabled: Boolean(name),
    staleTime: 10_000,
  });
}
