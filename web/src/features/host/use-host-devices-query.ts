import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const hostDevicesQueryKey = ["host", "devices"] as const;

export function useHostDevicesQuery() {
  return useQuery({
    queryKey: hostDevicesQueryKey,
    queryFn: api.hostDevices,
    staleTime: 10_000,
  });
}
