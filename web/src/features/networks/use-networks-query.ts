import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const networksQueryKey = ["networks"] as const;
export const hostBridgesQueryKey = ["networks", "bridges"] as const;

export function useNetworksQuery(enabled = true) {
  return useQuery({
    queryKey: networksQueryKey,
    queryFn: api.networks,
    staleTime: 15_000,
    enabled,
  });
}

export function useHostBridgesQuery(enabled = true) {
  return useQuery({
    queryKey: hostBridgesQueryKey,
    queryFn: api.networkBridges,
    staleTime: 30_000,
    enabled,
  });
}
