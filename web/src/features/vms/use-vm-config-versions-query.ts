import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const vmConfigVersionsQueryKey = (uuid: string) => ["vm", uuid, "config-versions"] as const;
export const vmConfigVersionQueryKey = (uuid: string, versionNo: number) =>
  ["vm", uuid, "config-versions", versionNo] as const;

export function useVMConfigVersionsQuery(uuid: string) {
  return useQuery({
    queryKey: vmConfigVersionsQueryKey(uuid),
    queryFn: () => api.vmConfigVersions(uuid),
    staleTime: 5_000,
  });
}

export function useVMConfigVersionQuery(uuid: string, versionNo: number | null, enabled = true) {
  return useQuery({
    queryKey: vmConfigVersionQueryKey(uuid, versionNo ?? 0),
    queryFn: () => api.vmConfigVersion(uuid, versionNo as number),
    enabled: enabled && versionNo !== null && versionNo > 0,
    staleTime: 30_000,
  });
}
