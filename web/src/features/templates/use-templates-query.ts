import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const templatesQueryKey = queryKeys.templates;

export function useTemplatesQuery() {
  return useQuery({
    queryKey: templatesQueryKey,
    queryFn: api.templates,
    staleTime: 15_000,
  });
}
