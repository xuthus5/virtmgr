import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import { queryKeys } from "@/lib/query-keys";

export const imagesQueryKey = queryKeys.images;

export function useImagesQuery() {
  return useQuery({
    queryKey: imagesQueryKey,
    queryFn: api.images,
    staleTime: 15_000,
  });
}
