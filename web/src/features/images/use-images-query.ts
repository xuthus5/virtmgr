import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

export const imagesQueryKey = ["images"] as const;

export function useImagesQuery() {
  return useQuery({
    queryKey: imagesQueryKey,
    queryFn: api.images,
    staleTime: 15_000,
  });
}
