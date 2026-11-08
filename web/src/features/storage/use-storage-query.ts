import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const storagePoolsQueryKey = ["storage", "pools"] as const;

export function useStoragePoolsQuery(enabled = true) {
  return useQuery({
    queryKey: storagePoolsQueryKey,
    queryFn: api.storagePools,
    staleTime: 15_000,
    enabled,
  });
}

export function useStorageVolumesQuery(pool: string) {
  return useQuery({
    queryKey: ["storage", "volumes", pool],
    queryFn: () => api.storageVolumes(pool),
    staleTime: 15_000,
    enabled: Boolean(pool),
  });
}
