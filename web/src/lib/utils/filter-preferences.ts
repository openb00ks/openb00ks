import { browser } from "$app/environment";

export function readFilterPreference(key: string, fallback = "") {
  if (!browser) {
    return fallback;
  }
  return localStorage.getItem(key) ?? fallback;
}

export function writeFilterPreference(key: string, value: string) {
  if (!browser) {
    return;
  }
  localStorage.setItem(key, value);
}
