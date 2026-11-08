import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { VMXMLFormat } from "@/api/types";

export const vmQueryKey = (uuid: string) => ["vm", uuid] as const;
export const vmMetricsQueryKey = (uuid: string) => ["vm", uuid, "metrics"] as const;

export function useVMQuery(uuid: string) {
  return useQuery({
    queryKey: vmQueryKey(uuid),
    queryFn: () => api.vm(uuid),
    staleTime: 10_000,
  });
}

export function useVMMetricsQuery(uuid: string) {
  return useQuery({
    queryKey: vmMetricsQueryKey(uuid),
    queryFn: () => api.vmMetrics(uuid),
    staleTime: 1_500,
    refetchInterval: 2_000,
    retry: 1,
  });
}

export const vmXMLQueryKey = (uuid: string, format: VMXMLFormat) => ["vm", uuid, "xml", format] as const;

export function useVMXMLQuery(uuid: string, format: VMXMLFormat) {
  return useQuery({
    queryKey: vmXMLQueryKey(uuid, format),
    queryFn: () => api.vmXML(uuid, format),
    staleTime: 10_000,
  });
}

export const vmSnapshotsQueryKey = (uuid: string) => ["vm", uuid, "snapshots"] as const;

export function useVMSnapshotsQuery(uuid: string) {
  return useQuery({
    queryKey: vmSnapshotsQueryKey(uuid),
    queryFn: () => api.vmSnapshots(uuid),
    staleTime: 10_000,
  });
}

export const snapshotPoliciesQueryKey = (uuid: string) => ["vm", uuid, "snapshot-policies"] as const;

export function useSnapshotPoliciesQuery(uuid: string) {
  return useQuery({
    queryKey: snapshotPoliciesQueryKey(uuid),
    queryFn: () => api.snapshotPolicies(uuid),
    staleTime: 10_000,
  });
}
