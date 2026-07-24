import { writable } from "svelte/store";

// Global command-palette / search modal open state. Toggled by Ctrl/Cmd+K (see SearchModal) and by
// the header search trigger.
export const searchOpen = writable(false);

export function openSearch() {
  searchOpen.set(true);
}

export function closeSearch() {
  searchOpen.set(false);
}

export function toggleSearch() {
  searchOpen.update((open) => !open);
}
