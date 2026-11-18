import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { VMXMLFormat } from "@/api/types";
import { queryKeys } from "@/lib/query-keys";

export const vmQueryKey = (uuid: string) => queryKeys.vm(uuid).root;
export const vmMetricsQueryKey = (uuid: string) => queryKeys.vm(uuid).metrics;

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

export const vmXMLQueryKey = (uuid: string, format: VMXMLFormat) => queryKeys.vm(uuid).xml(format);

export function useVMXMLQuery(uuid: string, format: VMXMLFormat) {
  return useQuery({
    queryKey: vmXMLQueryKey(uuid, format),
    queryFn: () => api.vmXML(uuid, format),
    staleTime: 10_000,
  });
}

export const vmSnapshotsQueryKey = (uuid: string) => queryKeys.vm(uuid).snapshots;

export function useVMSnapshotsQuery(uuid: string) {
  return useQuery({
    queryKey: vmSnapshotsQueryKey(uuid),
    queryFn: () => api.vmSnapshots(uuid),
    staleTime: 10_000,
  });
}

export const snapshotPoliciesQueryKey = (uuid: string) => queryKeys.vm(uuid).snapshotPolicies;

export function useSnapshotPoliciesQuery(uuid: string) {
  return useQuery({
    queryKey: snapshotPoliciesQueryKey(uuid),
    queryFn: () => api.snapshotPolicies(uuid),
    staleTime: 10_000,
  });
}
