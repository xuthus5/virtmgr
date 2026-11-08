import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const templatesQueryKey = ["templates"] as const;

export function useTemplatesQuery() {
  return useQuery({
    queryKey: templatesQueryKey,
    queryFn: api.templates,
    staleTime: 15_000,
  });
}
