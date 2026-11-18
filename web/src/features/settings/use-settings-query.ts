import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const settingsQueryKey = queryKeys.settings;

export function useSettingsQuery(enabled = true) {
  return useQuery({
    queryKey: settingsQueryKey,
    queryFn: api.settings,
    staleTime: 10_000,
    enabled,
  });
}
