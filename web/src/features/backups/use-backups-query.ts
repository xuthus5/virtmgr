import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { BackupFilters } from "@/api/types";

export const backupsQueryKey = (filters: BackupFilters) => ["backups", filters] as const;
export const backupSummaryQueryKey = (vmUuid = "") => ["backups", "summary", vmUuid] as const;
export const backupRepositoriesQueryKey = ["backups", "repositories"] as const;
export const backupSchedulesQueryKey = (vmUuid = "") => ["backups", "schedules", vmUuid] as const;

export function useBackupsQuery(filters: BackupFilters = {}) {
  return useQuery({
    queryKey: backupsQueryKey(filters),
    queryFn: () => api.backups(filters),
    staleTime: 5_000,
    refetchInterval: 8_000,
  });
}

export function useBackupQuery(id: string | null) {
  return useQuery({
    queryKey: ["backups", "detail", id],
    queryFn: () => api.backup(id as string),
    enabled: Boolean(id),
    staleTime: 3_000,
  });
}

export function useBackupSummaryQuery(vmUuid = "") {
  return useQuery({
    queryKey: backupSummaryQueryKey(vmUuid),
    queryFn: () => api.backupSummary(vmUuid),
    staleTime: 8_000,
  });
}

export function useBackupRepositoriesQuery(enabled = true) {
  return useQuery({
    queryKey: backupRepositoriesQueryKey,
    queryFn: api.backupRepositories,
    staleTime: 15_000,
    enabled,
  });
}

export function useBackupSchedulesQuery(vmUuid = "", enabled = true) {
  return useQuery({
    queryKey: backupSchedulesQueryKey(vmUuid),
    queryFn: () => api.backupSchedules(vmUuid),
    staleTime: 10_000,
    enabled,
  });
}
