import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export function useVMsQuery(query: string, state: string, enabled = true, tag = "", owner = "", folder = "", group = "") {
  return useQuery({
    queryKey: queryKeys.vms.list(query, state, tag, owner, folder, group),
    queryFn: () => api.vms(query, state, tag, owner, folder, group),
    staleTime: 10_000,
    enabled,
  });
}
