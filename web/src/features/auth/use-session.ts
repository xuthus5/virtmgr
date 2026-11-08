import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const sessionQueryKey = ["session"] as const;

export function useSessionQuery() {
  return useQuery({
    queryKey: sessionQueryKey,
    queryFn: api.session,
    retry: false,
    staleTime: 30_000,
  });
}
