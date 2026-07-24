import { browser } from "$app/environment";
import { apiFetch } from "$lib/api/client";
import { ACTIVE_ENTITY_STORAGE_KEY } from "$lib/stores/entity";

type PreferencesResponse = {
  default_entity_id?: string;
};

export async function syncPreferences() {
  if (!browser) {
    return;
  }

  let defaultEntityID = localStorage.getItem(ACTIVE_ENTITY_STORAGE_KEY) ?? "";

  if (!defaultEntityID) {
    try {
      const prefs = await apiFetch<PreferencesResponse>("/me/preferences");
      defaultEntityID = prefs.default_entity_id ?? "";
    } catch {
      // Best-effort sync: preferences are non-critical, so failures are non-fatal.
    }
  }

  try {
    await apiFetch("/me/preferences", {
      method: "PATCH",
      body: {
        default_entity_id: defaultEntityID,
      },
    });
  } catch {
    // Ignore preference sync failures during early development.
  }
}
