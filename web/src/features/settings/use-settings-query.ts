import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const settingsQueryKey = ["settings"] as const;

export function useSettingsQuery(enabled = true) {
  return useQuery({
    queryKey: settingsQueryKey,
    queryFn: api.settings,
    staleTime: 10_000,
    enabled,
  });
}
