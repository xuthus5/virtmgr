import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const usersQueryKey = queryKeys.users;

export function useUsersQuery(enabled: boolean) {
  return useQuery({
    queryKey: usersQueryKey,
    queryFn: api.users,
    staleTime: 10_000,
    enabled,
  });
}
