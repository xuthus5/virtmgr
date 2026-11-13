import { useState } from "react";
import type { VMState } from "@/api/types";

export const vmListFiltersStorageKey = "virtmgr:vm-list-filters";
export const vmFavoritesStorageKey = "virtmgr:vm-favorites";

export type VMListFilters = {
  state: "" | VMState;
  tag: string;
  owner: string;
};

export const defaultVMListFilters: VMListFilters = { state: "", tag: "", owner: "" };

const validStates = new Set<VMState>(["running", "blocked", "paused", "shutting-down", "crashed", "suspended", "shutoff", "unknown"]);
const maxFavorites = 200;

export function parseVMListFilters(raw: string | null): VMListFilters {
  if (!raw) return defaultVMListFilters;
  try {
    const value = JSON.parse(raw) as Partial<Record<keyof VMListFilters, unknown>>;
    return {
      state: typeof value.state === "string" && validStates.has(value.state as VMState) ? value.state as VMState : "",
      tag: typeof value.tag === "string" ? value.tag.slice(0, 128) : "",
      owner: typeof value.owner === "string" ? value.owner.slice(0, 128) : "",
    };
  } catch {
    return defaultVMListFilters;
  }
}

export function serializeVMListFilters(filters: VMListFilters): string {
  return JSON.stringify(filters);
}

export function parseVMFavorites(raw: string | null): string[] {
  if (!raw) return [];
  try {
    const value = JSON.parse(raw) as unknown;
    if (!Array.isArray(value)) return [];
    return [...new Set(value.filter((item): item is string => typeof item === "string" && item.trim() !== ""))].slice(0, maxFavorites);
  } catch {
    return [];
  }
}

export function toggleVMFavorite(favorites: string[], uuid: string): string[] {
  if (!uuid) return favorites;
  if (favorites.includes(uuid)) return favorites.filter((item) => item !== uuid);
  return [uuid, ...favorites].slice(0, maxFavorites);
}

function readLocalStorage(key: string): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeLocalStorage(key: string, value: string) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Browsers can disable localStorage; preferences remain usable in memory.
  }
}

export function useVMListPreferences() {
  const [filters, setFiltersState] = useState<VMListFilters>(() => parseVMListFilters(readLocalStorage(vmListFiltersStorageKey)));
  const [favorites, setFavoritesState] = useState<string[]>(() => parseVMFavorites(readLocalStorage(vmFavoritesStorageKey)));

  function updateFilters(patch: Partial<VMListFilters>) {
    setFiltersState((current) => {
      const next = { ...current, ...patch };
      writeLocalStorage(vmListFiltersStorageKey, serializeVMListFilters(next));
      return next;
    });
  }

  function resetFilters() {
    setFiltersState(defaultVMListFilters);
    writeLocalStorage(vmListFiltersStorageKey, serializeVMListFilters(defaultVMListFilters));
  }

  function toggleFavorite(uuid: string) {
    setFavoritesState((current) => {
      const next = toggleVMFavorite(current, uuid);
      writeLocalStorage(vmFavoritesStorageKey, JSON.stringify(next));
      return next;
    });
  }

  return { filters, favorites, updateFilters, resetFilters, toggleFavorite };
}
