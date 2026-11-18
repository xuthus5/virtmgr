import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const hostDevicesQueryKey = queryKeys.host.devices;

export function useHostDevicesQuery() {
  return useQuery({
    queryKey: hostDevicesQueryKey,
    queryFn: api.hostDevices,
    staleTime: 10_000,
  });
}
