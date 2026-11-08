import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { TaskFilters } from "@/api/types";

export function useTasksQuery(filters: TaskFilters) {
  return useQuery({
    queryKey: ["tasks", filters],
    queryFn: () => api.tasks(filters),
    staleTime: 2_000,
    refetchInterval: 5_000,
  });
}

export function useTaskQuery(id: string | null) {
  return useQuery({
    queryKey: ["task", id],
    queryFn: () => api.task(id as string),
    enabled: Boolean(id),
    staleTime: 0,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "succeeded" || status === "failed" || status === "interrupted" ? false : 1_500;
    },
  });
}
