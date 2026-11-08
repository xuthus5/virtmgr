import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export function useVMsQuery(query: string, state: string, enabled = true) {
  return useQuery({
    queryKey: ["vms", query, state],
    queryFn: () => api.vms(query, state),
    staleTime: 10_000,
    enabled,
  });
}
