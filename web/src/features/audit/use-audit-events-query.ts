import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { AuditEventFilters } from "@/api/types";
import { queryKeys } from "@/lib/query-keys";

export function useAuditEventsQuery(filters: AuditEventFilters, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.auditEvents(filters),
    queryFn: () => api.auditEvents(filters),
    staleTime: 5_000,
    enabled,
  });
}
