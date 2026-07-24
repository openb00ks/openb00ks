import { browser } from "$app/environment";
import { writable } from "svelte/store";

export const SESSION_COOKIE_KEY = "ob_session_v1";

export type Session = {
  token: string;
  tokenType: string;
  expiresAt: number;
  refreshToken?: string;
  refreshExpiresAt?: number;
};

export const session = writable<Session | null>(readStoredSession());

function isFiniteTimestamp(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function hasValidCoreFields(parsed: Session) {
  return (
    typeof parsed.token === "string" &&
    parsed.token.trim() !== "" &&
    typeof parsed.tokenType === "string" &&
    parsed.tokenType.trim() !== "" &&
    isFiniteTimestamp(parsed.expiresAt)
  );
}

function hasValidRefreshWindow(parsed: Session, now: number) {
  if (!parsed.refreshToken) {
    return false;
  }
  const refreshExpiresAt = parsed.refreshExpiresAt;
  return isFiniteTimestamp(refreshExpiresAt) && now < refreshExpiresAt;
}

function readCookie(name: string) {
  if (!browser) {
    return null;
  }
  const encodedName = `${encodeURIComponent(name)}=`;
  const parts = document.cookie ? document.cookie.split("; ") : [];
  for (const part of parts) {
    if (part.startsWith(encodedName)) {
      return decodeURIComponent(part.slice(encodedName.length));
    }
  }
  return null;
}

function writeCookie(name: string, value: string, maxAgeSeconds: number) {
  if (!browser) {
    return;
  }
  const cookie = [
    `${encodeURIComponent(name)}=${encodeURIComponent(value)}`,
    "Path=/",
    "SameSite=Lax",
    `Max-Age=${Math.max(0, Math.floor(maxAgeSeconds))}`,
  ];
  if (location.protocol === "https:") {
    cookie.push("Secure");
  }
  document.cookie = cookie.join("; ");
}

function deleteCookie(name: string) {
  if (!browser) {
    return;
  }
  document.cookie = [
    `${encodeURIComponent(name)}=`,
    "Path=/",
    "SameSite=Lax",
    "Max-Age=0",
  ].join("; ");
}

function readStoredSession(): Session | null {
  if (!browser) {
    return null;
  }
  const raw = readCookie(SESSION_COOKIE_KEY);
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as Session;
    if (!hasValidCoreFields(parsed)) {
      deleteCookie(SESSION_COOKIE_KEY);
      return null;
    }
    const now = Date.now();
    if (now >= parsed.expiresAt && !hasValidRefreshWindow(parsed, now)) {
      deleteCookie(SESSION_COOKIE_KEY);
      return null;
    }
    return parsed;
  } catch {
    deleteCookie(SESSION_COOKIE_KEY);
    return null;
  }
}

export function initSession() {
  session.set(readStoredSession());
}

export function setSession(next: Session) {
  if (!hasValidCoreFields(next)) {
    clearSession();
    return;
  }
  if (browser) {
    const now = Date.now();
    const expiry = next.refreshExpiresAt && next.refreshExpiresAt > now
      ? next.refreshExpiresAt
      : next.expiresAt;
    writeCookie(SESSION_COOKIE_KEY, JSON.stringify(next), (expiry - now) / 1000);
  }
  session.set(next);
}

export function clearSession() {
  if (browser) {
    deleteCookie(SESSION_COOKIE_KEY);
  }
  session.set(null);
}
