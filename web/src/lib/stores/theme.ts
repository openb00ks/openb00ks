import { browser } from "$app/environment";
import { writable } from "svelte/store";

export const THEME_STORAGE_KEY = "ob_theme";

export type Theme = "light" | "dark" | "system";

export const theme = writable<Theme>("system");
let mediaQuery: MediaQueryList | null = null;

function resolveTheme(next: Theme) {
  if (next !== "system") {
    return next;
  }
  if (!browser) {
    return "light";
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function applyTheme(next: Theme) {
  if (!browser) {
    return;
  }
  const resolved = resolveTheme(next);
  document.documentElement.classList.toggle("dark", resolved === "dark");
}

function normalize(value: string | null) {
  switch (value) {
    case "light":
    case "dark":
    case "system":
      return value;
    default:
      return null;
  }
}

function persist(next: Theme) {
  if (!browser) {
    return;
  }
  localStorage.setItem(THEME_STORAGE_KEY, next);
}

function handleSystemChange() {
  theme.update((current) => {
    if (current === "system") {
      applyTheme(current);
    }
    return current;
  });
}

export async function initTheme() {
  if (!browser) {
    return;
  }
  if (!mediaQuery) {
    mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    mediaQuery.addEventListener("change", handleSystemChange);
  }

  const stored = normalize(localStorage.getItem(THEME_STORAGE_KEY));
  if (stored) {
    theme.set(stored);
    applyTheme(stored);
    return;
  }
  theme.set("system");
  applyTheme("system");
}

export function setTheme(next: Theme) {
  persist(next);
  theme.set(next);
  applyTheme(next);
}

export function toggleTheme() {
  theme.update((current) => {
    const next = current === "dark" ? "light" : "dark";
    persist(next);
    applyTheme(next);
    return next;
  });
}
