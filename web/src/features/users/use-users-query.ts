import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const usersQueryKey = ["users"] as const;

export function useUsersQuery(enabled: boolean) {
  return useQuery({
    queryKey: usersQueryKey,
    queryFn: api.users,
    staleTime: 10_000,
    enabled,
  });
}
