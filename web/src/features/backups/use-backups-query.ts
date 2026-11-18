import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { BackupFilters } from "@/api/types";
import { queryKeys } from "@/lib/query-keys";

export const backupsQueryKey = queryKeys.backups.list;
export const backupSummaryQueryKey = queryKeys.backups.summary;
export const backupRepositoriesQueryKey = queryKeys.backups.repositories;
export const backupSchedulesQueryKey = queryKeys.backups.schedules;

export function useBackupsQuery(filters: BackupFilters = {}, enabled = true) {
  return useQuery({
    queryKey: backupsQueryKey(filters),
    queryFn: () => api.backups(filters),
    staleTime: 5_000,
    refetchInterval: 8_000,
    enabled,
  });
}

export function useBackupQuery(id: string | null) {
  return useQuery({
    queryKey: queryKeys.backups.detail(id),
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
