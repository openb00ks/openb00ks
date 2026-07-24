import { derived, writable } from "svelte/store";
import { apiFetch } from "$lib/api/client";

export type CurrentUser = {
  id: string;
  email: string;
  is_admin: boolean;
  default_tenant_id?: string;
  created_at?: string;
};

// The signed-in user, loaded from GET /me once a session exists. Null until loaded or when signed out.
export const currentUser = writable<CurrentUser | null>(null);

// Admin-only UI (admin dashboard, user management, system settings) gates on this.
export const isAdmin = derived(currentUser, ($user) => Boolean($user?.is_admin));

export async function loadCurrentUser() {
  try {
    currentUser.set(await apiFetch<CurrentUser>("/me"));
  } catch {
    currentUser.set(null);
  }
}

export function clearCurrentUser() {
  currentUser.set(null);
}
