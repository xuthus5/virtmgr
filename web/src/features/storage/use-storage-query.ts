import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const storagePoolsQueryKey = queryKeys.storage.pools;

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
    queryKey: queryKeys.storage.volumes(pool),
    queryFn: () => api.storageVolumes(pool),
    staleTime: 15_000,
    refetchOnMount: "always",
    enabled: Boolean(pool),
  });
}
