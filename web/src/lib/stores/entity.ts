import { browser } from "$app/environment";
import { writable } from "svelte/store";
import { apiFetch } from "$lib/api/client";
import { syncPreferences } from "$lib/stores/preferences";

export const ACTIVE_ENTITY_STORAGE_KEY = "ob_active_entity_v1";

export type Entity = {
  id: string;
  name: string;
  fiscal_year_start_month?: number;
  fiscal_year_start_day?: number;
};

export const entities = writable<Entity[]>([]);

// False until the first /entities load resolves, so the UI can distinguish "still loading" from "no
// entities" (avoids flashing an empty-state on load or on a transient API error).
export const entitiesLoaded = writable(false);

const baseStore = writable<string | null>(null);

function normalize(value: string | null | undefined) {
  if (!value) {
    return null;
  }
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

function persist(value: string | null) {
  if (!browser) {
    return;
  }
  if (value) {
    localStorage.setItem(ACTIVE_ENTITY_STORAGE_KEY, value);
  } else {
    localStorage.removeItem(ACTIVE_ENTITY_STORAGE_KEY);
  }
}

export const activeEntity = {
  subscribe: baseStore.subscribe,
  set: (value: string | null) => {
    const next = normalize(value);
    persist(next);
    baseStore.set(next);
    void syncPreferences();
  },
  update: (updater: (value: string | null) => string | null) => {
    baseStore.update((current) => {
      const next = normalize(updater(current));
      persist(next);
      return next;
    });
    void syncPreferences();
  },
};

type PreferencesResponse = {
  default_entity_id?: string;
};

export async function initEntity() {
  if (!browser) {
    return;
  }
  const stored = normalize(localStorage.getItem(ACTIVE_ENTITY_STORAGE_KEY));
  if (stored) {
    baseStore.set(stored);
  }

  try {
    const response = await apiFetch<Entity[]>("/entities");
    entities.set(response ?? []);
  } catch {
    entities.set([]);
  } finally {
    entitiesLoaded.set(true);
  }

  if (!stored) {
    try {
      const prefs = await apiFetch<PreferencesResponse>("/me/preferences");
      const fallback = normalize(prefs.default_entity_id);
      if (fallback) {
        persist(fallback);
        baseStore.set(fallback);
      }
    } catch {
      // Best-effort: a missing/failed preference lookup falls back to no active entity.
    }
  }
}

export function selectEntity(id: string) {
  activeEntity.set(id);
}

export function clearActiveEntity() {
  if (!browser) {
    return;
  }
  localStorage.removeItem(ACTIVE_ENTITY_STORAGE_KEY);
  baseStore.set(null);
  entities.set([]);
  entitiesLoaded.set(false);
}
