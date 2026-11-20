import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export function useNetworkDiagnosticsQuery(name: string | null) {
  return useQuery({
    queryKey: queryKeys.networks.diagnostics(name),
    queryFn: () => api.networkDiagnostics(name as string),
    enabled: Boolean(name),
    staleTime: 10_000,
    refetchOnWindowFocus: false,
  });
}
